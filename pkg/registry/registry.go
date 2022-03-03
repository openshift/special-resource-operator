package registry

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/utils"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	pullSecretNamespace = "openshift-config"
	pullSecretName      = "pull-secret"
	pullSecretFileName  = ".dockerconfigjson"
)

var (
	Interface Registry
)

type DriverToolkitEntry struct {
	ImageURL            string `json:"imageURL"`
	KernelFullVersion   string `json:"kernelFullVersion"`
	RTKernelFullVersion string `json:"RTKernelFullVersion"`
	OSVersion           string `json:"OSVersion"`
}

//go:generate mockgen -source=registry.go -package=registry -destination=mock_registry_api.go

type Registry interface {
	LastLayer(context.Context, string) (v1.Layer, error)
	ExtractToolkitRelease(v1.Layer) (*DriverToolkitEntry, error)
	ReleaseManifests(v1.Layer) (string, string, error)
}

func NewRegistry(kubeClient clients.ClientsInterface) Registry {
	return &registry{
		kubeClient: kubeClient,
		log:        zap.New(zap.UseDevMode(true)).WithName(utils.Print("registry", utils.Brown)),
	}
}

type registry struct {
	kubeClient clients.ClientsInterface
	log        logr.Logger
}

type dockerAuth struct {
	Auth  string
	Email string
}

func (r *registry) registryFromImageURL(image string) (string, error) {
	u, err := url.Parse(image)
	if err != nil || u.Host == "" {
		// "reg.io/org/repo:tag" are invalid from URL scheme POV
		// prepending double slash in case of error or empty Host and trying to parse again
		u, err = url.Parse("//" + image)
	}

	if err != nil || u.Host == "" {
		return "", fmt.Errorf("failed to parse image url: %s", image)
	}

	return u.Host, nil
}

func (r *registry) getImageRegistryCredentials(ctx context.Context, registry string) (dockerAuth, error) {
	s, err := r.kubeClient.GetSecret(ctx, pullSecretNamespace, pullSecretName, metav1.GetOptions{})
	if err != nil {
		return dockerAuth{}, errors.Wrap(err, "could not retrieve pull secrets")
	}

	pullSecretData, ok := s.Data[pullSecretFileName]
	if !ok {
		return dockerAuth{}, errors.New("could not find data content in the secret")
	}

	auths := struct {
		Auths map[string]dockerAuth
	}{}

	err = json.Unmarshal(pullSecretData, &auths)
	if err != nil {
		return dockerAuth{}, errors.New("failed to unmarshal auths")
	}

	if auth, ok := auths.Auths[registry]; !ok {
		return dockerAuth{}, fmt.Errorf("cluster PullSecret does not contain auth for registry %s", registry)
	} else {
		return auth, nil
	}
}

func (r *registry) LastLayer(ctx context.Context, entry string) (v1.Layer, error) {
	registry, err := r.registryFromImageURL(entry)
	if err != nil {
		return nil, err
	}

	auth, err := r.getImageRegistryCredentials(ctx, registry)
	if err != nil {
		return nil, err
	}

	var repo string

	if hash := strings.Split(entry, "@"); len(hash) > 1 {
		repo = hash[0]
	} else if tag := strings.Split(entry, ":"); len(tag) > 1 {

		repo = tag[0]
	}

	var registryAuths []crane.Option
	if auth.Auth != "" {
		registryAuths = append(registryAuths, crane.WithAuth(authn.FromConfig(authn.AuthConfig{Username: auth.Email, Auth: auth.Auth})))
	}

	manifest, err := crane.Manifest(entry, registryAuths...)
	if err != nil {
		return nil, err
	}

	release := unstructured.Unstructured{}
	if err = json.Unmarshal(manifest, &release.Object); err != nil {
		return nil, err
	}

	layers, _, err := unstructured.NestedSlice(release.Object, "layers")
	if err != nil {
		return nil, err
	}

	last := layers[len(layers)-1]

	digest := last.(map[string]interface{})["digest"].(string)

	return crane.PullLayer(repo+"@"+digest, registryAuths...)
}

func (r *registry) ExtractToolkitRelease(layer v1.Layer) (*DriverToolkitEntry, error) {
	dtk := &DriverToolkitEntry{}

	targz, err := layer.Compressed()
	if err != nil {
		return dtk, err
	}
	defer r.dclose(targz)

	gr, err := gzip.NewReader(targz)
	if err != nil {
		return dtk, err
	}
	defer r.dclose(gr)

	tr := tar.NewReader(gr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}

		if header.Name == "etc/driver-toolkit-release.json" {
			buff, err := io.ReadAll(tr)
			if err != nil {
				return dtk, err
			}

			obj := unstructured.Unstructured{}

			if err = json.Unmarshal(buff, &obj.Object); err != nil {
				return dtk, err
			}

			entry, _, err := unstructured.NestedString(obj.Object, "KERNEL_VERSION")
			if err != nil {
				return dtk, err
			}
			r.log.Info("DTK", "kernel-version", entry)
			dtk.KernelFullVersion = entry

			entry, _, err = unstructured.NestedString(obj.Object, "RT_KERNEL_VERSION")
			if err != nil {
				return dtk, err
			}
			r.log.Info("DTK", "rt-kernel-version", entry)
			dtk.RTKernelFullVersion = entry

			entry, _, err = unstructured.NestedString(obj.Object, "RHEL_VERSION")
			if err != nil {
				return dtk, err
			}
			r.log.Info("DTK", "rhel-version", entry)
			dtk.OSVersion = entry

			return dtk, err
		}

	}

	return dtk, errors.New("Missing driver toolkit entry: /etc/driver-toolkit-release.json")
}

func (r *registry) ReleaseManifests(layer v1.Layer) (string, string, error) {

	targz, err := layer.Compressed()
	if err != nil {
		return "", "", err
	}
	defer r.dclose(targz)

	gr, err := gzip.NewReader(targz)
	if err != nil {
		return "", "", err
	}
	defer r.dclose(gr)

	tr := tar.NewReader(gr)

	version := ""
	imageURL := ""

	for {
		header, err := tr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return "", "", err
		}

		if header.Name == "release-manifests/image-references" {

			buff, err := io.ReadAll(tr)
			if err != nil {
				return "", "", err
			}

			obj := unstructured.Unstructured{}

			if err = json.Unmarshal(buff, &obj.Object); err != nil {
				return "", "", err
			}

			tags, _, err := unstructured.NestedSlice(obj.Object, "spec", "tags")
			if err != nil {
				return "", "", err
			}

			for _, tag := range tags {
				if tag.(map[string]interface{})["name"] == "driver-toolkit" {
					from := tag.(map[string]interface{})["from"]
					imageURL = from.(map[string]interface{})["name"].(string)
				}
			}

		}

		if header.Name == "release-manifests/release-metadata" {

			buff, err := io.ReadAll(tr)
			if err != nil {
				return "", "", err
			}

			obj := unstructured.Unstructured{}

			if err = json.Unmarshal(buff, &obj.Object); err != nil {
				return "", "", err
			}

			version, _, err = unstructured.NestedString(obj.Object, "version")
			if err != nil {
				return "", "", err
			}
		}

		if version != "" && imageURL != "" {
			break
		}

	}

	return version, imageURL, nil
}

func (r *registry) dclose(c io.Closer) {
	if err := c.Close(); err != nil {
		utils.WarnOnError(err)
		//log.Error(err)
	}
}
