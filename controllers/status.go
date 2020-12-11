package controllers

import (
	"context"
	"fmt"
	"os"

	"github.com/openshift-psap/special-resource-operator/pkg/color"
	configv1 "github.com/openshift/api/config/v1"
	operatorv1helpers "github.com/openshift/library-go/pkg/operator/v1helpers"
	errs "github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	client "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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
	log.Info(color.PrettyPrint("TOOD: Do we need to create a ClusterOperator Object? ", color.Red))
	return errs.New("ClusterOperator can not be found")
}

func (r *SpecialResourceReconciler) clusterOperotarStatusSet() error {

	if releaseVersion := os.Getenv("RELEASE_VERSION"); len(releaseVersion) > 0 {
		operatorv1helpers.SetOperandVersion(&r.clusterOperator.Status.Versions, configv1.OperandVersion{Name: "operator", Version: releaseVersion})
	}
	return nil
}

func (r *SpecialResourceReconciler) clusterOperatorStatusReconcile(
	conditions []configv1.ClusterOperatorStatusCondition) error {

	r.clusterOperator.Status.Conditions = conditions

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

// ReportSpecialResourcesStatus Depdending on what error we're getting from the
// reconciliation loop we're updating the status
// nil -> All things good and default conditiions can be applied
func ReportSpecialResourcesStatus(r *SpecialResourceReconciler, req ctrl.Request) (ctrl.Result, error) {

	conditions := conditionsNotAvailableProgressingNotDegraded(
		"Reconciling "+req.Name,
		"Reconciling "+req.Name,
		conditionDegradedDefaultMsg,
	)

	log = r.Log.WithName(color.PrettyPrint("status", color.Blue))
	if err := r.clusterOperatorStatusGetOrCreate(); err != nil {
		return reconcile.Result{Requeue: true}, errs.Wrap(err, "Cannot get or create ClusterOperator")
	}

	log.Info("Reconcling ClusterOperator")
	if err := r.clusterOperatorStatusReconcile(conditions); err != nil {
		log.Info("Reconcling ClusterOperator failed", "error", fmt.Sprintf("%v", err))
		return reconcile.Result{Requeue: true}, nil
	}

	ctrlResult, err := ReconcilerSpecialResources(r, req)

	log = r.Log.WithName(color.PrettyPrint("status", color.Blue))
	if err := r.clusterOperatorStatusGetOrCreate(); err != nil {
		return reconcile.Result{Requeue: true}, errs.Wrap(err, "Cannot get or create ClusterOperator")
	}
	if err == nil {
		conditions = conditionsAvailableNotProgressingNotDegraded()
	}

	log.Info("Reconcling ClusterOperator")
	if err := r.clusterOperatorStatusReconcile(conditions); err != nil {
		log.Info("Reconcling ClusterOperator failed", "error", fmt.Sprintf("%v", err))
		return reconcile.Result{Requeue: true}, nil
	}

	return ctrlResult, err
}
