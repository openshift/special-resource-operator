package filter

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/special-resource-operator/api/v1beta1"
	"github.com/openshift/special-resource-operator/pkg/kernel"
	"github.com/openshift/special-resource-operator/pkg/lifecycle"
	"github.com/openshift/special-resource-operator/pkg/storage"
	"github.com/openshift/special-resource-operator/pkg/utils"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type Filter interface {
	GetPredicates() predicate.Predicate
	GetMode() string
}

func NewFilter(kind, ownedLabel string, lifecycle lifecycle.Lifecycle, storage storage.Storage, kernelData kernel.KernelData) Filter {
	return &filter{
		log:        zap.New(zap.UseDevMode(true)).WithName(utils.Print("filter", utils.Purple)),
		lifecycle:  lifecycle,
		storage:    storage,
		kernelData: kernelData,
		kind:       kind,
		ownedLabel: ownedLabel,
	}
}

type filter struct {
	log        logr.Logger
	lifecycle  lifecycle.Lifecycle
	storage    storage.Storage
	kernelData kernel.KernelData
	kind       string
	ownedLabel string

	mode string
}

func (f *filter) GetMode() string {
	return f.mode
}

func (f *filter) isSpecialResourceUnmanaged(obj client.Object) bool {
	sr, ok := obj.(*v1beta1.SpecialResource)
	if !ok {
		return false
	}
	return sr.Spec.ManagementState == operatorv1.Unmanaged
}

func (f *filter) isSpecialResource(obj client.Object) bool {

	kind := obj.GetObjectKind().GroupVersionKind().Kind

	if kind == f.kind {
		return true
	}

	t := reflect.TypeOf(obj).String()

	if strings.Contains(t, f.kind) {
		return true

	}

	// If SRO owns the resource then it cannot be a SpecialResource
	if f.owned(obj) {
		return false
	}

	// We need this because a newly created SpecialResource will not yet
	// have a GVK
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

func (f *filter) owned(obj client.Object) bool {

	for _, owner := range obj.GetOwnerReferences() {
		if owner.Kind == f.kind {
			return true
		}
	}

	var labels map[string]string

	if labels = obj.GetLabels(); labels != nil {
		if _, found := labels[f.ownedLabel]; found {
			return true
		}
	}
	return false
}

func (f *filter) GetPredicates() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {

			f.mode = "CREATE"
			// If a specialresource dependency is deleted we
			// want to recreate it so handle the delete event
			obj := e.Object

			if f.isSpecialResource(obj) {
				if !f.isSpecialResourceUnmanaged(obj) {
					f.log.Info("Creating managed special resource", "name", obj.GetName())
					return true
				}
				return false
			}

			if f.owned(obj) {
				f.log.Info("Creating owned object", "name", obj.GetName(), "namespace", obj.GetNamespace(), "kind", obj.GetObjectKind())
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
			f.mode = "UPDATE"

			e.ObjectOld.GetGeneration()
			e.ObjectOld.GetOwnerReferences()

			obj := e.ObjectNew

			// Required for the case when pods are deleted due to OS upgrade
			if f.owned(obj) && f.kernelData.IsObjectAffine(obj) {
				if e.ObjectOld.GetGeneration() == e.ObjectNew.GetGeneration() &&
					e.ObjectOld.GetResourceVersion() == e.ObjectNew.GetResourceVersion() {
					return false
				} else {
					if reflect.TypeOf(obj).String() == "*v1.DaemonSet" && e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() {
						err := f.lifecycle.UpdateDaemonSetPods(context.TODO(), obj)
						utils.WarnOnError(err)
					}
					if f.isSpecialResource(obj) && f.isSpecialResourceUnmanaged(obj) {
						return false
					}
					f.log.Info("Updating owned object. generation or resourceVersion kernel affine changed",
						"name", obj.GetName(), "namespace", obj.GetNamespace(), "kind", obj.GetObjectKind())
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

			if f.isSpecialResource(obj) {
				if f.isSpecialResourceUnmanaged(obj) {
					return false
				}
				f.log.Info("Updating special resource", "name", obj.GetName())
				return true
			}

			// If we do not own the object, do not care
			if f.owned(obj) {
				if reflect.TypeOf(obj).String() == "*v1.DaemonSet" {
					err := f.lifecycle.UpdateDaemonSetPods(context.TODO(), obj)
					utils.WarnOnError(err)
				}
				f.log.Info("Updating owned object", "name", obj.GetName(), "namespace", obj.GetNamespace(), "kind", obj.GetObjectKind())
				return true
			}

			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {

			f.mode = "DELETE"
			// If a specialresource dependency is deleted we
			/* want to recreate it so handle the delete event */
			obj := e.Object
			if f.isSpecialResource(obj) {
				f.log.Info("Deleting special resource", "name", obj.GetName())
				return true
			}

			// If we do not own the object, do not care
			if f.owned(obj) {

				ins := types.NamespacedName{
					Namespace: os.Getenv("OPERATOR_NAMESPACE"),
					Name:      "special-resource-lifecycle",
				}
				key, err := utils.FNV64a(obj.GetNamespace() + obj.GetName())
				if err != nil {
					utils.WarnOnError(err)
					return false
				}
				err = f.storage.DeleteConfigMapEntry(context.TODO(), key, ins)
				utils.WarnOnError(err)
				f.log.Info("Deleting owned object", "name", obj.GetName(), "namespace", obj.GetNamespace(), "kind", obj.GetObjectKind())
				return true
			}
			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {

			f.mode = "GENERIC"

			// If a specialresource dependency is updated we
			// want to reconcile it, handle the update event
			obj := e.Object
			if f.isSpecialResource(obj) {
				if !f.isSpecialResourceUnmanaged(obj) {
					f.log.Info("Generic special resource", "name", obj.GetName())
					return true
				}
				return false
			}
			// If we do not own the object, do not care
			if f.owned(obj) {
				f.log.Info("Generic owned resource", "name", obj.GetName(), "namespace", obj.GetNamespace(), "kind", obj.GetObjectKind())
				return true
			}
			return false

		},
	}
}
