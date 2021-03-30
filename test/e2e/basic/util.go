package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	errs "github.com/pkg/errors"

	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/onsi/ginkgo"
	"github.com/openshift-psap/special-resource-operator/pkg/osversion"
	"github.com/openshift-psap/special-resource-operator/test/framework"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	//srov1beta1 "github.com/openshift-psap/special-resource-operator/api/v1beta1"
)

// Return kernel full version, os version, openshift version
//                    feature.node.kubernetes.io/kernel-version.full=4.18.0-240.10.1.el8_3.x86_64
//                    feature.node.kubernetes.io/system-os_release.RHEL_VERSION=8.3
//                    feature.node.kubernetes.io/system-os_release.VERSION_ID=4.7
func GetVersionTriplet(cs *framework.ClientSet) (string, string, string, error) {
	nodes, err := GetNodesByRole(cs, "worker")
	if err != nil {
		return "", "", "", err
	}
	node := nodes[0]
	labels := node.GetLabels()

	// Assuming all nodes are running the same OS...
	os := "feature.node.kubernetes.io/system-os_release"
	nodeOSrel := labels[os+".ID"]
	nodeKernelFullVersion := labels["feature.node.kubernetes.io/kernel-version.full"]
	nodeOSVersion := labels[os+".RHEL_VERSION"]
	nodeOCPVersion := labels[os+".VERSION_ID"]
	if len(nodeKernelFullVersion) == 0 || len(nodeOSVersion) == 0 {
		return "", "", "", errs.New("Cannot extract feature.node.kubernetes.io/system-os_release.*, is NFD running? Check node labels")
	}

	if nodeOSVersion == "" {
		// Old NFD version without OSVersion -- try to render it
		os := "feature.node.kubernetes.io/system-os_release"
		nodeOSmaj := labels[os+".VERSION_ID.major"]
		nodeOSmin := labels[os+".VERSION_ID.minor"]

		_, _, nodeOSVersion, err = osversion.RenderOperatingSystem(nodeOSrel, nodeOSmaj, nodeOSmin)
		if err != nil {
			return "", "", "", errs.New("Could not determine operating system version")
		}
	}

	if nodeOSrel != "rhcos" {
		return nodeKernelFullVersion, nodeOSrel + nodeOSVersion, nodeOCPVersion, errs.New("Unexpected error, node not running rhcos")
	}

	return nodeKernelFullVersion, "rhel" + nodeOSVersion, nodeOCPVersion, nil
}

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

// GetPodForNode returns a Pod that runs on a given node, in a given namespace,
// given a set of ListOptions which must narrow the number of potential Pods to 1.
func GetPodForNode(cs *framework.ClientSet, node *corev1.Node, ns string, listOptions metav1.ListOptions) (*corev1.Pod, error) {
	podList, err := cs.Pods(ns).List(context.TODO(), listOptions)
	if err != nil {
		return nil, fmt.Errorf("couldn't get a list of Pods with the given listOptions: %v", err)
	}

	if len(podList.Items) != 1 {
		if len(podList.Items) == 0 {
			return nil, fmt.Errorf("couldn't find any Pods matching the listOptions on the node %s", node.Name)
		}
		return nil, fmt.Errorf("too many (%d) matching Pods for node %s", len(podList.Items), node.Name)
	}
	return &podList.Items[0], nil
}

func Logf(format string, args ...interface{}) (n int, err error) {
	return fmt.Fprintf(ginkgo.GinkgoWriter, format+"\n", args...)
}

// execCommand executes command 'name' with arguments 'args' and optionally
// ('log') logs the output.  Returns captured standard output, standard error
// and the error returned.
func execCommand(log bool, name string, args ...string) (bytes.Buffer, bytes.Buffer, error) {
	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	cmd := exec.Command(name, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if log {
		_, err := Logf("run command '%s %v':\n  out=%s\n  err=%s\n  ret=%v",
			name, args, strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), err)
		if err != nil {
			return stdout, stderr, err
		}
	}

	return stdout, stderr, err
}

func GetRecentPodLogs(name string, namespace string, interval time.Duration) error {
	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	commandName := "oc"
	args := []string{"logs", "--since", fmt.Sprint(interval), name, "-n", namespace}
	cmd := exec.Command(commandName, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		_, err = Logf("Error running command '%s %v':\n  out=%s\n err=%s\n  ret=%v", commandName, args, strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), err)
		return err
	}

	_, err = Logf(strings.TrimSpace(stdout.String()))
	return err
}

// ExecAndLogCommand executes command 'name' with arguments 'args' and logs
// the output.  Returns captured standard output, standard error and the error
// returned.
func ExecAndLogCommand(name string, args ...string) (bytes.Buffer, bytes.Buffer, error) {
	return execCommand(true, name, args...)
}

// ExecCmdInPod executes command with arguments 'cmd' in Pod 'pod'.
func ExecCmdInPod(pod *corev1.Pod, cmd ...string) (string, error) {
	ocArgs := []string{"rsh", "-n", pod.ObjectMeta.Namespace, pod.Name}
	ocArgs = append(ocArgs, cmd...)

	stdout, stderr, err := execCommand(false, "oc", ocArgs...)
	if err != nil {
		return "", fmt.Errorf("failed to run %s in Pod %s:\n  out=%s\n  err=%s\n  ret=%v", cmd, pod.Name, stdout.String(), stderr.String(), err.Error())
	}

	return stdout.String(), nil
}

// waitForCmdOutputInPod runs command with arguments 'cmd' in Pod 'pod' at
// an interval 'interval' and retries for at most the duration 'duration'.
// If 'valExp' is not nil, it also expects standard output of the command with
// leading and trailing whitespace optionally ('trim') trimmed to match 'valExp'.
// The function returns the retrieved standard output and an error returned when
// running 'cmd'.  Non-nil error is also returned when standard output of 'cmd'
// did not match non-nil 'valExp' by the time duration 'duration' elapsed.
func WaitForCmdOutputInPod(interval, duration time.Duration, pod *corev1.Pod, valExp *string, trim bool, cmd ...string) (string, error) {
	var (
		val          string
		err, explain error
	)
	err = wait.PollImmediate(interval, duration, func() (bool, error) {
		val, err = ExecCmdInPod(pod, cmd...)

		if err != nil {
			explain = fmt.Errorf("out=%s; err=%s", val, err.Error())
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
		return val, fmt.Errorf("command %s outputs (leading/trailing whitespace trimmed) %s in Pod %s, expected %s: %v", cmd, val, pod.Name, *valExp, explain)
	}

	return val, err
}

// WaitForClusterOperatorCondition blocks until the SRO ClusterOperator status
// condition 'conditionType' is equal to the value of 'conditionStatus'.
// The execution interval to check the value is 'interval' and retries last
// for at most the duration 'duration'.
func WaitForClusterOperatorCondition(cs *framework.ClientSet, interval, duration time.Duration,
	conditionType configv1.ClusterStatusConditionType, conditionStatus configv1.ConditionStatus) error {
	var explain error

	startTime := time.Now()
	if err := wait.PollImmediate(interval, duration, func() (bool, error) {
		co, err := cs.ClusterOperators().Get(context.TODO(), "special-resource-operator", metav1.GetOptions{})
		if err != nil {
			explain = err
			return false, nil
		}

		for _, cond := range co.Status.Conditions {
			if cond.Type == conditionType &&
				cond.Status == conditionStatus {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		return errors.Wrapf(err, "failed to wait for ClusterOperator/special-resource-operator %s == %s (waited %s): %v",
			conditionType, conditionStatus, time.Since(startTime), explain)
	}
	return nil
}
