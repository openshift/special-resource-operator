package filter

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"github.com/openshift-psap/special-resource-operator/pkg/hash"
	"github.com/openshift-psap/special-resource-operator/pkg/kernel"
	"github.com/openshift-psap/special-resource-operator/pkg/lifecycle"
	"github.com/openshift-psap/special-resource-operator/pkg/storage"
	"github.com/openshift-psap/special-resource-operator/pkg/warn"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	sroGVK = "SpecialResource"
	owned  = "specialresource.openshift.io/owned"
)

var (
	Mode string
	log  = zap.New(zap.UseDevMode(true)).WithName(color.Print("filter", color.Purple))
)

func SetLabel(obj *unstructured.Unstructured) error {

	var labels map[string]string

	if labels = obj.GetLabels(); labels == nil {
		labels = make(map[string]string)
	}

	labels[owned] = "true"
	obj.SetLabels(labels)

	return SetSubResourceLabel(obj)
}

func SetSubResourceLabel(obj *unstructured.Unstructured) error {

	if obj.GetKind() == "DaemonSet" || obj.GetKind() == "Deployment" ||
		obj.GetKind() == "StatefulSet" {

		labels, found, err := unstructured.NestedMap(obj.Object, "spec", "template", "metadata", "labels")
		if err != nil {
			return err
		}
		if !found {
			return errors.New("Labels not found")
		}

		labels[owned] = "true"
		if err := unstructured.SetNestedMap(obj.Object, labels, "spec", "template", "metadata", "labels"); err != nil {
			return err
		}
	}

	if obj.GetKind() == "BuildConfig" {
		log.Info("TODO: how to set label ownership for Builds and related Pods")
		/*
			output, found, err := unstructured.NestedMap(obj.Object, "spec", "output")
			if err != nil {
				return err
			}
			if !found {
				return errors.New("output not found")
			}

			label := make(map[string]interface{})
			label["name"] = owned
			label["value"] = "true"
			imageLabels := append(make([]interface{}, 0), label)

			if _, found := output["imageLabels"]; !found {
				err := unstructured.SetNestedSlice(obj.Object, imageLabels, "spec", "output", "imageLabels")
				if err != nil {
					return err
				}
			}
		*/
	}
	return nil
}

func IsSpecialResource(obj client.Object) bool {

	kind := obj.GetObjectKind().GroupVersionKind().Kind

	if kind == sroGVK {
		log.Info(Mode+" IsSpecialResource (sroGVK)", "Name", obj.GetName(), "Type", reflect.TypeOf(obj).String())
		return true
	}

	t := reflect.TypeOf(obj).String()

	if strings.Contains(t, sroGVK) {
		log.Info(Mode+" IsSpecialResource (reflect)", "Name", obj.GetName(), "Type", reflect.TypeOf(obj).String())
		return true

	}

	// If SRO owns the resource than it cannot be a SpecialResource
	if Owned(obj) {
		return false
	}

	// We need this because a newly created SpecialResource will not yet
	// have a GVK
	selfLink := obj.GetSelfLink()
	if strings.Contains(selfLink, "/apis/sro.openshift.io/v") {
		log.Info(Mode+" IsSpecialResource (selflink)", "Name", obj.GetName(), "Type", reflect.TypeOf(obj).String())
		return true
	}
	if kind == "" {
		objstr := fmt.Sprintf("%+v", obj)
		if strings.Contains(objstr, "sro.openshift.io/v") {
			log.Info(Mode+" IsSpecialResource (contains)", "Name", obj.GetName(), "Type", reflect.TypeOf(obj).String())
			return true
		}
	}

	return false
}

func Owned(obj client.Object) bool {

	for _, owner := range obj.GetOwnerReferences() {
		if owner.Kind == sroGVK {
			log.Info(Mode+" Owned (sroGVK)", "Name", obj.GetName(),
				"Type", reflect.TypeOf(obj).String())
			return true
		}
	}

	var labels map[string]string

	if labels = obj.GetLabels(); labels != nil {
		if _, found := labels[owned]; found {
			log.Info(Mode+" Owned (label)", "Name", obj.GetName(),
				"Type", reflect.TypeOf(obj).String())
			return true
		}
	}
	return false
}

func Predicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {

			Mode = "CREATE"
			// If a specialresource dependency is deleted we
			/* want to recreate it so handle the delete event */
			obj := e.Object

			if IsSpecialResource(obj) {
				return true
			}

			if Owned(obj) {
				return true
			}

			return false
		},

		UpdateFunc: func(e event.UpdateEvent) bool {
			// Ignore updates if the resourceVersion does not change
			// resourceVersion is updated when the object is modified

			/* UPDATING THE STATUS WILL INCREASE THE RESOURCEVERSION DISABLING
			 * BUT KEEPING FOR REFERENCE
			if e.MetaOld.GetResourceVersion() == e.MetaNew.GetResourceVersion() {
				return false
			}*/
			Mode = "UPDATE"

			e.ObjectOld.GetGeneration()
			e.ObjectOld.GetOwnerReferences()

			obj := e.ObjectNew

			// Required for the case when pods are deleted due to OS upgrade

			if Owned(obj) && kernel.IsObjectAffine(obj) {
				if e.ObjectOld.GetGeneration() == e.ObjectNew.GetGeneration() &&
					e.ObjectOld.GetResourceVersion() == e.ObjectNew.GetResourceVersion() {
					return false
				} else {
					log.Info(Mode+" Owned Generation or resourceVersion Changed for kernel affine object",
						"Name", obj.GetName(), "Type", reflect.TypeOf(obj).String())
					if reflect.TypeOf(obj).String() == "*v1.DaemonSet" && e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() {
						err := lifecycle.UpdateDaemonSetPods(obj)
						warn.OnError(err)
					}
					return true
				}
			}

			// Ignore updates to CR status in which case metadata.Generation does not change
			if e.ObjectOld.GetGeneration() == e.ObjectNew.GetGeneration() {
				return false
			}
			// Some objects will increase generation on Update SRO sets the
			// resourceversion New = Old so we can filter on those even if an
			// update does not change anything see e.g. Deployment or SCC
			if e.ObjectOld.GetResourceVersion() == e.ObjectNew.GetResourceVersion() {
				return false
			}

			// If a specialresource dependency is updated we
			// want to reconcile it, handle the update event

			if IsSpecialResource(obj) {
				log.Info(Mode+" IsSpecialResource GenerationChanged",
					"Name", obj.GetName(), "Type", reflect.TypeOf(obj).String())
				return true
			}

			// If we do not own the object, do not care
			if Owned(obj) {

				log.Info(Mode+" Owned GenerationChanged",
					"Name", obj.GetName(), "Type", reflect.TypeOf(obj).String())

				if reflect.TypeOf(obj).String() == "*v1.DaemonSet" {
					err := lifecycle.UpdateDaemonSetPods(obj)
					warn.OnError(err)
				}

				return true
			}

			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {

			Mode = "DELETE"
			// If a specialresource dependency is deleted we
			/* want to recreate it so handle the delete event */
			obj := e.Object
			if IsSpecialResource(obj) {
				return true
			}

			// If we do not own the object, do not care
			if Owned(obj) {

				ins := types.NamespacedName{
					Namespace: os.Getenv("OPERATOR_NAMESPACE"),
					Name:      "special-resource-lifecycle",
				}
				key, err := hash.FNV64a(obj.GetNamespace() + obj.GetName())
				if err != nil {
					warn.OnError(err)
					return false
				}
				err = storage.DeleteConfigMapEntry(key, ins)
				warn.OnError(err)

				return true
			}
			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {

			Mode = "GENERIC"

			// If a specialresource dependency is updated we
			// want to reconcile it, handle the update event
			obj := e.Object
			if IsSpecialResource(obj) {
				return true
			}
			// If we do not own the object, do not care
			if Owned(obj) {
				return true
			}
			return false

		},
	}
}
