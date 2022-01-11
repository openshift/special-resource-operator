package finalizers

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/openshift-psap/special-resource-operator/api/v1beta1"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/poll"
	"github.com/openshift-psap/special-resource-operator/pkg/state"
	"github.com/openshift-psap/special-resource-operator/pkg/utils"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const FinalizerString = "sro.openshift.io/finalizer"

type SpecialResourceFinalizer interface {
	AddToSpecialResource(ctx context.Context, sr *v1beta1.SpecialResource) error
	Finalize(ctx context.Context, sr *v1beta1.SpecialResource) error
}

type specialResourceFinalizer struct {
	kubeClient  clients.ClientsInterface
	log         logr.Logger
	pollActions poll.PollActions
}

func NewSpecialResourceFinalizer(
	kubeClient clients.ClientsInterface,
	pollActions poll.PollActions,
) SpecialResourceFinalizer {
	return &specialResourceFinalizer{
		kubeClient:  kubeClient,
		log:         ctrl.Log.WithName("finalizers"),
		pollActions: pollActions,
	}
}

func (srf *specialResourceFinalizer) AddToSpecialResource(ctx context.Context, sr *v1beta1.SpecialResource) error {
	srf.log.Info("Adding finalizer to special resource")
	controllerutil.AddFinalizer(sr, FinalizerString)

	// Update CR
	if err := srf.kubeClient.Update(ctx, sr); err != nil {
		srf.log.Error(err, "Adding finalizer failed")
		return err
	}

	return nil
}

func (srf *specialResourceFinalizer) Finalize(ctx context.Context, sr *v1beta1.SpecialResource) error {
	if utils.StringSliceContains(sr.GetFinalizers(), FinalizerString) {
		// Run finalization logic for specialresource
		if err := srf.finalizeSpecialResource(ctx, sr); err != nil {
			srf.log.Error(err, "Finalization logic failed.")
			return err
		}

		controllerutil.RemoveFinalizer(sr, FinalizerString)

		if err := srf.kubeClient.Update(ctx, sr); err != nil {
			srf.log.Error(err, "Could not remove finalizer after running finalization logic")
			return err
		}
	}
	return nil
}

func (srf *specialResourceFinalizer) finalizeNodes(ctx context.Context, sr *v1beta1.SpecialResource, remove string) error {
	nodes, err := srf.kubeClient.GetNodesByLabels(ctx, sr.Spec.NodeSelector)
	if err != nil {
		return fmt.Errorf("could not fetch nodes: %v", err)
	}

	for _, node := range nodes.Items {
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
		err := srf.kubeClient.Update(ctx, &node)
		if apierrors.IsForbidden(err) {
			return errors.Wrap(err, "forbidden check Role, ClusterRole and Bindings for operator %s")
		}
		if apierrors.IsConflict(err) {
			return fmt.Errorf("node Conflict Label %s err %s", state.CurrentName, err)
		}

	}
	return nil
}

func (srf *specialResourceFinalizer) finalizeSpecialResource(ctx context.Context, sr *v1beta1.SpecialResource) error {
	// TODO(user): Add the cleanup steps that the operator
	// needs to do before the CR can be deleted. Examples
	// of finalizers include performing backups and deleting
	// resources that are not owned by this CR, like a PVC.

	// If this special resources is deleted we're going to remove all
	// specialresource labels from the nodes.
	if sr.Name == "special-resource-preamble" {
		err := srf.finalizeNodes(ctx, sr, "specialresource.openshift.io")
		if err != nil {
			srf.log.Error(err, "finalizeSpecialResource special-resource-preamble failed")
			return err
		}
	}

	if err := srf.finalizeNodes(ctx, sr, "specialresource.openshift.io/state-"+sr.Name); err != nil {
		return err
	}

	if sr.Name != "special-resource-preamble" {
		ns := unstructured.Unstructured{}

		ns.SetKind("Namespace")
		ns.SetAPIVersion("v1")
		ns.SetName(sr.Spec.Namespace)
		key := client.ObjectKeyFromObject(&ns)

		if err := srf.kubeClient.Get(ctx, key, &ns); err != nil {
			if apierrors.IsNotFound(err) {
				srf.log.Info("Successfully finalized (Namespace IsNotFound)", "SpecialResource:", sr.Name)
				return nil
			} else {
				srf.log.Error(err, "Failed to get namespace", "namespace", sr.Spec.Namespace, "SpecialResource", sr.Name)
				return err
			}
		}

		for _, owner := range ns.GetOwnerReferences() {
			if owner.Kind == "SpecialResource" {
				srf.log.Info("Namespaces is owned by SpecialResource deleting")

				if err := srf.kubeClient.Delete(ctx, &ns); err != nil {
					srf.log.Error(err, "Failed to delete namespace", "namespace", sr.Spec.Namespace)
					return err
				}

				if err := srf.pollActions.ForResourceUnavailability(ctx, &ns); err != nil {
					srf.log.Error(err, "Failed to delete namespace", "namespace", sr.Spec.Namespace)
					return err
				}
			}
		}
	}

	srf.log.Info("Successfully finalized", "SpecialResource:", sr.Name)
	return nil
}
