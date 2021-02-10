package controllers

import (
	"context"
	"fmt"
	"sort"

	"github.com/openshift-psap/special-resource-operator/pkg/exit"
	"github.com/openshift-psap/special-resource-operator/pkg/metrics"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const specialresourceFinalizer = "finalizer.sro.openshift.io"

func reconcileFinalizers(r *SpecialResourceReconciler) error {
	if contains(r.specialresource.GetFinalizers(), specialresourceFinalizer) {
		// Run finalization logic for specialresource
		if err := finalizeSpecialResource(r); err != nil {
			log.Info("Finalization logic failed.", "error", fmt.Sprintf("%v", err))
			return err
		}

		controllerutil.RemoveFinalizer(&r.specialresource, specialresourceFinalizer)
		err := r.Update(context.TODO(), &r.specialresource)
		if err != nil {
			log.Info("Could not remove finalizer after running finalization logic", "error", fmt.Sprintf("%v", err))
			return err
		}
	}
	return nil
}

func finalizeSpecialResource(r *SpecialResourceReconciler) error {
	// TODO(user): Add the cleanup steps that the operator
	// needs to do before the CR can be deleted. Examples
	// of finalizers include performing backups and deleting
	// resources that are not owned by this CR, like a PVC.
	var err error
	var config *unstructured.Unstructured
	var manifests map[string]interface{}
	var found bool

	config, err = getHardwareConfiguration(r)
	if err != nil {
		log.Info("Failed to get hardware states while reconciling finalizer")
		return err
	}

	manifests, found, err = unstructured.NestedMap(config.Object, "data")
	exit.OnErrorOrNotFound(found, err)

	states := make([]string, 0, len(manifests))
	for key := range manifests {
		states = append(states, key)
	}

	sort.Strings(states)

	for _, state := range states {
		log.Info("Deleting metric for", "state:", state)
		metrics.DeleteCompleteStates(r.specialresource.Name, state)
	}

	log.Info("Successfully finalized", "SpecialResource:", r.specialresource.Name)
	return nil
}

func addFinalizer(r *SpecialResourceReconciler) error {
	log.Info("Adding finalizer to special resource")
	controllerutil.AddFinalizer(&r.specialresource, specialresourceFinalizer)

	// Update CR
	err := r.Update(context.TODO(), &r.specialresource)
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
