//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"io/ioutil"
	"strings"
	"sync"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	srov1beta1 "github.com/openshift-psap/special-resource-operator/api/v1beta1"
	"github.com/openshift-psap/special-resource-operator/test/framework"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	pingPongName      = "ping-pong"
	pingPongChartPath = "../../../charts/example/ping-pong-0.0.1/ping-pong.yaml"
)

var _ = ginkgo.Describe("[basic][ping-pong] Test ping-pong", func() {

	cs := framework.NewClientSet()
	cl := framework.NewControllerRuntimeClient()
	clientSet, err := GetKubeClientSet()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	ginkgo.BeforeEach(func() {
		ginkgo.By("[pre] Creating ping pong SpecialResource...")
		err := pingPongCreate(cs, cl)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		ginkgo.By("[pre] Waiting ping pong pods to be ready...")
		err = waitPingPongReady(clientSet)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

	ginkgo.AfterEach(func() {
		ginkgo.By("[post] Deleting ping-pong SpecialResource...")
		err := pingPongDelete(cs, cl)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		err = waitPingPongDeleted(cl)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

	ginkgo.Context("when installed", func() {
		ginkgo.It("Check logs", func() {
			err := checkPingPong(clientSet, cs, cl)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})
	})
})

func getPodLogs(clientSet *kubernetes.Clientset, namespace string, podName string) (string, error) {
	count := int64(10)
	podLogOptions := v1.PodLogOptions{
		Follow:    false,
		TailLines: &count,
	}

	podLogRequest := clientSet.CoreV1().
		Pods(namespace).
		GetLogs(podName, &podLogOptions)
	stream, err := podLogRequest.Stream(context.Background())
	if err != nil {
		return "", err
	}
	defer stream.Close()

	buf := make([]byte, 2000)
	numBytes, err := stream.Read(buf)
	if err != nil {
		return "", err
	}
	if numBytes == 0 {
		return "", nil
	}
	message := string(buf[:numBytes])

	return message, nil
}

func checkPingPong(clientSet *kubernetes.Clientset, cs *framework.ClientSet, cl client.Client) error {
	pods, err := cs.Pods(pingPongName).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	err = wait.PollImmediate(pollInterval, waitDuration, func() (bool, error) {
		found := 0
		for _, pod := range pods.Items {
			log, err := getPodLogs(clientSet, pod.Namespace, pod.Name)
			if err != nil {
				return false, err
			}
			if (strings.Contains(log, "Sending: Ping") || strings.Contains(log, "Sending: Pong")) && (strings.Contains(log, "Received: Ping") || strings.Contains(log, "Received: Pong")) {
				found++
			}
		}
		if found == 2 {
			return true, nil
		}
		return false, nil
	})
	return err
}

func pingPongDelete(cs *framework.ClientSet, cl client.Client) error {
	sr, err := ioutil.ReadFile(pingPongChartPath)
	if err != nil {
		return err
	}
	return framework.DeleteFromYAMLWithCR(sr, cl)
}

func waitPingPongDeleted(cl client.Client) error {
	err := wait.PollImmediate(pollInterval, waitDuration, func() (bool, error) {
		specialresources := &srov1beta1.SpecialResourceList{}
		err := cl.List(context.Background(), specialresources, []client.ListOption{}...)
		if err != nil {
			return false, err
		}
		for _, n := range specialresources.Items {
			if n.ObjectMeta.Name == pingPongName {
				return false, nil
			}
		}
		return true, nil

	})
	return err
}

func waitPingPongReady(clientSet *kubernetes.Clientset) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	watch, err := clientSet.CoreV1().Pods(pingPongName).Watch(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		for {
			select {
			case event, ok := <-watch.ResultChan():
				if !ok {
					return
				}
				p, ok := event.Object.(*v1.Pod)
				if !ok {
					continue
				}
				if p.Status.Phase == v1.PodRunning {
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

func pingPongCreate(cs *framework.ClientSet, cl client.Client) error {
	sr, err := ioutil.ReadFile(pingPongChartPath)
	if err != nil {
		return err
	}
	return framework.CreateFromYAML(sr, cl)
}
