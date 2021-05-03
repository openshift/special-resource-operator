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
		pollInterval = 10 * time.Second
		waitDuration = 5 * time.Minute
	)

	cs := framework.NewClientSet()

	var explain error

	// Check that operator deployment has 1 available pod
	ginkgo.It("Operator pod is running", func() {
		ginkgo.By("Wait for deployment/special-resource-controller-manager to have 1 ready replica")
		err := wait.PollImmediate(pollInterval, waitDuration, func() (bool, error) {
			deployments, err := cs.Deployments("openshift-special-resource-operator").List(context.TODO(), metav1.ListOptions{})
			if err != nil {
				return false, fmt.Errorf("Error getting list of deployments, %v", err)
			}

			if len(deployments.Items) < 1 {
				_, _ = Logf("Waiting for 1 deployment in openshift-special-resource-operator namespace, currently: %d", len(deployments.Items))
				return false, nil
			}

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
	ginkgo.It("clusteroperator/special-resource-operator available and not degraded", func() {
		ginkgo.By("wait for clusteroperator/special-resource-operator available")
		err := WaitForClusterOperatorCondition(cs, pollInterval, waitDuration, configv1.OperatorAvailable, configv1.ConditionTrue)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), explain)

		ginkgo.By("wait for clusteroperator/special-resource-operator not degraded")
		err = WaitForClusterOperatorCondition(cs, pollInterval, waitDuration, configv1.OperatorDegraded, configv1.ConditionFalse)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), explain)

		ginkgo.By("verify clusteroperator has the operator namespace in relatedObjects")
		err = wait.PollImmediate(pollInterval, waitDuration, func() (bool, error) {
			co, err := cs.ClusterOperators().Get(context.TODO(), "special-resource-operator", metav1.GetOptions{})
			if err != nil {
				explain = err
				return false, nil
			}

			for _, relatedObject := range co.Status.RelatedObjects {
				if relatedObject.Resource == "namespaces" &&
					relatedObject.Name == "openshift-special-resource-operator" {
					return true, nil
				}
			}

			return false, nil
		})
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), explain)

	})

	// TODO Check that operator is setting the upgradeable condition
})
