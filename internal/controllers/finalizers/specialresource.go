package finalizers

import (
	"context"
	"fmt"
	"strings"

	"github.com/openshift/special-resource-operator/api/v1beta1"
	"github.com/openshift/special-resource-operator/pkg/clients"
	"github.com/openshift/special-resource-operator/pkg/poll"
	"github.com/openshift/special-resource-operator/pkg/utils"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const finalizerString = "sro.openshift.io/finalizer"
const prevVersionFinalizerString = "finalizer.sro.openshift.io"

type SpecialResourceFinalizer interface {
	AddFinalizerToSpecialResource(ctx context.Context, sr *v1beta1.SpecialResource) error
	Finalize(ctx context.Context, sr *v1beta1.SpecialResource) error
	RemoveResources(ctx context.Context, ownedLabel string, sr *v1beta1.SpecialResource) error
}

type specialResourceFinalizer struct {
	kubeClient  clients.ClientsInterface
	pollActions poll.PollActions
}

func NewSpecialResourceFinalizer(
	kubeClient clients.ClientsInterface,
	pollActions poll.PollActions,
) SpecialResourceFinalizer {
	return &specialResourceFinalizer{
		kubeClient:  kubeClient,
		pollActions: pollActions,
	}
}

func (srf *specialResourceFinalizer) AddFinalizerToSpecialResource(ctx context.Context, sr *v1beta1.SpecialResource) error {
	if utils.StringSliceContains(sr.GetFinalizers(), finalizerString) {
		return nil
	}

	ctrl.LoggerFrom(ctx).Info("Adding finalizer")

	controllerutil.AddFinalizer(sr, finalizerString)

	// Update CR
	if err := srf.kubeClient.Update(ctx, sr); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "Adding finalizer failed")
		return err
	}

	return nil
}

func (srf *specialResourceFinalizer) Finalize(ctx context.Context, sr *v1beta1.SpecialResource) error {
	if utils.StringSliceContains(sr.GetFinalizers(), finalizerString) {
		// Run finalization logic for specialresource
		if err := srf.finalizeSpecialResource(ctx, sr); err != nil {
			ctrl.LoggerFrom(ctx).Error(err, "Finalization logic failed.")
			return err
		}

		controllerutil.RemoveFinalizer(sr, finalizerString)
		// remove the 4.9 finalizer string, in case operator was update from 4.9
		// to later versions
		controllerutil.RemoveFinalizer(sr, prevVersionFinalizerString)

		if err := srf.kubeClient.Update(ctx, sr); err != nil {
			ctrl.LoggerFrom(ctx).Error(err, "Could not remove finalizer after running finalization logic")
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
			return errors.Wrap(err, "forbidden check Role, ClusterRole and Bindings for operator")
		}
		if apierrors.IsConflict(err) {
			return fmt.Errorf("conflict during label removal: %s", err)
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
	if err := srf.finalizeNodes(ctx, sr, "specialresource.openshift.io/state-"+sr.Name); err != nil {
		return err
	}

	ns := unstructured.Unstructured{}

	ns.SetKind("Namespace")
	ns.SetAPIVersion("v1")
	ns.SetName(sr.Spec.Namespace)
	key := client.ObjectKeyFromObject(&ns)

	if err := srf.kubeClient.Get(ctx, key, &ns); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		} else {
			ctrl.LoggerFrom(ctx).Error(err, "Failed to get namespace", "namespace", sr.Spec.Namespace, "SpecialResource", sr.Name)
			return err
		}
	}

	for _, owner := range ns.GetOwnerReferences() {
		if owner.Kind == "SpecialResource" {
			if err := srf.kubeClient.Delete(ctx, &ns); err != nil {
				ctrl.LoggerFrom(ctx).Error(err, "Failed to delete namespace", "namespace", sr.Spec.Namespace)
				return err
			}

			if err := srf.pollActions.ForResourceUnavailability(ctx, &ns); err != nil {
				ctrl.LoggerFrom(ctx).Error(err, "Failed to delete namespace", "namespace", sr.Spec.Namespace)
				return err
			}
		}
	}
	return nil
}

func (srf *specialResourceFinalizer) RemoveResources(ctx context.Context, ownedLabel string, sr *v1beta1.SpecialResource) error {
	_, apiResources, err := srf.kubeClient.ServerGroupsAndResources()
	if err != nil {
		return fmt.Errorf("unable to retrieve server groups and resources: %w", err)
	}
	for _, apiResource := range apiResources {
		for _, resource := range apiResource.APIResources {
			if err := srf.deleteResource(ctx, ownedLabel, sr.Spec.Namespace, apiResource.GroupVersion, resource.Kind, sr.UID); err != nil {
				return fmt.Errorf("unable to delete owned resources %s/%s: %w", apiResource.GroupVersion, resource.Kind, err)
			}
		}
	}
	return nil
}

func (srf *specialResourceFinalizer) deleteResource(ctx context.Context, ownedLabel, namespace, apiVersion, kind string, UID types.UID) error {
	obj := unstructured.UnstructuredList{}
	obj.SetKind(kind)
	obj.SetAPIVersion(apiVersion)
	sl := metav1.LabelSelector{
		MatchLabels: map[string]string{ownedLabel: "true"},
	}
	selector, _ := metav1.LabelSelectorAsSelector(&sl)
	if err := srf.kubeClient.List(ctx, &obj, &client.ListOptions{
		LabelSelector: selector,
		Namespace:     namespace,
	}); err != nil {
		return nil
	}
	for _, object := range obj.Items {
		for _, or := range object.GetOwnerReferences() {
			if or.UID != UID {
				continue
			}
			if err := srf.kubeClient.Delete(ctx, &object); err != nil {
				return err
			}
			ctrl.LoggerFrom(ctx).Info("Owned object deleted", "name", object.GetName(), "Kind", object.GetKind())
		}
	}
	return nil
}
