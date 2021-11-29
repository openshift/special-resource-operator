//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"io/ioutil"
	"sync"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"github.com/openshift-psap/special-resource-operator/test/framework"
	"sigs.k8s.io/controller-runtime/pkg/client"

	srov1beta1 "github.com/openshift-psap/special-resource-operator/api/v1beta1"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

const (
	simpleKmodName      = "simple-kmod"
	simpleKmodChartPath = "../../../charts/example/simple-kmod-0.0.1/simple-kmod.yaml"
)

func createSimpleKmod(cl client.Client) error {
	simpleKmodCrYAML, err := ioutil.ReadFile(simpleKmodChartPath)
	if err != nil {
		return err
	}
	return framework.CreateFromYAML(simpleKmodCrYAML, cl)
}

func deleteSimpleKmod(cl client.Client) error {
	simpleKmodCrYAML, err := ioutil.ReadFile(simpleKmodChartPath)
	if err != nil {
		return err
	}
	return framework.DeleteFromYAMLWithCR(simpleKmodCrYAML, cl)
}

func waitImageReady(clientSet *kubernetes.Clientset) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	watch, err := clientSet.CoreV1().Pods(simpleKmodName).Watch(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		for {
			select {
			case event, ok := <-watch.ResultChan():
				if !ok {
					return
				}
				p, ok := event.Object.(*corev1.Pod)
				if !ok {
					continue
				}
				if _, ok := p.GetAnnotations()["openshift.io/build.name"]; !ok {
					continue
				}
				if p.Status.Phase == corev1.PodSucceeded {
					wg.Done()
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	wg.Wait()
	return nil
}

func deleteDaemonSetPods(clientSet *kubernetes.Clientset) error {
	// dont wait for backoff
	// Sometimes the DS is created when the build is still ongoing, triggering image pull backoff. There are
	// situations where the build has just ended right after the backoff triggered another pull retry. If
	// this happens then the test will take longer, so we preemptively delete all pods so they get rescheduled.
	list, err := clientSet.CoreV1().Pods(simpleKmodName).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, pod := range list.Items {
		if pod.Status.Phase != corev1.PodSucceeded && pod.Status.Phase != corev1.PodRunning {
			err = clientSet.CoreV1().Pods(simpleKmodName).Delete(context.Background(), pod.Name, metav1.DeleteOptions{})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func waitDaemonSetReady(cs *framework.ClientSet) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	watch, err := cs.DaemonSets(simpleKmodName).Watch(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		for {
			select {
			case event, ok := <-watch.ResultChan():
				if !ok {
					return
				}
				ds, ok := event.Object.(*v1.DaemonSet)
				if !ok {
					continue
				}
				if ds.Status.DesiredNumberScheduled > 0 && ds.Status.DesiredNumberScheduled == ds.Status.NumberAvailable {
					wg.Done()
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	wg.Wait()
	return nil
}

func waitSimpleKmodDeleted(cs *framework.ClientSet, cl client.Client) error {
	err := wait.PollImmediate(pollInterval, waitDuration, func() (bool, error) {
		specialresources := &srov1beta1.SpecialResourceList{}
		err := cl.List(context.Background(), specialresources, []client.ListOption{}...)
		if err != nil {
			return false, err
		}
		for _, element := range specialresources.Items {
			if element.Name == simpleKmodName {
				return false, nil
			}
		}
		return true, nil
	})
	return err
}

func checkModuleLoaded(cs *framework.ClientSet) error {
	nodes, err := GetNodesByRole(cs, "worker")
	if err != nil {
		return err
	}
	for _, node := range nodes {
		if !IsNodeReady(node) {
			continue
		}
		unloadCmd := []string{"/bin/sh", "-c", "/host/usr/sbin/lsmod | grep -o simple_kmod"}
		valExp := "simple_kmod"
		_, err = WaitForCmdOutputInNode(pollInterval, waitDuration, node.Name, &valExp, true, unloadCmd...)
		if err != nil {
			return err
		}
	}
	return nil
}

func checkModuleUnloaded(cs *framework.ClientSet) error {
	nodes, err := GetNodesByRole(cs, "worker")
	if err != nil {
		return err
	}
	for _, node := range nodes {
		if !IsNodeReady(node) {
			continue
		}
		unloadCmd := []string{"/bin/sh", "-c", "/host/usr/sbin/lsmod | grep -c simple-kmod || true"}
		unloadValExp := "0"
		_, err = WaitForCmdOutputInNode(pollInterval, waitDuration, node.Name, &unloadValExp, true, unloadCmd...)
		if err != nil {
			return err
		}

	}
	return nil
}

var _ = ginkgo.Describe("[basic][simple-kmod] create and deploy simple-kmod", func() {

	cs := framework.NewClientSet()
	cl := framework.NewControllerRuntimeClient()
	clientSet, err := GetKubeClientSet()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	ginkgo.BeforeEach(func() {
		ginkgo.By("[pre] Creating SpecialResource...")
		err := createSimpleKmod(cl)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		ginkgo.By("[pre] Waiting build container to complete...")
		err = waitImageReady(clientSet)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		ginkgo.By("[pre] Deleting pods to avoid backoff delay...")
		err = deleteDaemonSetPods(clientSet)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		ginkgo.By("[pre] Waiting DaemonSet pods to be ready...")
		err = waitDaemonSetReady(cs)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

	ginkgo.AfterEach(func() {
		ginkgo.By("[post] Deleting SpecialResource...")
		err := deleteSimpleKmod(cl)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		ginkgo.By("[post] Waiting SpecialResource deletion...")
		err = waitSimpleKmodDeleted(cs, cl)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		ginkgo.By("[post] Checking module is unloaded from nodes...")
		err = checkModuleUnloaded(cs)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

	ginkgo.Context("when installed", func() {
		ginkgo.It("Check module is loaded in the nodes", func() {
			ginkgo.By("Checking module in nodes...")
			err := checkModuleLoaded(cs)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})
	})
})
