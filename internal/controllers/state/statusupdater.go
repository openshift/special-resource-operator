package state

import (
	"context"
	"fmt"

	"github.com/openshift-psap/special-resource-operator/api/v1beta1"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	Ready       = "SpecialResourceIsReady"
	Progressing = "Progressing"
	Errored     = "ErrorHasOccurred"

	// Following strings are Reasons

	Success                       = "Success"
	HandlingState                 = "HandlingState"
	MarkedForDeletion             = "MarkedForDeletion"
	ChartFailure                  = "ChartFailure"
	DependencyChartFailure        = "DependencyChartFailure"
	FailedToStoreDependencyInfo   = "FailedToStoreDependencyInfo"
	FailedToCreateDependencySR    = "FailedToCreateDependencySR"
	FailedToDeployDependencyChart = "FailedToDeployDependencyChart"
	FailedToDeployChart           = "FailedToDeployChart"
)

//go:generate mockgen -source=statusupdater.go -package=state -destination=mock_statusupdater_api.go

type StatusUpdater interface {
	SetAsReady(ctx context.Context, sr *v1beta1.SpecialResource, reason, message string) error
	SetAsProgressing(ctx context.Context, sr *v1beta1.SpecialResource, reason, message string) error
	SetAsErrored(ctx context.Context, sr *v1beta1.SpecialResource, reason, message string) error
}

type statusUpdater struct {
	kubeClient clients.ClientsInterface
}

func NewStatusUpdater(kubeClient clients.ClientsInterface) StatusUpdater {
	return &statusUpdater{
		kubeClient: kubeClient,
	}
}

// SetAsProgressing changes SpecialResource's Progressing condition as true and changes Ready and Errored conditions to false, and updates the status in the API.
func (su *statusUpdater) SetAsProgressing(ctx context.Context, sr *v1beta1.SpecialResource, reason, message string) error {
	meta.SetStatusCondition(&sr.Status.Conditions, metav1.Condition{Type: v1beta1.SpecialResourceProgressing, Status: metav1.ConditionTrue, Reason: reason, Message: message})
	meta.SetStatusCondition(&sr.Status.Conditions, metav1.Condition{Type: v1beta1.SpecialResourceReady, Status: metav1.ConditionFalse, Reason: Progressing})
	meta.SetStatusCondition(&sr.Status.Conditions, metav1.Condition{Type: v1beta1.SpecialResourceErrored, Status: metav1.ConditionFalse, Reason: Progressing})

	sr.Status.State = fmt.Sprintf("Progressing: %s", message)

	return su.kubeClient.StatusUpdate(ctx, sr)
}

// SetAsReady changes SpecialResource's Ready condition as true and changes Progressing and Errored conditions to false, and updates the status in the API.
func (su *statusUpdater) SetAsReady(ctx context.Context, sr *v1beta1.SpecialResource, reason, message string) error {
	meta.SetStatusCondition(&sr.Status.Conditions, metav1.Condition{Type: v1beta1.SpecialResourceReady, Status: metav1.ConditionTrue, Reason: reason, Message: message})
	meta.SetStatusCondition(&sr.Status.Conditions, metav1.Condition{Type: v1beta1.SpecialResourceProgressing, Status: metav1.ConditionFalse, Reason: Ready})
	meta.SetStatusCondition(&sr.Status.Conditions, metav1.Condition{Type: v1beta1.SpecialResourceErrored, Status: metav1.ConditionFalse, Reason: Ready})

	sr.Status.State = fmt.Sprintf("Ready: %s", message)

	return su.kubeClient.StatusUpdate(ctx, sr)
}

// SetAsErrored changes SpecialResource's Errored condition as true and changes Ready and Progressing conditions to false, and updates the status in the API.
func (su *statusUpdater) SetAsErrored(ctx context.Context, sr *v1beta1.SpecialResource, reason, message string) error {
	meta.SetStatusCondition(&sr.Status.Conditions, metav1.Condition{Type: v1beta1.SpecialResourceErrored, Status: metav1.ConditionTrue, Reason: reason, Message: message})
	meta.SetStatusCondition(&sr.Status.Conditions, metav1.Condition{Type: v1beta1.SpecialResourceReady, Status: metav1.ConditionFalse, Reason: Errored})
	meta.SetStatusCondition(&sr.Status.Conditions, metav1.Condition{Type: v1beta1.SpecialResourceProgressing, Status: metav1.ConditionFalse, Reason: Errored})

	sr.Status.State = fmt.Sprintf("Errored: %s", message)

	return su.kubeClient.StatusUpdate(ctx, sr)
}
