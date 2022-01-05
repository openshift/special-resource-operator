package utils

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/library-go/pkg/config/clusteroperator/v1helpers"
)

type conditionTemplate struct {
	condType configv1.ClusterStatusConditionType
	status   configv1.ConditionStatus
	reason   string
	message  string
}

func findAndCompareCondition(conditions []configv1.ClusterOperatorStatusCondition, expected conditionTemplate) {
	cond := v1helpers.FindStatusCondition(conditions, expected.condType)

	Expect(cond).NotTo(BeNil())
	Expect(cond.Status).To(Equal(expected.status))
	Expect(cond.Reason).To(Equal(expected.reason))
	Expect(cond.Message).To(Equal(expected.message))
	Expect(cond.LastTransitionTime).NotTo(BeZero())
}

var _ = Describe("Conditions", func() {
	Context("AvailableNotProgressingNotDegraded", func() {
		templates := []conditionTemplate{
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

		entries := make([]TableEntry, 0, len(templates))

		for _, ct := range templates {
			entries = append(entries, Entry(ct.condType, ct))
		}

		conds := AvailableNotProgressingNotDegraded()

		DescribeTable(
			"all conditions",
			func(ct conditionTemplate) {
				findAndCompareCondition(conds, ct)
			},
			entries...,
		)
	})

	Context("NotAvailableProgressingNotDegraded", func() {
		const (
			msgAvailable   = "some-msg-available"
			msgProgressing = "some-msg-progressing"
			msgDegraded    = "some-msg-degraded"
		)

		conds := NotAvailableProgressingNotDegraded(msgAvailable, msgProgressing, msgDegraded)

		templates := []conditionTemplate{
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

		entries := make([]TableEntry, 0, len(templates))

		for _, ct := range templates {
			entries = append(entries, Entry(ct.condType, ct))
		}

		DescribeTable(
			"all conditions",
			func(ct conditionTemplate) {
				findAndCompareCondition(conds, ct)
			},
			entries...,
		)
	})
})
