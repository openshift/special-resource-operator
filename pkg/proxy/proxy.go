package proxy

import (
	"context"
	"fmt"
	"strings"

	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
)

type Configuration struct {
	HttpProxy  string
	HttpsProxy string
	NoProxy    string
	TrustedCA  string
}

//go:generate mockgen -source=proxy.go -package=proxy -destination=mock_proxy_api.go

type ProxyAPI interface {
	Setup(ctx context.Context, obj *unstructured.Unstructured) error
	ClusterConfiguration(ctx context.Context) (Configuration, error)
}

type proxy struct {
	kubeClient clients.ClientsInterface
	config     Configuration
}

func NewProxyAPI(kubeClient clients.ClientsInterface) ProxyAPI {
	return &proxy{
		kubeClient: kubeClient,
	}
}

func (p *proxy) Setup(ctx context.Context, obj *unstructured.Unstructured) error {

	if strings.Compare(obj.GetKind(), "Pod") == 0 {
		if err := p.setupPod(ctx, obj); err != nil {
			return errors.Wrap(err, "Cannot setup Pod Proxy")
		}
	}
	if strings.Compare(obj.GetKind(), "DaemonSet") == 0 {
		if err := p.setupDaemonSet(ctx, obj); err != nil {
			return errors.Wrap(err, "Cannot setup DaemonSet Proxy")
		}

	}

	return nil
}

// We may generalize more depending on how many entities need proxy settings.
// path... -> Pod, DaemonSet, BuildConfig, etc.
func (p *proxy) setupDaemonSet(ctx context.Context, obj *unstructured.Unstructured) error {
	containers, found, err := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	if err != nil {
		return err
	}

	if !found {
		return fmt.Errorf("spec.template.spec.containers not found in the daemon yaml")
	}

	if err = p.setupContainersProxy(ctx, containers); err != nil {
		return fmt.Errorf("cannot set proxy for Pod: %w", err)
	}

	return nil
}

func (p *proxy) setupPod(ctx context.Context, obj *unstructured.Unstructured) error {
	containers, found, err := unstructured.NestedSlice(obj.Object, "spec", "containers")
	if err != nil {
		return err
	}

	if !found {
		return fmt.Errorf("spec.containers not found in the pod yaml")
	}

	if err = p.setupContainersProxy(ctx, containers); err != nil {
		return fmt.Errorf("cannot set proxy for Pod: %w", err)
	}

	return nil
}

func (p *proxy) setupContainersProxy(ctx context.Context, containers []interface{}) error {

	for _, container := range containers {
		switch container := container.(type) {
		case map[string]interface{}:
			env, found, err := unstructured.NestedSlice(container, "env")
			if err != nil {
				return err
			}

			// If env not found we are creating a new env slice
			// otherwise we're appending it to the existing env slice
			httpproxy := make(map[string]interface{})
			httpsproxy := make(map[string]interface{})
			noproxy := make(map[string]interface{})

			httpproxy["name"] = "HTTP_PROXY"
			httpproxy["value"] = p.config.HttpProxy

			httpsproxy["name"] = "HTTPS_PROXY"
			httpsproxy["value"] = p.config.HttpsProxy

			noproxy["name"] = "NO_PROXY"
			noproxy["value"] = p.config.NoProxy

			if !found {
				env = make([]interface{}, 0)
			}

			env = append(env, httpproxy)
			env = append(env, httpsproxy)
			env = append(env, noproxy)

			if err = unstructured.SetNestedSlice(container, env, "env"); err != nil {
				return fmt.Errorf("cannot set env for container: %w", err)
			}

		default:
			ctrl.LoggerFrom(ctx).Info("Unexpected container type to set proxy for", "container", container)
		}
		break
	}

	return nil
}

func (p *proxy) ClusterConfiguration(ctx context.Context) (Configuration, error) {
	log := ctrl.LoggerFrom(ctx)
	proxy := &p.config

	proxiesAvailable, err := p.kubeClient.HasResource(configv1.SchemeGroupVersion.WithResource("proxies"))
	if err != nil {
		return *proxy, errors.Wrap(err, "Error discovering proxies API resource")
	}
	if !proxiesAvailable {
		log.Info("Warning: Could not find proxies API resource. Can be ignored on vanilla K8s.")
		return *proxy, nil
	}

	cfgs := &unstructured.UnstructuredList{}
	cfgs.SetAPIVersion("config.openshift.io/v1")
	cfgs.SetKind("ProxyList")

	err = p.kubeClient.List(ctx, cfgs)
	if err != nil {
		return *proxy, errors.Wrap(err, "Client cannot get ProxyList")
	}

	for _, cfg := range cfgs.Items {
		cfgName := cfg.GetName()

		var err error
		// If no proxy is configured, we do not exit we just give a warning
		// and initialized the Proxy struct with zero sized strings
		if strings.Contains(cfgName, "cluster") {
			if proxy.HttpProxy, _, err = unstructured.NestedString(cfg.Object, "spec", "httpProxy"); err != nil {
				log.Error(err, "Failed to obtain httpProxy")
				proxy.HttpProxy = ""
			}

			if proxy.HttpsProxy, _, err = unstructured.NestedString(cfg.Object, "spec", "httpsProxy"); err != nil {
				log.Error(err, "Failed to obtain httpsProxy")
				proxy.HttpsProxy = ""
			}

			if proxy.NoProxy, _, err = unstructured.NestedString(cfg.Object, "spec", "noProxy"); err != nil {
				log.Error(err, "Failed to obtain noProxy")
				proxy.NoProxy = ""
			}

			if proxy.TrustedCA, _, err = unstructured.NestedString(cfg.Object, "spec", "trustedCA", "name"); err != nil {
				log.Error(err, "Failed to obtain trustedCA's name")
				proxy.TrustedCA = ""
			}
		}
	}

	return *proxy, nil
}
