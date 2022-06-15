package controllers

import (
	"context"
	"fmt"
	"strings"

	"github.com/openshift-psap/special-resource-operator/pkg/cache"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/poll"
	"github.com/openshift-psap/special-resource-operator/pkg/state"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const specialresourceFinalizer = "sro.openshift.io/finalizer"
const prevVersionFinalizerString = "finalizer.sro.openshift.io"

var (
	ns unstructured.Unstructured
)

func init() {
	ns.SetKind("Namespace")
	ns.SetAPIVersion("v1")
}

func reconcileFinalizers(r *SpecialResourceReconciler) error {
	if contains(r.specialresource.GetFinalizers(), specialresourceFinalizer) {
		// Run finalization logic for specialresource
		if err := finalizeSpecialResource(r); err != nil {
			log.Info("Finalization logic failed.", "error", fmt.Sprintf("%v", err))
			return err
		}

		controllerutil.RemoveFinalizer(&r.specialresource, specialresourceFinalizer)
		// remove the 4.9 finalizer string, in case operator was update from 4.9
		// to later versions
		controllerutil.RemoveFinalizer(&r.specialresource, prevVersionFinalizerString)
		err := clients.Interface.Update(context.TODO(), &r.specialresource)
		if err != nil {
			log.Info("Could not remove finalizer after running finalization logic", "error", fmt.Sprintf("%v", err))
			return err
		}
	}
	return nil
}

func finalizeNodes(r *SpecialResourceReconciler, remove string) error {
	for _, node := range cache.Node.List.Items {
		labels := node.GetLabels()
		update := make(map[string]string)
		// Remove all specialresource labels
		for k, v := range labels {
			if strings.Contains(k, remove) {
				continue
			}
			update[k] = v
		}

		node.SetLabels(update)
		err := clients.Interface.Update(context.TODO(), &node)
		if apierrors.IsForbidden(err) {
			return errors.Wrap(err, "forbidden check Role, ClusterRole and Bindings for operator %s")
		}
		if apierrors.IsConflict(err) {
			var err error

			if cacheErr := cache.Nodes(r.specialresource.Spec.NodeSelector, true); cacheErr != nil {
				return errors.Wrap(cacheErr, "Could not cache nodes for api conflict")
			}

			return fmt.Errorf("node Conflict Label %s err %s", state.CurrentName, err)
		}

	}
	return nil
}

func finalizeSpecialResource(r *SpecialResourceReconciler) error {
	// TODO(user): Add the cleanup steps that the operator
	// needs to do before the CR can be deleted. Examples
	// of finalizers include performing backups and deleting
	// resources that are not owned by this CR, like a PVC.

	// If this special resources is deleted we're going to remove all
	// specialresource labels from the nodes.
	if r.specialresource.Name == "special-resource-preamble" {
		err := finalizeNodes(r, "specialresource.openshift.io")
		if err != nil {
			log.Error(err, "finalizeSpecialResource special-resource-preamble failed")
			return err
		}
	}
	err := finalizeNodes(r, "specialresource.openshift.io/state-"+r.specialresource.Name)
	if err != nil {
		return err
	}

	if r.specialresource.Name != "special-resource-preamble" {

		ns.SetName(r.specialresource.Spec.Namespace)
		key := client.ObjectKeyFromObject(&ns)

		err := clients.Interface.Get(context.TODO(), key, &ns)
		if err != nil {
			if apierrors.IsNotFound(err) {
				log.Info("Successfully finalized (Namespace IsNotFound)", "SpecialResource:", r.specialresource.Name)
				return nil
			} else {
				log.Error(err, "Failed to get namespace", "namespace", r.specialresource.Spec.Namespace, "SpecialResource", r.specialresource.Name)
				return err
			}
		}

		for _, owner := range ns.GetOwnerReferences() {
			if owner.Kind == "SpecialResource" {
				log.Info("Namespaces is owned by SpecialResource deleting")
				err = clients.Interface.Delete(context.TODO(), &ns)
				if err != nil {
					log.Error(err, "Failed to delete namespace", "namespace", r.specialresource.Spec.Namespace)
					return err
				}
				err = poll.ForResourceUnavailability(&ns)
				if err != nil {
					log.Error(err, "Failed waiting for resource being completely deleted", "namespace", r.specialresource.Spec.Namespace)
					return err
				}
			}
		}
	}

	log.Info("Successfully finalized", "SpecialResource:", r.specialresource.Name)
	return nil
}

func addFinalizer(r *SpecialResourceReconciler) error {
	log.Info("Adding finalizer to special resource")
	controllerutil.AddFinalizer(&r.specialresource, specialresourceFinalizer)

	// Update CR
	err := clients.Interface.Update(context.TODO(), &r.specialresource)
	if err != nil {
		log.Info("Adding finalizer failed", "error", fmt.Sprintf("%v", err))
		return err
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
