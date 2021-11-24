package clients

import (
	"fmt"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/openshift-psap/special-resource-operator/pkg/color"
	buildv1 "github.com/openshift/api/build/v1"
	clientconfigv1 "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"

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
	log        = zap.New(zap.UseDevMode(true)).WithName(color.Print("clients", color.Brown))
	Interface  *ClientsInterface
	RestConfig *rest.Config
	Namespace  string
	config     genericclioptions.ConfigFlags
)

type ClientsInterface struct {
	client.Client
	kubernetes.Clientset
	clientconfigv1.ConfigV1Client
	record.EventRecorder
	authn.Keychain
	discovery.CachedDiscoveryInterface
}

// GetKubeClientSet returns a native non-caching client for advanced CRUD operations
func GetKubeClientSet() (*kubernetes.Clientset, error) {
	return kubernetes.NewForConfig(RestConfig)
}

// GetConfigClient returns a configv1 client to the reconciler
func GetConfigClient() (*clientconfigv1.ConfigV1Client, error) {
	return clientconfigv1.NewForConfig(RestConfig)
}

func GetCachedDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	return config.ToDiscoveryClient()
}

func HasResource(resource schema.GroupVersionResource) (bool, error) {
	dclient, err := discovery.NewDiscoveryClientForConfig(RestConfig)
	if err != nil {
		return false, fmt.Errorf("Cannot retrieve a DiscoveryClient: %w", err)
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
		return false, fmt.Errorf("Cannot query ServerResources: %w", err)
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

func GetPlatform() (string, error) {
	clusterIsOCP, err := BuildConfigsAvailable()
	if err != nil {
		return "", err
	}
	if clusterIsOCP {
		return "OCP", nil
	} else {
		return "K8S", nil
	}
}
