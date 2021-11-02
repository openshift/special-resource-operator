package conditions

import (
	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/library-go/pkg/config/clusteroperator/v1helpers"
	"github.com/stretchr/testify/assert"
	"testing"
)

type conditionTemplate struct {
	condType configv1.ClusterStatusConditionType
	status   configv1.ConditionStatus
	reason   string
	message  string
}

func findAndCompareCondition(
	t *testing.T,
	conditions []configv1.ClusterOperatorStatusCondition,
	expected conditionTemplate) {
	t.Helper()

	cond := v1helpers.FindStatusCondition(conditions, expected.condType)

	assert.NotNil(t, cond)
	assert.Equal(t, expected.status, cond.Status)
	assert.Equal(t, expected.reason, cond.Reason)
	assert.Equal(t, expected.message, cond.Message)
	assert.False(t, cond.LastTransitionTime.IsZero())
}

func TestAvailableNotProgressingNotDegraded(t *testing.T) {
	conds := AvailableNotProgressingNotDegraded()

	cases := []conditionTemplate{
		{
			condType: configv1.OperatorAvailable,
			status:   configv1.ConditionTrue,
			reason:   "AsExpected",
			message:  "Reconciled all SpecialResources",
		},
		{
			condType: configv1.OperatorProgressing,
			status:   configv1.ConditionFalse,
			reason:   "Reconciled",
			message:  "SpecialResources up to date",
		},
		{
			condType: configv1.OperatorDegraded,
			status:   configv1.ConditionFalse,
			reason:   "AsExpected",
			message:  "Special Resource Operator reconciling special resources",
		},
	}

	for _, c := range cases {
		t.Run(string(c.condType), func(t *testing.T) {
			findAndCompareCondition(t, conds, c)
		})
	}
}

func TestNotAvailableProgressingNotDegraded(t *testing.T) {
	const (
		msgAvailable   = "some-msg-available"
		msgProgressing = "some-msg-progressing"
		msgDegraded    = "some-msg-degraded"
	)

	conds := NotAvailableProgressingNotDegraded(msgAvailable, msgProgressing, msgDegraded)

	cases := []conditionTemplate{
		{
			condType: configv1.OperatorAvailable,
			status:   configv1.ConditionFalse,
			reason:   "Reconciling",
			message:  msgAvailable,
		},
		{
			condType: configv1.OperatorProgressing,
			status:   configv1.ConditionTrue,
			reason:   "Reconciling",
			message:  msgProgressing,
		},
		{
			condType: configv1.OperatorDegraded,
			status:   configv1.ConditionFalse,
			reason:   "Reconciled",
			message:  msgDegraded,
		},
	}

	for _, c := range cases {
		t.Run(string(c.condType), func(t *testing.T) {
			findAndCompareCondition(t, conds, c)
		})
	}
}
