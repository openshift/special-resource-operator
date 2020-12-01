package framework

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	clientconfigv1 "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	appsv1client "k8s.io/client-go/kubernetes/typed/apps/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	//tunedv1client "github.com/openshift/cluster-node-tuning-operator/pkg/generated/clientset/versioned/typed/tuned/v1"
	//clientmachineconfigv1 "github.com/openshift/machine-config-operator/pkg/generated/clientset/versioned/typed/machineconfiguration.openshift.io/v1"
)

type ClientSet struct {
	corev1client.CoreV1Interface
	appsv1client.AppsV1Interface
	clientconfigv1.ConfigV1Interface
	//tunedv1client.TunedV1Interface
	//clientmachineconfigv1.MachineconfigurationV1Interface
}

// NewClientSet returns a *ClientBuilder with the given kubeconfig.
func NewClientSet() *ClientSet {
	kubeconfig, err := getConfig()
	if err != nil {
		panic(err)
	}

	clientSet := &ClientSet{}
	clientSet.CoreV1Interface = corev1client.NewForConfigOrDie(kubeconfig)
	clientSet.ConfigV1Interface = clientconfigv1.NewForConfigOrDie(kubeconfig)
	//clientSet.TunedV1Interface = tunedv1client.NewForConfigOrDie(kubeconfig)
	clientSet.AppsV1Interface = appsv1client.NewForConfigOrDie(kubeconfig)
	//	clientSet.MachineconfigurationV1Interface = clientmachineconfigv1.NewForConfigOrDie(kubeconfig)

	return clientSet
}

func getConfig() (*rest.Config, error) {
	configFromFlags := func(kubeConfig string) (*rest.Config, error) {
		if _, err := os.Stat(kubeConfig); err != nil {
			return nil, fmt.Errorf("cannot stat kubeconfig '%s'", kubeConfig)
		}
		return clientcmd.BuildConfigFromFlags("", kubeConfig)
	}

	// If an env variable is specified with the config location, use that
	kubeConfig := os.Getenv("KUBECONFIG")
	if len(kubeConfig) > 0 {
		return configFromFlags(kubeConfig)
	}
	// If no explicit location, try the in-cluster config
	if c, err := rest.InClusterConfig(); err == nil {
		return c, nil
	}
	// If no in-cluster config, try the default location in the user's home directory
	if usr, err := user.Current(); err == nil {
		kubeConfig := filepath.Join(usr.HomeDir, ".kube", "config")
		return configFromFlags(kubeConfig)
	}

	return nil, fmt.Errorf("could not locate a kubeconfig")
}
