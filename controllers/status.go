package controllers

import (
	"context"
	"os"

	srov1beta1 "github.com/openshift-psap/special-resource-operator/api/v1beta1"
	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"github.com/openshift-psap/special-resource-operator/pkg/exit"
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
	exit.OnError(err)

	for _, clusterOperator := range clusterOperators.Items {
		if clusterOperator.GetName() == r.GetName() {
			clusterOperator.DeepCopyInto(&r.clusterOperator)
			return nil
		}
	}

	// If we land here there is no clusteroperator object for SRO, create it.
	log = r.Log.WithName(color.Print("status", color.Blue))
	log.Info("No ClusterOperator found... Creating ClusterOperator for SRO")

	co := &configv1.ClusterOperator{ObjectMeta: metav1.ObjectMeta{Name: r.GetName()}}

	co, err = r.ClusterOperators().Create(context.TODO(), co, metav1.CreateOptions{})
	if err != nil {
		return errs.Wrap(err, "Failed to create ClusterOperator "+co.Name)
	}
	co.DeepCopyInto(&r.clusterOperator)
	return nil
}

func (r *SpecialResourceReconciler) clusterOperatorStatusSet() error {

	if releaseVersion := os.Getenv("RELEASE_VERSION"); len(releaseVersion) > 0 {
		operatorv1helpers.SetOperandVersion(&r.clusterOperator.Status.Versions, configv1.OperandVersion{Name: "operator", Version: releaseVersion})
	}
	return nil
}

func (r *SpecialResourceReconciler) clusterOperatorStatusReconcile(
	conditions []configv1.ClusterOperatorStatusCondition) error {

	r.clusterOperator.Status.Conditions = conditions

	if err := r.clusterOperatorUpdateRelatedObjects(); err != nil {
		return errs.Wrap(err, "Cannot set ClusterOperator related objects")
	}

	if err := r.clusterOperatorStatusSet(); err != nil {
		return errs.Wrap(err, "Cannot update the ClusterOperator status")
	}

	if err := r.clusterOperatorStatusUpdate(); err != nil {
		return errs.Wrap(err, "Could not update ClusterOperator")
	}

	return nil
}

func (r *SpecialResourceReconciler) clusterOperatorStatusUpdate() error {

	if _, err := r.ClusterOperators().UpdateStatus(context.TODO(), &r.clusterOperator, metav1.UpdateOptions{}); err != nil {
		return err
	}
	return nil
}

func (r *SpecialResourceReconciler) clusterOperatorUpdateRelatedObjects() error {
	relatedObjects := []configv1.ObjectReference{
		{Group: "", Resource: "namespaces", Name: "openshift-special-resource-operator"},
		{Group: "sro.openshift.io", Resource: "specialresources", Name: ""},
	}

	// Get all specialresource objects
	specialresources := &srov1beta1.SpecialResourceList{}
	err := r.List(context.TODO(), specialresources, []client.ListOption{}...)
	if err != nil {
		return err
	}

	//Add namespace for each specialresource to related objects
	for _, sr := range specialresources.Items {
		if sr.Spec.Namespace != "" { // preamble specialresource has no namespace
			log.Info("Adding to relatedObjects", "namespace", sr.Spec.Namespace)
			relatedObjects = append(relatedObjects, configv1.ObjectReference{Group: "", Resource: "namespaces", Name: sr.Spec.Namespace})
		}
	}

	r.clusterOperator.Status.RelatedObjects = relatedObjects

	return nil
}

// SpecialResourcesStatusInit Depending on what error we're getting from the
// reconciliation loop we're updating the status
// nil -> All things good and default conditions can be applied
func SpecialResourcesStatus(r *SpecialResourceReconciler, req ctrl.Request, cond []configv1.ClusterOperatorStatusCondition) (ctrl.Result, error) {
	log = r.Log.WithName(color.Print("status", color.Blue))
	if err := r.clusterOperatorStatusGetOrCreate(); err != nil {
		return reconcile.Result{Requeue: true}, errs.Wrap(err, "Cannot get or create ClusterOperator")
	}

	log.Info("Reconciling ClusterOperator")
	if err := r.clusterOperatorStatusReconcile(cond); err != nil {
		return reconcile.Result{Requeue: true}, errs.Wrap(err, "Reconciling ClusterOperator failed")
	}
	return reconcile.Result{}, nil
}
