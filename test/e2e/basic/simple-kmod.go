package e2e

import (
	"context"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"github.com/openshift-psap/special-resource-operator/pkg/hash"
	"github.com/openshift-psap/special-resource-operator/test/framework"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
)

var _ = ginkgo.Describe("[basic][simple-kmod] create and deploy simple-kmod", func() {
	const (
		pollInterval = 10 * time.Second
		waitDuration = 15 * time.Minute
	)

	cs := framework.NewClientSet()
	cl := framework.NewControllerRuntimeClient()

	var explain string

	var nodeKernelFullVersion string
	var nodeOSVersion string
	var nodeOCPVersion string

	// Check that operator deployment has 1 available pod
	ginkgo.It("Can create driver-container-base and deploy simple-kmod", func() {

		ginkgo.By("Checking if NFD is running, getting kernel, os, ocp versions")
		var err error
		nodeKernelFullVersion, nodeOSVersion, nodeOCPVersion, err = GetVersionTriplet(cs)
		_, _ = Logf("Info: KernelVersion: " + nodeKernelFullVersion)
		_, _ = Logf("Info: OSVersion: " + nodeOSVersion)
		_, _ = Logf("Info: OpenShift Version: " + nodeOCPVersion)

		if err != nil {
			explain = err.Error()
		}
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), explain)

		kernelVersion := strings.ReplaceAll(nodeKernelFullVersion, "_", "-")
		hash64 := hash.FNV64a(nodeOSVersion + "-" + kernelVersion)
		_, _ = Logf("Info: hash64 for object names: " + hash64)

		simpleKmodCrYAML, err := ioutil.ReadFile("../../../config/recipes/simple-kmod/0000-simple-kmod-cr.yaml")
		if err != nil {
			panic(err)
		}
		framework.CreateFromYAML(simpleKmodCrYAML, cl)

		ginkgo.By("waiting for completion driver-container-base build")
		err = wait.PollImmediate(pollInterval, waitDuration, func() (bool, error) {

			dcbPods, err := cs.Pods("driver-container-base").List(context.TODO(), metav1.ListOptions{})
			if err != nil {
				return false, fmt.Errorf("Error getting list of pods, %v", err)
			}

			if len(dcbPods.Items) < 1 {
				return false, nil
			}

			driverContainerBase, err := cs.Pods("driver-container-base").Get(context.TODO(), "driver-container-base"+"-"+hash64, metav1.GetOptions{})
			if err != nil {
				return false, fmt.Errorf("Couldn't get driver-container-base pod, %v", err)
			}

			// Get logs of driverContainerBase
			_ = GetRecentPodLogs(driverContainerBase.GetName(), driverContainerBase.GetNamespace(), pollInterval)

			if driverContainerBase.Status.Phase == "Succeeded" {
				return true, nil
			}

			return false, nil
		})
		if err != nil {
			explain = err.Error()
		}
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), explain)

		ginkgo.By("waiting for simple-kmod daemonset to be ready")
		err = WaitForDaemonsetReady(cs, pollInterval, waitDuration, "simple-kmod", "simple-kmod-driver-container-"+hash64)
		if err != nil {
			explain = err.Error()
		}
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), explain)

		// Now check if module is actually running
		ginkgo.By("getting a list of worker nodes")
		nodes, err := GetNodesByRole(cs, "worker")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(len(nodes)).NotTo(gomega.BeZero(), "number of worker nodes is 0")
		workerNode := nodes[0]

		//get driver container pod on worker node
		ginkgo.By(fmt.Sprintf("getting a simple-kmod-driver-container Pod running on node %s", workerNode.Name))
		simpleKmodPodListOptions := metav1.ListOptions{ //TODO fix
			FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": workerNode.Name}).String(),
			LabelSelector: labels.SelectorFromSet(labels.Set{"app": "simple-kmod-driver-container-" + hash64}).String(),
		}

		pod, err := GetPodForNode(cs, &workerNode, "simple-kmod", simpleKmodPodListOptions)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		//run command in pod
		ginkgo.By("Ensuring that the simple-kmod is loaded")
		lsmodCmd := []string{"/bin/sh", "-c", "lsmod | grep -o simple_kmod"}
		valExp := "simple_kmod"
		_, err = WaitForCmdOutputInPod(pollInterval, waitDuration, pod, &valExp, true, lsmodCmd...)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By("deleting simple-kmod")
		framework.DeleteFromYAMLWithCR(simpleKmodCrYAML, cl)
		err = wait.PollImmediate(pollInterval, waitDuration, func() (bool, error) {
			namespaces, err := cs.Namespaces().List(context.TODO(), metav1.ListOptions{})
			if err != nil {
				return false, fmt.Errorf("Couldn't get namespaces: %v", err)
			}

			for _, n := range namespaces.Items {
				if n.ObjectMeta.Name == "simple-kmod" {
					return false, nil
				}
			}

			return true, nil

		})
		if err != nil {
			explain = err.Error()
		}
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), explain)

		ginkgo.By("Creating simple-kmod again")
		framework.CreateFromYAML(simpleKmodCrYAML, cl)

		ginkgo.By("waiting for simple-kmod daemonset to be ready")
		err = WaitForDaemonsetReady(cs, pollInterval, waitDuration, "simple-kmod", "simple-kmod-driver-container-"+hash64)
		if err != nil {
			explain = err.Error()
		}
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), explain)

		// Now check if module is actually running
		//get driver container pod on worker node
		ginkgo.By(fmt.Sprintf("getting a simple-kmod-driver-container Pod running on node %s", workerNode.Name))

		pod, err = GetPodForNode(cs, &workerNode, "simple-kmod", simpleKmodPodListOptions)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		//run command in pod
		ginkgo.By("Ensuring that the simple-kmod is loaded")
		_, err = WaitForCmdOutputInPod(pollInterval, waitDuration, pod, &valExp, true, lsmodCmd...)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By("deleting simple-kmod")
		framework.DeleteFromYAMLWithCR(simpleKmodCrYAML, cl)
		err = wait.PollImmediate(pollInterval, waitDuration, func() (bool, error) {
			namespaces, err := cs.Namespaces().List(context.TODO(), metav1.ListOptions{})
			if err != nil {
				return false, fmt.Errorf("Couldn't get namespaces: %v", err)
			}

			for _, n := range namespaces.Items {
				if n.ObjectMeta.Name == "simple-kmod" {
					return false, nil
				}
			}

			return true, nil

		})
		if err != nil {
			explain = err.Error()
		}
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), explain)

	})

})
