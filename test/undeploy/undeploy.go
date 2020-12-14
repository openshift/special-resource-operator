package main

import (
	"context"
	"flag"
	"os"

	srov1beta1 "github.com/openshift-psap/special-resource-operator/api/v1beta1"
	"github.com/openshift-psap/special-resource-operator/pkg/assets"
	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"github.com/openshift-psap/special-resource-operator/pkg/exit"
	sroscheme "github.com/openshift-psap/special-resource-operator/pkg/scheme"
	"github.com/openshift-psap/special-resource-operator/pkg/yamlutil"
	errs "github.com/pkg/errors"
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
	log    = ctrl.Log.WithName(color.Print("undeploy", color.Blue))
)

func init() {

	utilruntime.Must(sroscheme.AddToScheme(scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(srov1beta1.AddToScheme(scheme))
}

func main() {

	path := flag.String("path", "", "Path to manifests that need to be deployed via kubeclient. (Required)")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
	})
	exit.OnError(errs.Wrap(err, "unable to start manager"))

	if *path == "" {
		flag.PrintDefaults()
		os.Exit(0)
	}

	cl := mgr.GetClient()

	manifests := assets.GetFrom(*path)

	for _, manifest := range manifests {
		deleteFromYAML(manifest.Content, cl)
	}

}
func deleteFromYAML(yamlFile []byte, cl client.Client) {

	scanner := yamlutil.NewYAMLScanner(yamlFile)

	for scanner.Scan() {
		yamlSpec := scanner.Bytes()

		obj := &unstructured.Unstructured{}
		jsonSpec, err := yaml.YAMLToJSON(yamlSpec)
		exit.OnError(errs.Wrap(err, "Could not convert yaml file to json: "+string(yamlSpec)))

		err = obj.UnmarshalJSON(jsonSpec)
		exit.OnError(errs.Wrap(err, "Cannot unmarshall json spec, check your manifests"))

		// CRD is deleted so CR deletion will fail since already gone
		if obj.GetKind() == "SpecialResource" {
			continue
		}

		err = cl.Delete(context.TODO(), obj)
		exit.OnError(err)

		log.Info("Deleted", "Kind", obj.GetKind(), "Name", obj.GetName())
	}
}
