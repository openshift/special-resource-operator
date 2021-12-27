package main

import (
	"flag"
	"log"
	"os"
	"time"

	srov1beta1 "github.com/openshift-psap/special-resource-operator/api/v1beta1"
	"github.com/openshift-psap/special-resource-operator/pkg/assets"
	sroscheme "github.com/openshift-psap/special-resource-operator/pkg/scheme"
	"github.com/openshift-psap/special-resource-operator/test/framework"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

var (
	scheme = runtime.NewScheme()
)

func init() {

	utilruntime.Must(sroscheme.AddToScheme(scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(srov1beta1.AddToScheme(scheme))
}

func main() {

	path := flag.String("path", "", "Path to manifests that need to be deployed via kubeclient. (Required)")
	flag.Parse()

	if *path == "" {
		flag.PrintDefaults()
		os.Exit(0)
	}

	cl, err := framework.NewControllerRuntimeClient()
	if err != nil {
		log.Fatalf("Error getting a controller client: %v", err)
	}

	if err = framework.DeleteAllSpecialResources(cl); err != nil {
		log.Fatalf("Error deleting all special resources: %v", err)
	}
	// sleep 10 for finalizers to kick in
	time.Sleep(10 * time.Second)

	assetsInterface := assets.NewAssets()
	manifests := assetsInterface.GetFrom(*path)

	for _, manifest := range manifests {
		if err = framework.DeleteFromYAML(manifest.Content, cl); err != nil {
			log.Fatalf("Error deleting from YAML: %v", err)
		}
	}
}
