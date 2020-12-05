package controllers

import (
	"context"
	"os"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1helpers "github.com/openshift/library-go/pkg/operator/v1helpers"
	errs "github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	client "sigs.k8s.io/controller-runtime/pkg/client"
)

// Operator Status
func operatorStatusUpdate(obj *unstructured.Unstructured, r *SpecialResourceReconciler, label map[string]string) {

	var current []string

	for k := range label {
		current = append(current, k)
	}

	r.specialresource.Status.State = current[0]

	err := r.Status().Update(context.TODO(), &r.specialresource)
	if err != nil {
		log.Error(err, "Failed to update SpecialResource status")
		return
	}
}

// ClusterOperator Status ------------------------------------------------------
func (r *SpecialResourceReconciler) clusterOperatorStatusGetOrCreate() error {

	clusterOperators := &configv1.ClusterOperatorList{}

	opts := []client.ListOption{}
	err := r.List(context.TODO(), clusterOperators, opts...)
	exitOnError(err)

	for _, clusterOperator := range clusterOperators.Items {
		if clusterOperator.GetName() == r.GetName() {
			clusterOperator.DeepCopyInto(&r.clusterOperator)
			return nil
		}
	}
	// If we land here there is no clusteroperator object for SRO
	// Unsure if we need to create a clusteroperator object ...
	log.Info(prettyPrint("TOOD: Do we need to create a ClusterOperator Object? ", Red))
	return errs.New("ClusterOperator can not be found")
}

func (r *SpecialResourceReconciler) clusterOperotarStatusSet() error {

	if releaseVersion := os.Getenv("RELEASE_VERSION"); len(releaseVersion) > 0 {
		operatorv1helpers.SetOperandVersion(&r.clusterOperator.Status.Versions, configv1.OperandVersion{Name: "operator", Version: releaseVersion})
	}
	return nil
}

func (r *SpecialResourceReconciler) clusterOperatorConditionsDefault() {
	available := configv1.ClusterOperatorStatusCondition{
		Type:               configv1.OperatorAvailable,
		Status:             configv1.ConditionTrue,
		Reason:             "AsExpected",
		Message:            "Reconciled all special resources",
		LastTransitionTime: metav1.Now(),
	}
	progressing := configv1.ClusterOperatorStatusCondition{
		Type:               configv1.OperatorProgressing,
		Status:             configv1.ConditionFalse,
		Reason:             "Reconciled",
		Message:            "SpecialResources up to date",
		LastTransitionTime: metav1.Now(),
	}
	degraded := configv1.ClusterOperatorStatusCondition{
		Type:               configv1.OperatorDegraded,
		Status:             configv1.ConditionFalse,
		Reason:             "Reconciled",
		Message:            "SpecialResources up to date",
		LastTransitionTime: metav1.Now(),
	}
	conditions := []configv1.ClusterOperatorStatusCondition{}

	conditions = append(conditions, available)
	conditions = append(conditions, progressing)
	conditions = append(conditions, degraded)

	r.clusterOperator.Status.Conditions = conditions

}

func (r *SpecialResourceReconciler) clusterOperatorStatusReconcile() error {

	if err := r.clusterOperatorStatusGetOrCreate(); err != nil {
		return errs.Wrap(err, "Cannot get or create ClusterOperator")
	}

	r.clusterOperatorConditionsDefault()

	if err := r.clusterOperotarStatusSet(); err != nil {
		return errs.Wrap(err, "Cannot update the ClusterOperator status")
	}

	if err := r.clusterOperatorStatusUpdate(); err != nil {
		return errs.Wrap(err, "Could not update ClusterOperator")
	}

	return nil
}

func (r *SpecialResourceReconciler) clusterOperatorStatusUpdate() error {

	if _, err := configclient.ClusterOperators().UpdateStatus(context.TODO(), &r.clusterOperator, metav1.UpdateOptions{}); err != nil {
		return err
	}
	return nil
}
