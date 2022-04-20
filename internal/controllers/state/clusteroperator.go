package state

import (
	"context"
	"fmt"
	"os"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1helpers "github.com/openshift/library-go/pkg/operator/v1helpers"
	srov1beta1 "github.com/openshift/special-resource-operator/api/v1beta1"
	"github.com/openshift/special-resource-operator/pkg/clients"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

//go:generate mockgen -source=clusteroperator.go -package=state -destination=mock_clusteroperator_api.go

// ClusterOperatorManager provides helpers for the controller to publish and update its ClusterOperator resource.
type ClusterOperatorManager interface {
	GetOrCreate(context.Context) error
	Refresh(context.Context, []configv1.ClusterOperatorStatusCondition) error
}

type clusterOperatorManager struct {
	kubeClient   clients.ClientsInterface
	operatorName string
}

// NewClusterOperatorManager creates a new ClusterOperatorManager instance.
func NewClusterOperatorManager(kubeClient clients.ClientsInterface, operatorName string) ClusterOperatorManager {
	return &clusterOperatorManager{
		kubeClient:   kubeClient,
		operatorName: operatorName,
	}
}

// GetOrCreate refreshes the locally cached configv1.ClusterOperator instance with what is currently in the cluster.
// If the cluster has no such instance, this method creates one.
func (com *clusterOperatorManager) GetOrCreate(ctx context.Context) error {
	if _, err := com.getOrCreate(ctx); err != nil {
		return fmt.Errorf("could not get or create the ClusterOperator: %v", err)
	}

	return nil
}

// Refresh updates the configv1.ClusterOperator with cond and the value of the RELEASE_VERSION environment variable.
// If no configv1.ClusterOperator is available in the cluster, it is created.
func (com *clusterOperatorManager) Refresh(ctx context.Context, cond []configv1.ClusterOperatorStatusCondition) error {
	clusterOperatorAvailable, err := com.kubeClient.HasResource(configv1.SchemeGroupVersion.WithResource("clusteroperators"))

	if err != nil {
		return fmt.Errorf("Cannot discover ClusterOperator api resource: %w", err)
	}

	if !clusterOperatorAvailable {
		ctrl.LoggerFrom(ctx).Info("Warning: ClusterOperator resource not available. Can be ignored on vanilla k8s.")
		return nil
	}

	// If clusterOperator CRD does not exist, warn and return nil,
	co, err := com.getOrCreate(ctx)
	if err != nil {
		return fmt.Errorf("could not get or create the ClusterOperator: %v", err)
	}

	ctrl.LoggerFrom(ctx).Info("Updating ClusterOperator")

	co.Status.Conditions = cond

	if err := com.clusterOperatorUpdateRelatedObjects(ctx, co); err != nil {
		return fmt.Errorf("cannot set ClusterOperator related objects: %w", err)
	}

	if releaseVersion := os.Getenv("RELEASE_VERSION"); len(releaseVersion) > 0 {
		operatorv1helpers.SetOperandVersion(&co.Status.Versions, configv1.OperandVersion{Name: "operator", Version: releaseVersion})
	}

	if _, err := com.kubeClient.ClusterOperatorUpdateStatus(ctx, co, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("could not update ClusterOperator: %w", err)
	}

	return nil
}

func (com *clusterOperatorManager) getOrCreate(ctx context.Context) (*configv1.ClusterOperator, error) {
	co, err := com.kubeClient.ClusterOperatorGet(ctx, com.operatorName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			ctrl.LoggerFrom(ctx).Info("SRO's ClusterOperator not found - creating")

			co = &configv1.ClusterOperator{
				ObjectMeta: metav1.ObjectMeta{Name: com.operatorName},
			}

			co, err = com.kubeClient.ClusterOperatorCreate(ctx, co, metav1.CreateOptions{})
			if err != nil {
				return nil, fmt.Errorf("failed to create ClusterOperator %s: %w", co.Name, err)
			}
		} else {
			return nil, err
		}
	}

	return co, nil
}

func (com *clusterOperatorManager) clusterOperatorUpdateRelatedObjects(ctx context.Context, co *configv1.ClusterOperator) error {
	relatedObjects := []configv1.ObjectReference{
		{Group: "", Resource: "namespaces", Name: os.Getenv("OPERATOR_NAMESPACE")},
		{Group: "sro.openshift.io", Resource: "specialresources", Name: ""},
	}

	// Get all specialresource objects
	specialresources := &srov1beta1.SpecialResourceList{}
	err := com.kubeClient.List(ctx, specialresources)
	if err != nil {
		return err
	}

	//Add namespace for each specialresource to related objects
	for _, sr := range specialresources.Items {
		if sr.Spec.Namespace != "" { // preamble specialresource has no namespace
			ctrl.LoggerFrom(ctx).Info("Adding namespace to ClusterOperator's RelatedObjects", "namespace", sr.Spec.Namespace)
			relatedObjects = append(relatedObjects, configv1.ObjectReference{Group: "", Resource: "namespaces", Name: sr.Spec.Namespace})
		}
	}

	co.Status.RelatedObjects = relatedObjects

	return nil
}
