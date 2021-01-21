package e2e

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"github.com/openshift-psap/special-resource-operator/test/framework"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	configv1 "github.com/openshift/api/config/v1"
	//srov1beta1 "github.com/openshift-psap/special-resource-operator/api/v1beta1"
)

var _ = ginkgo.Describe("[basic][available] Special Resource Operator availability", func() {
	const (
		pollInterval = 5 * time.Second
		waitDuration = 5 * time.Minute
	)

	cs := framework.NewClientSet()

	var explain string

	// Check that operator deployment has 1 available pod
	ginkgo.It(fmt.Sprintf("Operator pod is running"), func() {
		ginkgo.By(fmt.Sprintf("Wait for deployment/special-resource-controller-manager to have 1 ready replica"))
		err := wait.PollImmediate(pollInterval, waitDuration, func() (bool, error) {
			operatorDeployment, err := cs.Deployments("openshift-special-resource-operator").Get(context.TODO(), "special-resource-controller-manager", metav1.GetOptions{})
			if err != nil {
				return false, fmt.Errorf("Couldn't get operator deployment %v", err)
			}

			if operatorDeployment.Status.ReadyReplicas == 1 {
				return true, nil
			}
			return false, nil
		})
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), explain)
	})

	// Check that operator is reporting status to ClusterOperator
	ginkgo.It(fmt.Sprintf("clusteroperator/special-resource-operator available and not degraded"), func() {
		ginkgo.By(fmt.Sprintf("wait for clusteroperator/special-resource-operator available"))
		err := WaitForClusterOperatorCondition(cs, pollInterval, waitDuration, configv1.OperatorAvailable, configv1.ConditionTrue)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), explain)

		ginkgo.By(fmt.Sprintf("wait for clusteroperator/special-resource-operator not degraded"))
		err = WaitForClusterOperatorCondition(cs, pollInterval, waitDuration, configv1.OperatorDegraded, configv1.ConditionFalse)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), explain)
	})

	// TODO Check that operator is setting the upgradeable condition
})
