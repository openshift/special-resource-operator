package clients

import (
	"fmt"

	"github.com/go-logr/logr"
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
	log        logr.Logger
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

func init() {

	log = zap.New(zap.UseDevMode(true)).WithName(color.Print("clients", color.Brown))

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

////FIXME:ybettan: remove?
//func (k *k8sClients) Create(ctx context.Context, obj client.Object) error {
//	return k.runtimeClient.Create(ctx, obj)
//}
//
////FIXME:ybettan: remove?
//func (k *k8sClients) GetPodLogs(namespace, podName string, podLogOpts *v1.PodLogOptions) *restclient.Request {
//	return k.clientset.CoreV1().Pods(namespace).GetLogs(podName, podLogOpts)
//}
//
////FIXME:ybettan: remove?
//func (k *k8sClients) GetNamespace(ctx context.Context, name string, opts metav1.GetOptions) (*v1.Namespace, error) {
//	return k.clientset.CoreV1().Namespaces().Get(ctx, name, opts)
//}
//
////FIXME:ybettan: remove?
//func (k *k8sClients) GetSecret(ctx context.Context, namespace, name string, opts metav1.GetOptions) (*v1.Secret, error) {
//	return k.clientset.CoreV1().Secrets(namespace).Get(ctx, name, opts)
//}
//
////FIXME:ybettan: remove?
//func (k *k8sClients) GetConfigMap(ctx context.Context, namespace, name string, opts metav1.GetOptions) (*v1.ConfigMap, error) {
//	return k.clientset.CoreV1().ConfigMaps(namespace).Get(ctx, name, opts)
//}
//
////FIXME:ybettan: remove?
//func (k *k8sClients) GetImage(ctx context.Context, name string, opts metav1.GetOptions) (*configv1.Image, error) {
//	return k.configV1Client.Images().Get(ctx, name, opts)
//}
//
////FIXME:ybettan: remove?
//func (k *k8sClients) ClusterVersionGet(ctx context.Context, opts metav1.GetOptions) (result *configv1.ClusterVersion, err error) {
//	return k.configV1Client.ClusterVersions().Get(ctx, clusterVersionName, opts)
//}
//
////FIXME:ybettan: remove?
//func (k *k8sClients) Invalidate() {
//	k.cachedDiscovery.Invalidate()
//}
//
////FIXME:ybettan: remove?
//func (k *k8sClients) ServerGroups() (*metav1.APIGroupList, error) {
//	return k.cachedDiscovery.ServerGroups()
//}
//
////FIXME:ybettan: remove?
//func (k *k8sClients) ServerGroupsAndResources() ([]*metav1.APIGroup, []*metav1.APIResourceList, error) {
//	return k.cachedDiscovery.ServerGroupsAndResources()
//}
//
////FIXME:ybettan: remove?
//func (k *k8sClients) StatusUpdate(ctx context.Context, obj client.Object) error {
//	return k.runtimeClient.Status().Update(ctx, obj)
//}
//
////FIXME:ybettan: remove?
//func (k *k8sClients) StatusPatch(ctx context.Context, original, modified client.Object) error {
//	patch := client.MergeFrom(original)
//	return k.runtimeClient.Status().Patch(ctx, modified, patch)
//}
//
////FIXME:ybettan: remove?
//func (k *k8sClients) CreateOrUpdate(ctx context.Context, obj client.Object, fn controllerutil.MutateFn) (controllerutil.OperationResult, error) {
//	return controllerruntime.CreateOrUpdate(ctx, k.runtimeClient, obj, fn)
//}

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
