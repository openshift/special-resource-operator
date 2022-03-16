package state_test

import (
	"context"
	"strings"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift-psap/special-resource-operator/api/v1beta1"
	"github.com/openshift-psap/special-resource-operator/internal/controllers/state"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type legacyStatusMatcher struct {
	expectedSubstring string
}

func (l legacyStatusMatcher) Matches(arg interface{}) bool {
	sr := arg.(*v1beta1.SpecialResource)

	return strings.Contains(sr.Status.State, l.expectedSubstring)
}

func (l legacyStatusMatcher) String() string {
	return l.expectedSubstring
}

type conditionExclusivityMatcher struct {
	onlyConditionToBeTrue string
}

func (c conditionExclusivityMatcher) Matches(arg interface{}) bool {
	sr := arg.(*v1beta1.SpecialResource)

	for _, cond := range sr.Status.Conditions {
		if cond.Type == c.onlyConditionToBeTrue {
			if cond.Status != metav1.ConditionTrue {
				return false
			}
		} else {
			if cond.Status == metav1.ConditionTrue {
				return false
			}
		}
	}

	return true
}

func (c conditionExclusivityMatcher) String() string {
	return c.onlyConditionToBeTrue
}

var _ = Describe("SetAs{Ready,Progressing,Errored}", func() {
	const (
		name      = "sr-name"
		namespace = "sr-namespace"
	)

	var (
		kubeClient *clients.MockClientsInterface
		sr         *v1beta1.SpecialResource
	)

	BeforeEach(func() {
		ctrl := gomock.NewController(GinkgoT())
		kubeClient = clients.NewMockClientsInterface(ctrl)
		sr = &v1beta1.SpecialResource{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
	})

	DescribeTable("Setting one condition to true, should set others to false",
		func(expectedType string, call func(state.StatusUpdater) error) {
			gomock.InOrder(
				kubeClient.EXPECT().
					StatusUpdate(context.TODO(), gomock.All(conditionExclusivityMatcher{expectedType}, legacyStatusMatcher{expectedType})).
					Return(nil),
			)

			Expect(call(state.NewStatusUpdater(kubeClient))).To(Succeed())

			// Make sure Conditions are set for object that was passed in and visible outside
			Expect(sr.Status.Conditions).To(HaveLen(3))
		},
		Entry("Ready",
			v1beta1.SpecialResourceReady,
			func(su state.StatusUpdater) error { return su.SetAsReady(context.Background(), sr, "x", "x") },
		),
		Entry("Errored",
			v1beta1.SpecialResourceErrored,
			func(su state.StatusUpdater) error { return su.SetAsErrored(context.Background(), sr, "x", "x") },
		),
		Entry("Progressing",
			v1beta1.SpecialResourceProgressing,
			func(su state.StatusUpdater) error { return su.SetAsProgressing(context.Background(), sr, "x", "x") },
		),
	)
})
