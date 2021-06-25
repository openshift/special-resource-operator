package e2e

import (
	"context"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"github.com/openshift-psap/special-resource-operator/test/framework"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
)

var explain string

var nodeKernelFullVersion string
var nodeOSVersion string
var nodeOCPVersion string

var simpleKmodCrYAML []byte

const (
	pollInterval = 10 * time.Second
	waitDuration = 60 * time.Minute
)

func NFDGetVersionTriple(cs *framework.ClientSet) {

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

}

func simpleKmodCreate(cs *framework.ClientSet, cl client.Client) {

	var err error

	simpleKmodCrYAML, err = ioutil.ReadFile("../../../charts/example/simple-kmod-0.0.1/simple-kmod.yaml")
	if err != nil {
		panic(err)
	}
	framework.CreateFromYAML(simpleKmodCrYAML, cl)

	/*
		ginkgo.By("waiting for completion driver-container-base build")
		err = wait.PollImmediate(pollInterval, waitDuration, func() (bool, error) {

			dcbPods, err := cs.Pods("driver-container-base").List(context.TODO(), metav1.ListOptions{})
			if err != nil {
				return false, fmt.Errorf("Error getting list of pods, %v", err)
			}

			if len(dcbPods.Items) < 1 {
				return false, nil
			}

			var sum = len(dcbPods.Items)
			for _, pod := range dcbPods.Items {
				// Get logs of driverContainerBase
				_ = GetRecentPodLogs(pod.GetName(), pod.GetNamespace(), pollInterval)
				if pod.Status.Phase == "Succeeded" {
					sum = sum - 1
				}
			}

			if sum == 0 {
				return true, nil
			}

			return false, nil
		})
		if err != nil {
			explain = err.Error()
		}
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), explain)
	*/

	var dss *v1.DaemonSetList

	for {
		time.Sleep(10 * time.Second)

		ginkgo.By("Waiting for simple-kmod daemonset to be ready")
		dss, err = cs.DaemonSets("simple-kmod").List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			_, _ = Logf("Looking for DaemonSet", err)
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), explain)
		}

		if len(dss.Items) > 0 {
			break
		}
	}

	for _, ds := range dss.Items {
		err = WaitForDaemonsetReady(cs, pollInterval, waitDuration, "simple-kmod", ds.Name)
		if err != nil {
			explain = err.Error()
		}
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), explain)
	}

}

func simpleKmodModulesReady(cs *framework.ClientSet) {

	// Now check if module is actually running
	dss, err := cs.DaemonSets("simple-kmod").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		_, _ = Logf("Looking for DaemonSet", err)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), explain)
	}

	for _, ds := range dss.Items {

		//get driver container pod on worker node
		ginkgo.By(fmt.Sprintf("getting a simple-kmod-driver-container Pod in DS %s", ds.Name))

		opts := metav1.ListOptions{ //TODO fix
			LabelSelector: labels.SelectorFromSet(labels.Set{"app": ds.Name}).String(),
		}
		pods, err := cs.Pods("simple-kmod").List(context.TODO(), opts)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		for _, pod := range pods.Items {
			//run command in pod
			ginkgo.By("Ensuring that the simple-kmod is loaded")
			lsmodCmd := []string{"/bin/sh", "-c", "lsmod | grep -o simple_kmod"}
			valExp := "simple_kmod"
			_, err = WaitForCmdOutputInPod(pollInterval, waitDuration, &pod, &valExp, true, lsmodCmd...)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

		}

	}

}

func simpleKmodDelete(cs *framework.ClientSet, cl client.Client) {

	ginkgo.By("deleting simple-kmod")
	framework.DeleteFromYAMLWithCR(simpleKmodCrYAML, cl)
	err := wait.PollImmediate(pollInterval, waitDuration, func() (bool, error) {
		namespaces, err := cs.Namespaces().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return false, fmt.Errorf("couldn't get namespaces: %v", err)
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

	nodes, err := GetNodesByRole(cs, "worker")
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), explain)

	for _, node := range nodes {
		//run command in pod
		ginkgo.By("Ensuring that the simple-kmod is unloaded")
		// || true at the end of grep command because we don't want grep to exit with an error code if no matches are found.
		unloadCmd := []string{"/bin/sh", "-c", "/host/usr/sbin/lsmod | grep -c simple-kmod || true"}
		unloadValExp := "0"
		_, err = WaitForCmdOutputInDebugPod(pollInterval, waitDuration, node.Name, &unloadValExp, true, unloadCmd...)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

	}
}

var _ = ginkgo.Describe("[basic][simple-kmod] create and deploy simple-kmod", func() {

	cs := framework.NewClientSet()
	cl := framework.NewControllerRuntimeClient()

	// Check that operator deployment has 1 available pod
	ginkgo.It("Can create driver-container-base and deploy simple-kmod", func() {

		ginkgo.By("Fetching version triplet labels")
		NFDGetVersionTriple(cs)

		ginkgo.By("Creating simple-kmod #1")
		simpleKmodCreate(cs, cl)
		ginkgo.By("Checking modules ready on Nodes #1")
		simpleKmodModulesReady(cs)
		ginkgo.By("Deleting simple-kmod #1")
		simpleKmodDelete(cs, cl)

		ginkgo.By("Creating simple-kmod #2")
		simpleKmodCreate(cs, cl)
		ginkgo.By("Checking modules ready on Nodes #2")
		simpleKmodModulesReady(cs)
		ginkgo.By("Deleting simple-kmod #2")
		simpleKmodDelete(cs, cl)

	})

})
