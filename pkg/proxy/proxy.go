package proxy

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/utils"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type Configuration struct {
	HttpProxy  string
	HttpsProxy string
	NoProxy    string
	TrustedCA  string
}

//go:generate mockgen -source=proxy.go -package=proxy -destination=mock_proxy_api.go

type ProxyAPI interface {
	Setup(obj *unstructured.Unstructured) error
	ClusterConfiguration(ctx context.Context) (Configuration, error)
}

type proxy struct {
	log    logr.Logger
	config Configuration
}

func NewProxyAPI() ProxyAPI {
	return &proxy{
		log: zap.New(zap.UseDevMode(true)).WithName(utils.Print("proxy", utils.Green)),
	}
}

func (p *proxy) Setup(obj *unstructured.Unstructured) error {

	if strings.Compare(obj.GetKind(), "Pod") == 0 {
		if err := p.setupPod(obj); err != nil {
			return errors.Wrap(err, "Cannot setup Pod Proxy")
		}
	}
	if strings.Compare(obj.GetKind(), "DaemonSet") == 0 {
		if err := p.setupDaemonSet(obj); err != nil {
			return errors.Wrap(err, "Cannot setup DaemonSet Proxy")
		}

	}

	return nil
}

// We may generalize more depending on how many entities need proxy settings.
// path... -> Pod, DaemonSet, BuildConfig, etc.
func (p *proxy) setupDaemonSet(obj *unstructured.Unstructured) error {
	containers, found, err := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	if err != nil {
		return err
	}

	if !found {
		return fmt.Errorf("spec.template.spec.containers not found in the daemon yaml")
	}

	if err = p.setupContainersProxy(containers); err != nil {
		return fmt.Errorf("cannot set proxy for Pod: %w", err)
	}

	return nil
}

func (p *proxy) setupPod(obj *unstructured.Unstructured) error {
	containers, found, err := unstructured.NestedSlice(obj.Object, "spec", "containers")
	if err != nil {
		return err
	}

	if !found {
		return fmt.Errorf("spec.containers not found in the pod yaml")
	}

	if err = p.setupContainersProxy(containers); err != nil {
		return fmt.Errorf("cannot set proxy for Pod: %w", err)
	}

	return nil
}

func (p *proxy) setupContainersProxy(containers []interface{}) error {

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
			p.log.Info("container", "DEFAULT NOT THE CORRECT TYPE", container)
		}
		break
	}

	return nil
}

func (p *proxy) ClusterConfiguration(ctx context.Context) (Configuration, error) {
	proxy := &p.config

	proxiesAvailable, err := clients.Interface.HasResource(configv1.SchemeGroupVersion.WithResource("proxies"))
	if err != nil {
		return *proxy, errors.Wrap(err, "Error discovering proxies API resource")
	}
	if !proxiesAvailable {
		p.log.Info("Warning: Could not find proxies API resource. Can be ignored on vanilla K8s.")
		return *proxy, nil
	}

	cfgs := &unstructured.UnstructuredList{}
	cfgs.SetAPIVersion("config.openshift.io/v1")
	cfgs.SetKind("ProxyList")

	err = clients.Interface.List(ctx, cfgs)
	if err != nil {
		return *proxy, errors.Wrap(err, "Client cannot get ProxyList")
	}

	for _, cfg := range cfgs.Items {
		cfgName := cfg.GetName()

		var fnd bool
		var err error
		// If no proxy is configured, we do not exit we just give a warning
		// and initialized the Proxy struct with zero sized strings
		if strings.Contains(cfgName, "cluster") {
			if proxy.HttpProxy, fnd, err = unstructured.NestedString(cfg.Object, "spec", "httpProxy"); err != nil {
				utils.WarnOnErrorOrNotFound(fnd, err)
				proxy.HttpProxy = ""
			}

			if proxy.HttpsProxy, fnd, err = unstructured.NestedString(cfg.Object, "spec", "httpsProxy"); err != nil {
				utils.WarnOnErrorOrNotFound(fnd, err)
				proxy.HttpsProxy = ""
			}

			if proxy.NoProxy, fnd, err = unstructured.NestedString(cfg.Object, "spec", "noProxy"); err != nil {
				utils.WarnOnErrorOrNotFound(fnd, err)
				proxy.NoProxy = ""
			}

			if proxy.TrustedCA, fnd, err = unstructured.NestedString(cfg.Object, "spec", "trustedCA", "name"); err != nil {
				utils.WarnOnErrorOrNotFound(fnd, err)
				proxy.TrustedCA = ""
			}
		}
	}

	return *proxy, nil
}
