package main

import (
	"flag"
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

	cl := framework.NewControllerRuntimeClient()

	framework.DeleteAllSpecialResources(cl)
	// sleep 10 for finalizers to kick in
	time.Sleep(10 * time.Second)

	manifests := assets.GetFrom(*path)

	for _, manifest := range manifests {
		_ = framework.DeleteFromYAML(manifest.Content, cl)
	}

}
