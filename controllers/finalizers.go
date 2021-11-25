package controllers

import (
	"context"
	"fmt"
	"strings"

	"github.com/openshift-psap/special-resource-operator/pkg/cache"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/poll"
	"github.com/openshift-psap/special-resource-operator/pkg/state"
	"github.com/openshift-psap/special-resource-operator/pkg/warn"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const specialresourceFinalizer = "sro.openshift.io/finalizer"

var pollActions = poll.New()

func reconcileFinalizers(r *SpecialResourceReconciler) error {
	if contains(r.specialresource.GetFinalizers(), specialresourceFinalizer) {
		// Run finalization logic for specialresource
		if err := finalizeSpecialResource(r); err != nil {
			log.Info("Finalization logic failed.", "error", fmt.Sprintf("%v", err))
			return err
		}

		controllerutil.RemoveFinalizer(&r.specialresource, specialresourceFinalizer)
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

			if err = cache.Nodes(r.specialresource.Spec.NodeSelector, true); err != nil {
				return errors.Wrap(err, "Could not cache nodes for api conflict")
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
		warn.OnError(err)
	}
	err := finalizeNodes(r, "specialresource.openshift.io/state-"+r.specialresource.Name)
	warn.OnError(err)

	if r.specialresource.Name != "special-resource-preamble" {
		ns := unstructured.Unstructured{}

		ns.SetKind("Namespace")
		ns.SetAPIVersion("v1")
		ns.SetName(r.specialresource.Spec.Namespace)
		key := client.ObjectKeyFromObject(&ns)

		err := clients.Interface.Get(context.TODO(), key, &ns)
		if apierrors.IsNotFound(err) {
			log.Info("Successfully finalized (IsNotFound)", "SpecialResource:", r.specialresource.Name)
			return nil
		}

		for _, owner := range ns.GetOwnerReferences() {
			if owner.Kind == "SpecialResource" {
				log.Info("Namespaces is owned by SpecialResource deleting")
				err = clients.Interface.Delete(context.TODO(), &ns)
				if !apierrors.IsNotFound(err) {
					warn.OnError(err)
				}
				err = pollActions.ForResourceUnavailability(&ns)
				warn.OnError(err)
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
