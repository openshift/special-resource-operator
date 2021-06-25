package filter

import (
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"github.com/openshift-psap/special-resource-operator/pkg/exit"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var (
	owned string
	log   logr.Logger
)

func init() {
	owned = "specialresource.openshift.io/owned"
}

func init() {
	log = zap.New(zap.UseDevMode(true)).WithName(color.Print("filter", color.Purple))
}

func SetLabel(obj *unstructured.Unstructured) {

	var labels map[string]string

	if labels = obj.GetLabels(); labels == nil {
		labels = make(map[string]string)
	}

	labels[owned] = "true"
	obj.SetLabels(labels)

	SetSubResourceLabel(obj)
}

func SetSubResourceLabel(obj *unstructured.Unstructured) {

	if obj.GetKind() == "DaemonSet" {
		labels, found, err := unstructured.NestedMap(obj.Object, "spec", "template", "metadata", "labels")
		exit.OnErrorOrNotFound(found, err)

		labels[owned] = "true"
		err = unstructured.SetNestedMap(obj.Object, labels, "spec", "template", "metadata", "labels")
		exit.OnError(err)
	}

	if obj.GetKind() == "BuildConfig" {
		log.Info("TODO: how to set label ownership for Builds and related Pods")
		/*
			output, found, err := unstructured.NestedMap(obj.Object, "spec", "output")
			exit.OnErrorOrNotFound(found, err)

			label := make(map[string]interface{})
			label["name"] = owned
			label["value"] = "true"
			imageLabels := append(make([]interface{}, 0), label)

			if _, found := output["imageLabels"]; !found {
				err := unstructured.SetNestedSlice(obj.Object, imageLabels, "spec", "output", "imageLabels")
				exit.OnError(err)
			}
		*/
	}
}

var Mode string

func IsSpecialResource(obj client.Object) bool {

	kind := obj.GetObjectKind().GroupVersionKind().Kind

	if kind == "SpecialResource" {
		return true
	}

	selfLink := obj.GetSelfLink()
	if strings.Contains(selfLink, "/apis/sro.openshift.io/v") {
		return true
	}
	if kind == "" {
		objstr := fmt.Sprintf("%+v", obj)
		if strings.Contains(objstr, "sro.openshift.io/v") {
			return true
		}
	}

	return false
}

func NotOwned(obj client.Object) bool {

	refs := obj.GetOwnerReferences()

	for _, ref := range refs {
		if ref.Kind == "SpecialResource" {
			return false
		}
	}
	var labels map[string]string

	if labels = obj.GetLabels(); labels == nil {
		if _, found := labels[owned]; found {
			return false
		}
	}
	return true
}

func Predicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {

			Mode = "CREATE"
			// If a specialresource dependency is deleted we
			/* want to recreate it so handle the delete event */
			obj := e.Object
			if IsSpecialResource(obj) {
				log.Info(Mode+" IsSpecialResource", "GenerationChanged", e.Object.GetName())
				return true
			}

			if NotOwned(obj) {
				return false
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

			// Ignore updates to CR status in which case metadata.Generation does not change
			if e.ObjectOld.GetGeneration() == e.ObjectNew.GetGeneration() {
				return false
			}
			// Some objects will increate generation on Update SRO sets the
			// resourceversion New = Old so we can filter on those even if an
			// update does not change anything see e.g. Deployment or SCC
			if e.ObjectOld.GetResourceVersion() == e.ObjectNew.GetResourceVersion() {
				return false
			}

			// If a specialresource dependency is updated we
			// want to reconcile it, handle the update event
			obj := e.ObjectNew
			if IsSpecialResource(obj) {
				log.Info(Mode+" IsSpecialResource", "GenerationChanged", obj.GetName())
				return true
			}

			// If we do not own the object, do not care
			if NotOwned(obj) {
				return false
			}
			// We own the resource, do something
			log.Info(Mode+" Owned", "GenerationChanged", obj.GetName())
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {

			Mode = "DELETE"
			// If a specialresource dependency is deleted we
			/* want to recreate it so handle the delete event */
			obj := e.Object
			if IsSpecialResource(obj) {
				log.Info(Mode+" IsSpecialResource", "GenerationChanged", obj.GetName())
				return true
			}

			// If we do not own the object, do not care
			if NotOwned(obj) {
				return false
			}
			// We own the resource, do something
			log.Info(Mode+" Owned", "GenerationChanged", obj.GetName())
			return true
		},
		GenericFunc: func(e event.GenericEvent) bool {

			Mode = "GENERIC"

			// If a specialresource dependency is updated we
			// want to reconcile it, handle the update event
			obj := e.Object
			if IsSpecialResource(obj) {
				log.Info(Mode+" IsSpecialResource", "GenerationChanged", obj.GetName())
				return true
			}
			// If we do not own the object, do not care
			if NotOwned(obj) {
				return false
			}
			// We own the resource, do something
			log.Info(Mode+" Owned", "GenerationChanged", obj.GetName())
			return true

		},
	}
}
