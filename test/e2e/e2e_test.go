// Copyright 2018 The Operator-SDK Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package e2e

import (
	goctx "context"
	"errors"
	"fmt"
	"io/ioutil"
	"testing"
	"time"

	"github.com/ghodss/yaml"
	apis "github.com/zvonkok/special-resource-operator/pkg/apis"
	operator "github.com/zvonkok/special-resource-operator/pkg/apis/sro/v1alpha1"
	"github.com/zvonkok/special-resource-operator/pkg/yamlutil"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"

	"github.com/operator-framework/operator-sdk/pkg/test"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
)

//func GetLogs(kubeClient kubernetes.Interface, namespace string, podName, containerName string) (string, error) {
//	logs, err := kubeClient.CoreV1().RESTClient().Get().
//		Resource("pods").
//		Namespace(namespace).
//		Name(podName).SubResource("log").
//		Param("container", containerName).
//		Do().
//		Raw()
//	if err != nil {
//		return "", err
//	}
//	return string(logs), err
//}

var (
	retryInterval        = time.Second * 5
	timeout              = time.Second * 60
	cleanupRetryInterval = time.Second * 1
	cleanupTimeout       = time.Second * 30
	opName               = "gpu"
	opNamespace          = "openshift-sro"
	opImage              = "quay.io/zvonkok/node-feature-discovery:v4.2"
	//opImage = "registry.svc.ci.openshift.org/openshift/node-feature-discovery-container:v4.2"
)

func createFromYAML(yamlFile []byte, skipIfExists bool, cleanupOptions *CleanupOptions) error {
	//	namespace, err := ctx.GetNamespace()
	//	if err != nil {
	//		return err
	//	}
	scanner := yamlutil.NewYAMLScanner(yamlFile)
	for scanner.Scan() {
		yamlSpec := scanner.Bytes()

		obj := &unstructured.Unstructured{}
		jsonSpec, err := yaml.YAMLToJSON(yamlSpec)
		if err != nil {
			return fmt.Errorf("could not convert yaml file to json: %v", err)
		}
		obj.UnmarshalJSON(jsonSpec)
		obj.SetNamespace(namespace)
		err = Global.Client.Create(goctx.TODO(), obj, cleanupOptions)
		if skipIfExists && apierrors.IsAlreadyExists(err) {
			continue
		}
		if err != nil {
			_, restErr := restMapper.RESTMappings(obj.GetObjectKind().GroupVersionKind().GroupKind())
			if restErr == nil {
				return err
			}
			// don't store error, as only error will be timeout. Error from runtime client will be easier for
			// the user to understand than the timeout error, so just use that if we fail
			wait.PollImmediate(time.Second*1, time.Second*10, func() (bool, error) {
				restMapper.Reset()
				_, err := restMapper.RESTMappings(obj.GetObjectKind().GroupVersionKind().GroupKind())
				if err != nil {
					return false, nil
				}
				return true, nil
			})
			err = Global.Client.Create(goctx.TODO(), obj, cleanupOptions)
			if skipIfExists && apierrors.IsAlreadyExists(err) {
				continue
			}
			if err != nil {
				return err
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan manifest: (%v)", err)
	}
	return nil
}

func InitializeClusterResources(file string) error {
	// create namespaced resources
	namespacedYAML, err := ioutil.ReadFile(file)
	if err != nil {
		return fmt.Errorf("failed to read namespaced manifest: %v", err)
	}
	return createFromYAML(namespacedYAML, false)
}

func TestSpecialResourceAddScheme(t *testing.T) {
	sroList := &operator.SpecialResourceList{}

	err := framework.AddToFrameworkScheme(apis.AddToScheme, sroList)
	if err != nil {
		t.Fatalf("failed to add custom resource scheme to framework: %v", err)
	}
}

func TestSpecialResource(t *testing.T) {
	ctx := framework.NewTestCtx(t)

	defer ctx.Cleanup()

	err := ctx.InitializeClusterResources("/home/zvonkok/go/src/github.com/zvonkok/special-resource-operator/manifests/operator-init.yaml")
	if err != nil {
		t.Fatalf("failed to initialize cluster resources: %v", err)
	}
	t.Log("Initialized cluster resources")
	/*
		namespace, err := ctx.GetNamespace()
		if err != nil {
			t.Fatal(err)
		}

		err = createClusterRoleBinding(t, namespace, ctx)
		if err != nil {
			t.Fatal(err)
		}

		// get global framework variables
		f := framework.Global
		err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, "nfd-operator", 1, retryInterval, timeout)
		if err != nil {
			t.Fatal(err)
		}

		if err = nodeFeatureDiscovery(t, f, ctx); err != nil {
			t.Fatal(err)
		}
	*/
}

func createClusterRoleBinding(t *testing.T, namespace string, ctx *framework.TestCtx) error {
	// operator-sdk test cannot deploy clusterrolebinding
	obj := &rbacv1.ClusterRoleBinding{}

	namespacedYAML, err := ioutil.ReadFile("manifests/0400_cluster_role_binding.yaml")
	s := json.NewYAMLSerializer(json.DefaultMetaFactory, scheme.Scheme,
		scheme.Scheme)

	_, _, err = s.Decode(namespacedYAML, nil, obj)

	obj.SetNamespace(namespace)

	obj.Subjects[0].Namespace = namespace

	for _, subject := range obj.Subjects {
		if subject.Kind == "ServiceAccount" {
			subject.Namespace = namespace
		}
	}

	err = test.Global.Client.Create(goctx.TODO(), obj,
		&framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})

	if apierrors.IsAlreadyExists(err) {
		t.Errorf("ClusterRoleBinding already exists: %s", obj.Name)
	}

	return err
}

/*
func nodeFeatureDiscovery(t *testing.T, f *framework.Framework, ctx *framework.TestCtx) error {
	namespace, err := ctx.GetNamespace()
	if err != nil {
		return fmt.Errorf("could not get namespace: %v", err)
	}
	// create memcached custom resource
	nfd := &operator.SpecialResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      opName,
			Namespace: namespace,
		},
		Spec: operator.SpecialResourceSpec{
			OperandNamespace: opNamespace,
			OperandImage:     opImage,
		},
	}

	// use TestCtx's create helper to create the object and add a cleanup function for the new object
	err = f.Client.Create(goctx.TODO(), nfd, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	if err != nil {
		return err
	}

	t.Logf("Created CR with OperandNamespace: %s OperandImage %s", opNamespace, opImage)

	err = WaitForDaemonSet(t, f.KubeClient, opNamespace, "nfd-master", 0, retryInterval, timeout)
	if err != nil {
		return err
	}

	err = WaitForDaemonSet(t, f.KubeClient, opNamespace, "nfd-worker", 0, retryInterval, timeout)
	if err != nil {
		return err
	}

	return checkDefaultLabels(t, f.KubeClient)
}
*/
func checkDefaultLabels(t *testing.T, kubeclient kubernetes.Interface) error {

	opts := metav1.ListOptions{}
	nodeList, err := kubeclient.CoreV1().Nodes().List(opts)
	if err != nil {
		t.Error("Could not retrieve List of Nodes")
		return err
	}

	for _, node := range nodeList.Items {
		labels := node.GetLabels()
		key := "node-role.kubernetes.io/master"
		// don't care masters
		if val, ok := labels[key]; ok {
			t.Logf("Ignoring Master: %s=%s %s", key, val, node.Name)
			continue
		}
		key = "feature.node.kubernetes.io/kernel-version.full"
		if val, ok := labels[key]; ok {
			t.Logf("%s=%s: %s", key, val, node.Name)
		} else {
			t.Errorf("Node %s Label %s not found", node.Name, key)
			return errors.New("LabelIsNotFound")
		}
	}
	return nil
}

func WaitForDaemonSet(t *testing.T, kubeclient kubernetes.Interface, namespace, name string, replicas int, retryInterval, timeout time.Duration) error {
	return waitForDaemonSet(t, kubeclient, namespace, name, replicas, retryInterval, timeout, false)
}

func waitForDaemonSet(t *testing.T, kubeclient kubernetes.Interface, namespace, name string, replicas int, retryInterval, timeout time.Duration, isOperator bool) error {
	if isOperator && test.Global.LocalOperator {
		t.Log("Operator is running locally; skip waitForDaemonSet")
		return nil
	}
	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		daemonset, err := kubeclient.AppsV1().DaemonSets(namespace).Get(name, metav1.GetOptions{IncludeUninitialized: true})
		if err != nil {
			if apierrors.IsNotFound(err) {
				t.Logf("Waiting for availability of %s/%s DaemonSet\n", namespace, name)
				return false, nil
			}
			return false, err
		}

		if int(daemonset.Status.NumberUnavailable) == 0 {
			return true, nil
		}
		t.Logf("Waiting for full availability of %s/%s DaemonSet NumberUnavailable (%d/%d)\n", namespace, name, daemonset.Status.NumberUnavailable, 0)
		return false, nil
	})
	if err != nil {
		return err
	}
	t.Logf("DaemonSet NumberUnavailable (%d/%d)\n", 0, 0)
	return nil
}
