package framework

import (
	"context"
	"strings"

	srov1beta1 "github.com/openshift-psap/special-resource-operator/api/v1beta1"
	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"github.com/openshift-psap/special-resource-operator/pkg/exit"
	sroscheme "github.com/openshift-psap/special-resource-operator/pkg/scheme"
	"github.com/openshift-psap/special-resource-operator/pkg/warn"
	"github.com/openshift-psap/special-resource-operator/pkg/yamlutil"
	"github.com/pkg/errors"
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
	log    = ctrl.Log.WithName(color.Print("deploy", color.Blue))
)

func init() {
	utilruntime.Must(sroscheme.AddToScheme(scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(srov1beta1.AddToScheme(scheme))

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

}

func NewControllerRuntimeClient() client.Client {
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: "0",
	})
	exit.OnError(errors.Wrap(err, "unable to start manager"))

	client, err := client.New(mgr.GetConfig(), client.Options{Scheme: scheme})
	exit.OnError(err)
	// caching client
	// return mgr.GetClient()
	return client
}

func CreateFromYAML(yamlFile []byte, cl client.Client) error {

	scanner := yamlutil.NewYAMLScanner(yamlFile)

	for scanner.Scan() {
		yamlSpec := scanner.Bytes()
		obj := getObjFromYAMLSpec(yamlSpec)
		err := cl.Create(context.TODO(), obj)
		message := "Resource created"
		if err != nil {
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

func UpdateFromYAML(yamlFile []byte, cl client.Client) {

	scanner := yamlutil.NewYAMLScanner(yamlFile)

	for scanner.Scan() {
		yamlSpec := scanner.Bytes()

		obj := getObjFromYAMLSpec(yamlSpec)

		err := cl.Update(context.TODO(), obj)
		exit.OnError(err)

		log.Info("Updated", "Kind", obj.GetKind(), "Name", obj.GetName())
	}
}

// Don't use this to delete the CRD or undeploy the operator -- CR deletion will fail
func DeleteFromYAMLWithCR(yamlFile []byte, cl client.Client) error {

	scanner := yamlutil.NewYAMLScanner(yamlFile)

	for scanner.Scan() {
		yamlSpec := scanner.Bytes()
		obj := getObjFromYAMLSpec(yamlSpec)
		err := cl.Delete(context.TODO(), obj)
		if err != nil {
			return err
		}
		log.Info("Deleted", "Kind", obj.GetKind(), "Name", obj.GetName())
	}
	return nil
}

func DeleteFromYAML(yamlFile []byte, cl client.Client) error {

	scanner := yamlutil.NewYAMLScanner(yamlFile)

	for scanner.Scan() {
		yamlSpec := scanner.Bytes()
		obj := getObjFromYAMLSpec(yamlSpec)
		// CRD is deleted so CR deletion will fail since already gone
		if obj.GetKind() == "SpecialResource" {
			continue
		}
		err := cl.Delete(context.TODO(), obj)
		message := "Deleted resource"
		if err != nil {
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

func DeleteAllSpecialResources(cl client.Client) {

	specialresources := &srov1beta1.SpecialResourceList{}

	opts := []client.ListOption{}
	err := cl.List(context.TODO(), specialresources, opts...)
	if err != nil {
		if strings.Contains(err.Error(), "no matches for kind \"SpecialResource\" in version ") {
			warn.OnError(err)
			return
		}
		// This should never happen
		exit.OnError(err)
	}

	delOpts := []client.DeleteOption{}
	for _, sr := range specialresources.Items {
		log.Info("Deleting", "SR", sr.GetName())
		err := cl.Delete(context.TODO(), &sr, delOpts...)
		exit.OnError(err)
	}

}

func getObjFromYAMLSpec(yamlSpec []byte) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	jsonSpec, err := yaml.YAMLToJSON(yamlSpec)
	exit.OnError(errors.Wrap(err, "Could not convert yaml file to json: "+string(yamlSpec)))

	err = obj.UnmarshalJSON(jsonSpec)
	exit.OnError(errors.Wrap(err, "Cannot unmarshall json spec, check your manifests"))

	return obj
}
