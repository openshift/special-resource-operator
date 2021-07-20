package clients

import (
	"os"

	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"github.com/openshift-psap/special-resource-operator/pkg/exit"
	buildv1 "github.com/openshift/api/build/v1"
	clientconfigv1 "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	log        logr.Logger
	Interface  *ClientsInterface
	RestConfig *rest.Config
	Namespace  string
)

type ClientsInterface struct {
	client.Client
	kubernetes.Clientset
	clientconfigv1.ConfigV1Client
	record.EventRecorder
	authn.Keychain
}

func init() {

	log = zap.New(zap.UseDevMode(true)).WithName(color.Print("clients", color.Brown))

}

// GetKubeClientSetOrDie Add a native non-caching client for advanced CRUD operations
func GetKubeClientSetOrDie() kubernetes.Clientset {

	clientSet, err := kubernetes.NewForConfig(RestConfig)
	if err != nil {
		log.Info(color.Print("GetConfigClientOrDie: "+err.Error(), color.Red))
		os.Exit(1)
	}
	return *clientSet
}

// GetConfigClientOrDie Add a configv1 client to the reconciler
func GetConfigClientOrDie() clientconfigv1.ConfigV1Client {

	client, err := clientconfigv1.NewForConfig(RestConfig)
	if err != nil {
		log.Info(color.Print("GetConfigClientOrDie: "+err.Error(), color.Red))
		os.Exit(1)
	}
	return *client
}

func HasResource(resource schema.GroupVersionResource) (bool, error) {
	dclient, err := discovery.NewDiscoveryClientForConfig(RestConfig)
	if err != nil {
		return false, errors.Wrap(err, "Error: cannot retrieve a DiscoveryClient")
	}
	if dclient == nil {
		log.Info("Warning: cannot retrieve DiscoveryClient. Assuming vanilla k8s")
		return false, nil
	}

	resources, err := dclient.ServerResourcesForGroupVersion(resource.GroupVersion().String())
	if apierrors.IsNotFound(err) {
		// entire group is missing
		return false, nil
	}
	if err != nil {
		log.Info("Error while querying ServerResources")
		return false, errors.Wrap(err, "Cannot query ServerResources")
	} else {
		for _, serverResource := range resources.APIResources {
			if serverResource.Name == resource.Resource {
				//Found it
				return true, nil
			}
		}
	}

	log.Info("Could not find resource", "serverResource:", resource.Resource)
	return false, nil
}

func BuildConfigsAvailable() (bool, error) {
	return HasResource(buildv1.SchemeGroupVersion.WithResource("buildconfigs"))
}

func GetPlatform() string {
	clusterIsOCP, err := BuildConfigsAvailable()
	if err != nil {
		exit.OnError(err)
	}
	if clusterIsOCP {
		return "OCP"
	} else {
		return "K8S"
	}
}
