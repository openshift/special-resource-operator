package e2e

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	srov1beta1 "github.com/openshift-psap/special-resource-operator/api/v1beta1"
	"github.com/openshift-psap/special-resource-operator/pkg/warn"
	"github.com/openshift-psap/special-resource-operator/test/framework"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var clientSet kubernetes.Clientset

var _ = ginkgo.Describe("[basic][ping-pong] create and deploy ping-poing", func() {

	cs := framework.NewClientSet()
	cl := framework.NewControllerRuntimeClient()
	ginkgo.It("Can create and deploy ping-pong", func() {
		ginkgo.By("Creating ping-pong #1")
		specialResourceCreate(cs, cl, "../../../charts/example/ping-pong-0.0.1/ping-pong.yaml")
		checkPingPong(cs, cl)
		specialResourceDelete(cs, cl, "../../../charts/example/ping-pong-0.0.1/ping-pong.yaml")
	})
})

func GetPodLogs(namespace string, podName string, containerName string, follow bool) (string, error) {
	count := int64(100)
	podLogOptions := v1.PodLogOptions{
		//		Container: containerName,
		Follow:    follow,
		TailLines: &count,
	}

	podLogRequest := clientSet.CoreV1().
		Pods(namespace).
		GetLogs(podName, &podLogOptions)
	stream, err := podLogRequest.Stream(context.TODO())
	if err != nil {
		return "", err
	}
	defer stream.Close()

	var message string

	buf := make([]byte, 2000)
	numBytes, err := stream.Read(buf)
	if numBytes == 0 {
		return "", errors.New("Nothing read, returngin")
	}
	if err == io.EOF {
		return "", errors.New("EOF")
	}
	if err != nil {
		return "", err
	}
	message = string(buf[:numBytes])
	fmt.Print(message)

	return message, nil
}

func checkPingPong(cs *framework.ClientSet, cl client.Client) {

	clientSet = GetKubeClientSetOrDie()

	for {
		time.Sleep(60 * time.Second)

		ginkgo.By("Waiting for ping-pong Pods to be ready")
		opts := metav1.ListOptions{}
		pods, err := cs.Pods("ping-pong").List(context.TODO(), opts)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		for _, pod := range pods.Items {
			var log string
			//run command in pod
			ginkgo.By("Ensuring that ping-pong is working")
			if pod.Status.Phase == v1.PodRunning {
				log, err = GetPodLogs(pod.Namespace, pod.Name, "", false)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				if !strings.Contains(log, "Ping") || !strings.Contains(log, "Pong") {
					warn.OnError(errors.New("Did not see Ping or either Pong, waiting"))
				}

				if strings.Contains(log, "Ping") && strings.Contains(log, "Pong") {
					ginkgo.By("Found Ping, Pong in logs, done")
					return
				}
			}
		}
	}

}

func specialResourceDelete(cs *framework.ClientSet, cl client.Client, path string) {
	SRName := "ping-pong"
	ginkgo.By("deleting " + SRName)
	sr, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}
	framework.DeleteFromYAMLWithCR(sr, cl)

	ginkgo.By(fmt.Sprintf("Confirming %s is deleted", SRName))
	err = wait.PollImmediate(pollInterval, waitDuration, func() (bool, error) {
		specialresources := &srov1beta1.SpecialResourceList{}
		err = cl.List(context.TODO(), specialresources, []client.ListOption{}...)
		if err != nil {
			return false, fmt.Errorf("couldn't get SpecialResourceList: %v", err)
		}

		for _, n := range specialresources.Items {
			if n.ObjectMeta.Name == SRName {
				return false, nil
			}
		}

		return true, nil

	})
	if err != nil {
		explain = err.Error()
	}
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), explain)
}

func specialResourceCreate(cs *framework.ClientSet, cl client.Client, path string) {

	ginkgo.By("creating ping-pong")
	sr, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}
	framework.CreateFromYAML(sr, cl)
}

// GetKubeClientSetOrDie Add a native non-caching client for advanced CRUD operations
func GetKubeClientSetOrDie() kubernetes.Clientset {

	kubeconfig := os.Getenv("KUBERNETES_CONFIG")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	clientSet, err := kubernetes.NewForConfig(config)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return *clientSet
}
