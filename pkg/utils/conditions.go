package utils

import (
	configv1 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DegradedDefaultMsg DegradedDefaultMsg
const DegradedDefaultMsg string = "Special Resource Operator reconciling special resources"

// AvailableNotProgressingNotDegraded AvailableNotProgressingNotDegraded
func AvailableNotProgressingNotDegraded() []configv1.ClusterOperatorStatusCondition {
	return []configv1.ClusterOperatorStatusCondition{
		{
			Type:               configv1.OperatorAvailable,
			Status:             configv1.ConditionTrue,
			Reason:             "AsExpected",
			Message:            "Reconciled all SpecialResources",
			LastTransitionTime: metav1.Now(),
		},
		{
			Type:               configv1.OperatorProgressing,
			Status:             configv1.ConditionFalse,
			Reason:             "Reconciled",
			Message:            "SpecialResources up to date",
			LastTransitionTime: metav1.Now(),
		},
		{
			Type:               configv1.OperatorDegraded,
			Status:             configv1.ConditionFalse,
			Reason:             "AsExpected",
			Message:            DegradedDefaultMsg,
			LastTransitionTime: metav1.Now(),
		},
	}
}

// NotAvailableProgressingNotDegraded NotAvailableProgressingNotDegraded
func NotAvailableProgressingNotDegraded(
	msgAvailable string,
	msgProgressing string,
	msgDegradded string) []configv1.ClusterOperatorStatusCondition {

	return []configv1.ClusterOperatorStatusCondition{
		{
			Type:               configv1.OperatorAvailable,
			Status:             configv1.ConditionFalse,
			Reason:             "Reconciling",
			Message:            msgAvailable,
			LastTransitionTime: metav1.Now(),
		},
		{
			Type:               configv1.OperatorProgressing,
			Status:             configv1.ConditionTrue,
			Reason:             "Reconciling",
			Message:            msgProgressing,
			LastTransitionTime: metav1.Now(),
		},
		{
			Type:               configv1.OperatorDegraded,
			Status:             configv1.ConditionFalse,
			Reason:             "Reconciled",
			Message:            msgDegradded,
			LastTransitionTime: metav1.Now(),
		},
	}
}
