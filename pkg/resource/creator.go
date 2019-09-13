package resource

import (
	"fmt"
	"io/ioutil"
	"time"

	"github.com/ghodss/yaml"
	"github.com/zvonkok/special-resource-operator/pkg/yamlutil"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
)

func createFromYAML(yamlFile []byte, skipIfExists bool, cleanupOptions *CleanupOptions) error {
	namespace, err := ctx.GetNamespace()
	if err != nil {
		return err
	}
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

func InitializeClusterResources(cleanupOptions *CleanupOptions) error {
	// create namespaced resources
	namespacedYAML, err := ioutil.ReadFile(*Global.NamespacedManPath)
	if err != nil {
		return fmt.Errorf("failed to read namespaced manifest: %v", err)
	}
	return createFromYAML(namespacedYAML, false, cleanupOptions)
}
