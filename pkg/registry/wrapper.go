package registry

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"os"
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
)

var ErrNoAuthForRegistry = errors.New("no auth defined for this registry")

//go:generate mockgen -source=wrapper.go -package=registry -destination=mock_wrapper_api.go

type MirrorResolver interface {
	GetPullSourcesForImageReference(string) ([]sysregistriesv2.PullSource, error)
}

type mirrorResolver struct {
	sysCtx *types.SystemContext
}

func NewMirrorResolver(registriesConfPath string) MirrorResolver {
	return &mirrorResolver{
		sysCtx: &types.SystemContext{
			SystemRegistriesConfPath: registriesConfPath,
		},
	}
}

func (mr *mirrorResolver) GetPullSourcesForImageReference(imageName string) ([]sysregistriesv2.PullSource, error) {
	r, err := sysregistriesv2.FindRegistry(mr.sysCtx, imageName)
	if err != nil {
		return nil, fmt.Errorf("could not find registry for image %q: %v", imageName, err)
	}

	n, err := reference.ParseNamed(imageName)
	if err != nil {
		return nil, fmt.Errorf("could not parse image name %q: %v", imageName, err)
	}

	return r.PullSourcesFromReference(n)
}

type CertPoolGetter interface {
	SystemAndHostCertPool() (*x509.CertPool, error)
}

type certPoolGetter struct {
	hostBundlePath string
}

func NewCertPoolGetter(hostBundlePath string) CertPoolGetter {
	return &certPoolGetter{hostBundlePath: hostBundlePath}
}

func (cpg *certPoolGetter) SystemAndHostCertPool() (*x509.CertPool, error) {
	pool, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("could not access the system certificate pool: %v", err)
	}

	b, err := os.ReadFile(cpg.hostBundlePath)
	if err != nil {
		return nil, fmt.Errorf("could not open the host's certificate bundle at %q: %v", cpg.hostBundlePath, err)
	}

	if !pool.AppendCertsFromPEM(b) {
		return nil, fmt.Errorf("could not append host certificates to the pool: %v", err)
	}

	return pool, nil
}

type CraneWrapper interface {
	Manifest(context.Context, string, ...crane.Option) ([]byte, error)
	PullLayer(context.Context, string, ...crane.Option) (v1.Layer, error)
}

type craneWrapper struct {
	cpg        CertPoolGetter
	kubeClient clients.ClientsInterface
	mr         MirrorResolver
}

func NewCraneWrapper(cpg CertPoolGetter, kubeClient clients.ClientsInterface, mr MirrorResolver) CraneWrapper {
	return &craneWrapper{
		cpg:        cpg,
		kubeClient: kubeClient,
		mr:         mr,
	}
}

func (cw *craneWrapper) getAuthForRegistry(ctx context.Context, registry string) (authn.Authenticator, error) {
	s, err := cw.kubeClient.GetSecret(ctx, pullSecretNamespace, pullSecretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("could not retrieve pull secrets: %w", err)
	}

	pullSecretData, ok := s.Data[pullSecretFileName]
	if !ok {
		return nil, errors.New("could not find data content in the secret")
	}

	cfg, err := config.LoadFromReader(strings.NewReader(string(pullSecretData)))
	if err != nil {
		return nil, fmt.Errorf("could not read the pull secret as a configuration file: %v", err)
	}

	dockerAuth, err := cfg.GetAuthConfig(registry)
	if err != nil {
		return nil, fmt.Errorf("could not get AuthConfig for registry %q: %v", registry, err)
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

func (cw *craneWrapper) getOptions(ctx context.Context, imageName string) ([]crane.Option, error) {
	ps, err := cw.mr.GetPullSourcesForImageReference(imageName)
	if err != nil {
		return nil, fmt.Errorf("could not find a pull source for %q: %v", imageName, err)
	}

	if len(ps) == 0 {
		return nil, fmt.Errorf("empty pull sources for image %q: %v", imageName, err)
	}

	certPool, err := cw.cpg.SystemAndHostCertPool()
	if err != nil {
		return nil, fmt.Errorf("could not create a client certificate pool: %v", err)
	}

	t := http.DefaultTransport.(*http.Transport).Clone()
	t.TLSClientConfig.ClientCAs = certPool

	registry := reference.Domain(ps[0].Reference)

	opts := []crane.Option{
		crane.WithContext(ctx),
		crane.WithTransport(t),
	}

	if auth, err := cw.getAuthForRegistry(ctx, registry); err != nil {
		if !errors.Is(err, ErrNoAuthForRegistry) {
			return nil, fmt.Errorf("could not find auth for registry %s: %v", registry, err)
		}
	} else {
		opts = append(opts, crane.WithAuth(auth))
	}

	return opts, nil
}

func (cw *craneWrapper) Manifest(ctx context.Context, imageName string, opts ...crane.Option) ([]byte, error) {
	opts, err := cw.getOptions(ctx, imageName)
	if err != nil {
		return nil, fmt.Errorf("could not build crane options: %v", err)
	}

	return crane.Manifest(imageName, opts...)
}

func (cw *craneWrapper) PullLayer(ctx context.Context, imageName string, opts ...crane.Option) (v1.Layer, error) {
	opts, err := cw.getOptions(ctx, imageName)
	if err != nil {
		return nil, fmt.Errorf("could not build crane options: %v", err)
	}

	return crane.PullLayer(imageName, opts...)
}
