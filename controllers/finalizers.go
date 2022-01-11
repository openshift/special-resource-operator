package controllers

import (
	"context"
	"fmt"
	"strings"

	"github.com/openshift-psap/special-resource-operator/pkg/state"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const specialresourceFinalizer = "sro.openshift.io/finalizer"

func reconcileFinalizers(ctx context.Context, r *SpecialResourceReconciler) error {
	if contains(r.specialresource.GetFinalizers(), specialresourceFinalizer) {
		// Run finalization logic for specialresource
		if err := finalizeSpecialResource(ctx, r); err != nil {
			log.Error(err, "Finalization logic failed.")
			return err
		}

		controllerutil.RemoveFinalizer(&r.specialresource, specialresourceFinalizer)
		err := r.KubeClient.Update(ctx, &r.specialresource)
		if err != nil {
			log.Error(err, "Could not remove finalizer after running finalization logic")
			return err
		}
	}
	return nil
}

func finalizeNodes(ctx context.Context, r *SpecialResourceReconciler, remove string) error {
	nodeList, err := r.KubeClient.GetNodesByLabels(ctx, r.specialresource.Spec.NodeSelector)
	if err != nil {
		return fmt.Errorf("Failed to get node list based on NodeSelector labels during finalization: %v", err)
	}
	for _, node := range nodeList.Items {
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
		err := r.KubeClient.Update(ctx, &node)
		if apierrors.IsForbidden(err) {
			return errors.Wrap(err, "forbidden check Role, ClusterRole and Bindings for operator %s")
		}
		if apierrors.IsConflict(err) {
			return fmt.Errorf("node Conflict Label %s err %s", state.CurrentName, err)
		}

	}
	return nil
}

func finalizeSpecialResource(ctx context.Context, r *SpecialResourceReconciler) error {
	// TODO(user): Add the cleanup steps that the operator
	// needs to do before the CR can be deleted. Examples
	// of finalizers include performing backups and deleting
	// resources that are not owned by this CR, like a PVC.

	// If this special resources is deleted we're going to remove all
	// specialresource labels from the nodes.
	if r.specialresource.Name == "special-resource-preamble" {
		err := finalizeNodes(ctx, r, "specialresource.openshift.io")
		if err != nil {
			log.Error(err, "finalizeSpecialResource special-resource-preamble failed")
			return err
		}
	}
	err := finalizeNodes(ctx, r, "specialresource.openshift.io/state-"+r.specialresource.Name)
	if err != nil {
		return err
	}

	if r.specialresource.Name != "special-resource-preamble" {
		ns := unstructured.Unstructured{}

		ns.SetKind("Namespace")
		ns.SetAPIVersion("v1")
		ns.SetName(r.specialresource.Spec.Namespace)
		key := client.ObjectKeyFromObject(&ns)

		err := r.KubeClient.Get(ctx, key, &ns)
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
				err = r.KubeClient.Delete(ctx, &ns)
				if err != nil {
					log.Error(err, "Failed to delete namespace", "namespace", r.specialresource.Spec.Namespace)
					return err
				}
				err = r.PollActions.ForResourceUnavailability(ctx, &ns)
				if err != nil {
					log.Error(err, "Failed to delete namespace", "namespace", r.specialresource.Spec.Namespace)
					return err
				}
			}
		}
	}

	log.Info("Successfully finalized", "SpecialResource:", r.specialresource.Name)
	return nil
}

func addFinalizer(ctx context.Context, r *SpecialResourceReconciler) error {
	log.Info("Adding finalizer to special resource")
	controllerutil.AddFinalizer(&r.specialresource, specialresourceFinalizer)

	// Update CR
	err := r.KubeClient.Update(ctx, &r.specialresource)
	if err != nil {
		log.Error(err, "Adding finalizer failed")
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
