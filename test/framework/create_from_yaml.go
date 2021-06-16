package framework

import (
	"context"

	srov1beta1 "github.com/openshift-psap/special-resource-operator/api/v1beta1"
	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"github.com/openshift-psap/special-resource-operator/pkg/exit"
	sroscheme "github.com/openshift-psap/special-resource-operator/pkg/scheme"
	"github.com/openshift-psap/special-resource-operator/pkg/yamlutil"
	"github.com/pkg/errors"
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
		Scheme: scheme,
	})
	exit.OnError(errors.Wrap(err, "unable to start manager"))

	return mgr.GetClient()
}

func CreateFromYAML(yamlFile []byte, cl client.Client) {

	scanner := yamlutil.NewYAMLScanner(yamlFile)

	for scanner.Scan() {
		yamlSpec := scanner.Bytes()

		obj := getObjFromYAMLSpec(yamlSpec)

		err := cl.Create(context.TODO(), obj)
		exit.OnError(err)

		log.Info("Created", "Kind", obj.GetKind(), "Name", obj.GetName())
	}
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
func DeleteFromYAMLWithCR(yamlFile []byte, cl client.Client) {

	scanner := yamlutil.NewYAMLScanner(yamlFile)

	for scanner.Scan() {
		yamlSpec := scanner.Bytes()

		obj := getObjFromYAMLSpec(yamlSpec)

		err := cl.Delete(context.TODO(), obj)
		exit.OnError(err)

		log.Info("Deleted", "Kind", obj.GetKind(), "Name", obj.GetName())
	}
}

func DeleteFromYAML(yamlFile []byte, cl client.Client) {

	scanner := yamlutil.NewYAMLScanner(yamlFile)

	for scanner.Scan() {
		yamlSpec := scanner.Bytes()

		obj := getObjFromYAMLSpec(yamlSpec)

		// CRD is deleted so CR deletion will fail since already gone
		if obj.GetKind() == "SpecialResource" {
			continue
		}

		err := cl.Delete(context.TODO(), obj)
		exit.OnError(err)

		log.Info("Deleted", "Kind", obj.GetKind(), "Name", obj.GetName())
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
