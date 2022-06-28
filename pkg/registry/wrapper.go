package registry

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/pkg/sysregistriesv2"
	"github.com/containers/image/v5/types"
	"github.com/docker/cli/cli/config"
	dockertypes "github.com/docker/cli/cli/config/types"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/openshift/special-resource-operator/pkg/clients"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	RegistryConfFilePath = "/mnt/host/registries.conf"
)

var ErrNoAuthForRegistry = errors.New("no auth defined for this registry")

//go:generate mockgen -source=wrapper.go -package=registry -destination=mock_wrapper_api.go

type CraneWrapper interface {
	Manifest(context.Context, string) ([]byte, error)
	PullLayer(context.Context, string) (v1.Layer, error)
}

type craneWrapper struct {
	kubeClient clients.ClientsInterface
	ocg        OpenShiftCAGetter
	sysCtx     *types.SystemContext
}

func NewCraneWrapper(kubeClient clients.ClientsInterface, ocg OpenShiftCAGetter, registriesConfFilePath string) *craneWrapper {
	return &craneWrapper{
		kubeClient: kubeClient,
		ocg:        ocg,
		sysCtx: &types.SystemContext{
			SystemRegistriesConfPath: registriesConfFilePath,
		},
	}
}

func (cw *craneWrapper) getPullSourcesForImageReference(imageName string) ([]sysregistriesv2.PullSource, error) {
	r, err := sysregistriesv2.FindRegistry(cw.sysCtx, imageName)
	if err != nil {
		return nil, fmt.Errorf("could not find registry for image %q: %w", imageName, err)
	}

	n, err := reference.ParseNamed(imageName)
	if err != nil {
		return nil, fmt.Errorf("could not parse image name %q: %w", imageName, err)
	}

	// r is nil if none of the registries in registries.conf matched imageName.
	// In that case, return that reference as pull source.
	if r == nil {
		return []sysregistriesv2.PullSource{{Reference: n}}, nil
	}

	return r.PullSourcesFromReference(n)
}

func (cw *craneWrapper) getClusterCustomCertPool(ctx context.Context) (*x509.CertPool, error) {
	logger := ctrl.LoggerFrom(ctx)

	logger.Info("Loading system CA certificates")

	pool, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("could not access the system certificate pool: %w", err)
	}

	logger.Info("Getting the bundle")

	caBundlePEM, err := cw.ocg.CABundle(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting the CA bundle: %v", err)
	}

	if len(caBundlePEM) > 0 {
		if ok := pool.AppendCertsFromPEM(caBundlePEM); !ok {
			return nil, fmt.Errorf("could not append CA bundle to the pool: %v", err)
		}
	}

	logger.Info("Getting additional CAs")

	additionalCAs, err := cw.ocg.AdditionalTrustedCAs(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not get additional trusted CAs: %v", err)
	}

	for name, data := range additionalCAs {
		logger.V(1).Info("Adding certificate", "name", name)

		if ok := pool.AppendCertsFromPEM(data); !ok {
			return nil, fmt.Errorf("could not certificate %q to the pool: %v", name, err)
		}
	}

	return pool, nil
}

func (cw *craneWrapper) getAuthForRegistry(ctx context.Context, registry string) (authn.Authenticator, error) {
	s, err := cw.kubeClient.GetSecret(ctx, configNamespace, pullSecretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("could not retrieve pull secrets: %w", err)
	}

	pullSecretData, ok := s.Data[pullSecretFileName]
	if !ok {
		return nil, errors.New("could not find data content in the secret")
	}

	cfg, err := config.LoadFromReader(strings.NewReader(string(pullSecretData)))
	if err != nil {
		return nil, fmt.Errorf("could not read the pull secret as a configuration file: %w", err)
	}

	dockerAuth, err := cfg.GetAuthConfig(registry)
	if err != nil {
		return nil, fmt.Errorf("could not get AuthConfig for registry %q: %w", registry, err)
	}

	if dockerAuth == (dockertypes.AuthConfig{}) {
		return nil, ErrNoAuthForRegistry
	}

	authenticator := authn.FromConfig(authn.AuthConfig{
		Username:      dockerAuth.Username,
		Password:      dockerAuth.Password,
		Auth:          dockerAuth.Auth,
		IdentityToken: dockerAuth.IdentityToken,
		RegistryToken: dockerAuth.RegistryToken,
	})

	return authenticator, nil
}

func (cw *craneWrapper) getOptions(ctx context.Context, imageName string) (string, []crane.Option, error) {
	ps, err := cw.getPullSourcesForImageReference(imageName)
	if err != nil {
		return "", nil, fmt.Errorf("could not find a pull source for %q: %w", imageName, err)
	}

	if len(ps) == 0 {
		return "", nil, fmt.Errorf("empty pull sources for image %q: %w", imageName, err)
	}

	certPool, err := cw.getClusterCustomCertPool(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("could not create a client certificate pool: %w", err)
	}

	t := http.DefaultTransport.(*http.Transport).Clone()
	t.TLSClientConfig.RootCAs = certPool

	registry := reference.Domain(ps[0].Reference)

	opts := []crane.Option{
		crane.WithContext(ctx),
		crane.WithTransport(t),
	}

	if auth, err := cw.getAuthForRegistry(ctx, registry); err != nil {
		if !errors.Is(err, ErrNoAuthForRegistry) {
			return "", nil, fmt.Errorf("could not find auth for registry %s: %w", registry, err)
		}
	} else {
		opts = append(opts, crane.WithAuth(auth))
	}

	// In case the image is mirrored we will get a different registry
	return ps[0].Reference.String(), opts, nil
}

func (cw *craneWrapper) Manifest(ctx context.Context, imageName string) ([]byte, error) {
	imageName, opts, err := cw.getOptions(ctx, imageName)
	if err != nil {
		return nil, fmt.Errorf("could not build crane options: %w", err)
	}

	return crane.Manifest(imageName, opts...)
}

func (cw *craneWrapper) PullLayer(ctx context.Context, imageName string) (v1.Layer, error) {
	imageName, opts, err := cw.getOptions(ctx, imageName)
	if err != nil {
		return nil, fmt.Errorf("could not build crane options: %w", err)
	}

	return crane.PullLayer(imageName, opts...)
}
