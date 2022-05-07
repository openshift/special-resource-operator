package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/openshift/special-resource-operator/test/framework"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// GetNodesByRole returns a list of nodes that match a given role.
func GetNodesByRole(cs *framework.ClientSet, role string) ([]corev1.Node, error) {
	listOptions := metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{fmt.Sprintf("node-role.kubernetes.io/%s", role): ""}).String(),
	}
	nodeList, err := cs.Nodes().List(context.TODO(), listOptions)
	if err != nil {
		return nil, fmt.Errorf("couldn't get a list of nodes by role (%s): %v", role, err)
	}
	return nodeList.Items, nil
}

// execCommand executes command 'name' with arguments 'args' and optionally
// ('log') logs the output.  Returns captured standard output, standard error
// and the error returned.
func execCommand(name string, args ...string) (bytes.Buffer, bytes.Buffer, error) {
	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)
	cmd := exec.Command(name, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout, stderr, err
}

// waitForCmdOutputInNode runs command with arguments 'cmd' in Node 'nodename' at
// an interval 'interval' and retries for at most the duration 'duration'.
// If 'valExp' is not nil, it also expects standard output of the command with
// leading and trailing whitespace optionally ('trim') trimmed to match 'valExp'.
// The function returns the retrieved standard output and an error returned when
// running 'cmd'.  Non-nil error is also returned when standard output of 'cmd'
// did not match non-nil 'valExp' by the time duration 'duration' elapsed.
func WaitForCmdOutputInNode(interval, duration time.Duration, nodename, namespace string, valExp *string, trim bool, cmd ...string) (string, error) {
	var (
		val          string
		err, explain error
	)
	err = wait.PollImmediate(interval, duration, func() (bool, error) {
		// Run oc debug  node/nodename -- cmd...
		ocArgs := []string{"debug", "-n", namespace, "node/" + nodename, "--"}
		ocArgs = append(ocArgs, cmd...)

		stdout, stderr, err := execCommand("oc", ocArgs...)
		val = stdout.String()
		if err != nil {
			explain = fmt.Errorf("out=%s; stderr=%s, err=%s", val, stderr.String(), err.Error())
			return false, nil
		}

		if trim {
			val = strings.TrimSpace(val)
		}

		if valExp != nil && val != *valExp {
			return false, nil
		}
		return true, nil
	})
	if valExp != nil && val != *valExp {
		return val, fmt.Errorf("command %s outputs (leading/trailing whitespace trimmed) %s in debug pod on %s, expected %s: %v", cmd, val, nodename, *valExp, explain)
	}

	return val, err
}

// WaitSRORunning blocks until SRO deployment has one running pod.
func WaitSRORunning(clientSet *kubernetes.Clientset, namespace string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	watch, err := clientSet.AppsV1().Deployments(namespace).Watch(ctx, metav1.ListOptions{})
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
				d, ok := event.Object.(*appsv1.Deployment)
				if !ok {
					continue
				}
				if d.Status.ReadyReplicas == 1 {
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

// IsNodeReady helps determine if a given node is ready or not.
func IsNodeReady(node corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

// GetKubeClientSet Add a native non-caching client for advanced CRUD operations
func GetKubeClientSet() (*kubernetes.Clientset, error) {
	kubeconfig := os.Getenv("KUBERNETES_CONFIG")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return clientSet, nil
}
