package framework

import (
	"context"
	"fmt"
	"strings"

	srov1beta1 "github.com/openshift-psap/special-resource-operator/api/v1beta1"
	sroscheme "github.com/openshift-psap/special-resource-operator/pkg/scheme"
	"github.com/openshift-psap/special-resource-operator/pkg/utils"
	"github.com/openshift-psap/special-resource-operator/pkg/yamlutil"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/yaml"
)

var (
	scheme = runtime.NewScheme()
	log    = ctrl.Log.WithName(utils.Print("deploy", utils.Blue))
)

func init() {
	utilruntime.Must(sroscheme.AddToScheme(scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(srov1beta1.AddToScheme(scheme))

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

}

func NewControllerRuntimeClient() (client.Client, error) {
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: "0",
	})
	if err != nil {
		return nil, fmt.Errorf("unable to start manager: %w", err)
	}

	return client.New(mgr.GetConfig(), client.Options{Scheme: scheme})
}

func CreateFromYAML(ctx context.Context, yamlFile []byte, cl client.Client) error {

	scanner := yamlutil.NewYAMLScanner(yamlFile)

	for scanner.Scan() {
		yamlSpec := scanner.Bytes()

		obj, err := getObjFromYAMLSpec(yamlSpec)
		if err != nil {
			return err
		}

		message := "Resource created"

		if err = cl.Create(ctx, obj); err != nil {
			if apierrors.IsAlreadyExists(err) {
				message = "Resource already exists"
			} else {
				return err
			}
		}
		log.Info(message, "Kind", obj.GetKind(), "Name", obj.GetName())
	}
	return nil
}

// Don't use this to delete the CRD or undeploy the operator -- CR deletion will fail
func DeleteFromYAMLWithCR(ctx context.Context, yamlFile []byte, cl client.Client) error {

	scanner := yamlutil.NewYAMLScanner(yamlFile)

	for scanner.Scan() {
		yamlSpec := scanner.Bytes()

		obj, err := getObjFromYAMLSpec(yamlSpec)
		if err != nil {
			return err
		}

		err = cl.Delete(ctx, obj)
		if err != nil {
			return err
		}
		log.Info("Deleted", "Kind", obj.GetKind(), "Name", obj.GetName())
	}

	return nil
}

func DeleteFromYAML(ctx context.Context, yamlFile []byte, cl client.Client) error {

	scanner := yamlutil.NewYAMLScanner(yamlFile)

	for scanner.Scan() {
		yamlSpec := scanner.Bytes()

		obj, err := getObjFromYAMLSpec(yamlSpec)
		if err != nil {
			return err
		}

		// CRD is deleted so CR deletion will fail since already gone
		if obj.GetKind() == "SpecialResource" {
			continue
		}

		message := "Deleted resource"

		if err = cl.Delete(ctx, obj); err != nil {
			if apierrors.IsNotFound(err) {
				message = "Resource didnt exist"
			} else {
				return err
			}
		}

		log.Info(message, "Kind", obj.GetKind(), "Name", obj.GetName())
	}

	return nil
}

func DeleteAllSpecialResources(ctx context.Context, cl client.Client) error {

	specialresources := &srov1beta1.SpecialResourceList{}

	opts := []client.ListOption{}
	err := cl.List(ctx, specialresources, opts...)
	if err != nil {
		if strings.Contains(err.Error(), "no matches for kind \"SpecialResource\" in version ") {
			utils.WarnOnError(err)
			return nil
		}
		// This should never happen
		return err
	}

	delOpts := []client.DeleteOption{}
	for _, sr := range specialresources.Items {
		log.Info("Deleting", "SR", sr.GetName())
		if err = cl.Delete(ctx, &sr, delOpts...); err != nil {
			return err
		}
	}

	return nil
}

func getObjFromYAMLSpec(yamlSpec []byte) (*unstructured.Unstructured, error) {
	obj := &unstructured.Unstructured{}
	jsonSpec, err := yaml.YAMLToJSON(yamlSpec)
	if err != nil {
		return nil, fmt.Errorf("could not convert yaml file to json: %s: %w", yamlSpec, err)
	}

	if err = obj.UnmarshalJSON(jsonSpec); err != nil {
		return nil, fmt.Errorf("cannot unmarshall json spec, check your manifests: %w", err)
	}

	return obj, nil
}
