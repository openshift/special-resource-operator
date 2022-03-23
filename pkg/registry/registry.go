package registry

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/openshift/special-resource-operator/pkg/clients"
	"github.com/openshift/special-resource-operator/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	pullSecretNamespace = "openshift-config"
	pullSecretName      = "pull-secret"
	pullSecretFileName  = ".dockerconfigjson"
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
	ReleaseManifests(v1.Layer) (string, error)
	ReleaseImageMachineOSConfig(layer v1.Layer) (string, error)
	GetLayersDigests(context.Context, string) (string, []string, []crane.Option, error)
	GetLayerByDigest(string, string, []crane.Option) (v1.Layer, error)
}

func NewRegistry(kubeClient clients.ClientsInterface) Registry {
	return &registry{
		kubeClient: kubeClient,
	}
}

type registry struct {
	kubeClient clients.ClientsInterface
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
		return dockerAuth{}, fmt.Errorf("could not retrieve pull secrets, [%w]", err)
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
	repo, digests, registryAuths, err := r.GetLayersDigests(ctx, entry)
	if err != nil {
		return nil, err
	}
	return crane.PullLayer(repo+"@"+digests[len(digests)-1], registryAuths...)
}

func (r *registry) ExtractToolkitRelease(layer v1.Layer) (*DriverToolkitEntry, error) {
	var err error
	var found bool
	dtk := &DriverToolkitEntry{}
	obj, err := r.getHeaderFromLayer(layer, "etc/driver-toolkit-release.json")
	if err != nil {
		return nil, err
	}

	dtk.KernelFullVersion, found, err = unstructured.NestedString(obj.Object, "KERNEL_VERSION")
	if !found || err != nil {
		return nil, fmt.Errorf("failed to get KERNEL_VERSION from etc/driver-toolkit-release.json: err %w, found %t", err, found)
	}

	dtk.RTKernelFullVersion, found, err = unstructured.NestedString(obj.Object, "RT_KERNEL_VERSION")
	if !found || err != nil {
		return nil, fmt.Errorf("failed to get RT_KERNEL_VERSION from etc/driver-toolkit-release.json: err %w, found %t", err, found)
	}

	dtk.OSVersion, found, err = unstructured.NestedString(obj.Object, "RHEL_VERSION")
	if !found || err != nil {
		return nil, fmt.Errorf("failed to get RHEL_VERSION from etc/driver-toolkit-release.json: err %w, found %t", err, found)
	}
	return dtk, nil
}

func (r *registry) ReleaseManifests(layer v1.Layer) (string, error) {
	obj, err := r.getHeaderFromLayer(layer, "release-manifests/image-references")
	if err != nil {
		return "", err
	}

	tags, found, err := unstructured.NestedSlice(obj.Object, "spec", "tags")
	if !found || err != nil {
		return "", fmt.Errorf("failed to find spec/tag in the release-manifests/image-references: err %w, found %t", err, found)
	}
	for _, tag := range tags {
		if tag.(map[string]interface{})["name"] == "driver-toolkit" {
			from, ok := tag.(map[string]interface{})["from"]
			if !ok {
				return "", errors.New("invalid image reference format for driver toolkit entry, missing `from` tag")
			}
			imageURL, ok := from.(map[string]interface{})["name"].(string)
			if !ok {
				return "", errors.New("invalid image reference format for driver toolkit entry, missing `name` tag")
			}
			return imageURL, nil
		}
	}
	return "", errors.New("failed to find driver-toolkit in the release-manifests/image-references")
}

func (r *registry) ReleaseImageMachineOSConfig(layer v1.Layer) (string, error) {
	obj, err := r.getHeaderFromLayer(layer, "release-manifests/image-references")
	if err != nil {
		return "", err
	}

	tags, found, err := unstructured.NestedSlice(obj.Object, "spec", "tags")
	if !found || err != nil {
		return "", fmt.Errorf("failed to find spec/tags in release-manifests/image-references: error %w, found %t", err, found)
	}

	for _, tag := range tags {
		if tag.(map[string]interface{})["name"] == "machine-os-content" {
			annotations, ok := tag.(map[string]interface{})["annotations"]
			if !ok {
				return "", fmt.Errorf("invalid machine-os-content format, annotations tag")
			}
			osVersion, ok := annotations.(map[string]interface{})["io.openshift.build.versions"].(string)
			if !ok {
				return "", fmt.Errorf("invalid machine-os-content format, io.openshift.build.versions tag")
			}
			return osVersion, nil
		}
	}
	return "", fmt.Errorf("failed to find machine-os-content in the release-manifests/image-references")
}

func (r *registry) GetLayersDigests(ctx context.Context, entry string) (string, []string, []crane.Option, error) {
	registry, err := r.registryFromImageURL(entry)
	if err != nil {
		return "", nil, nil, err
	}

	auth, err := r.getImageRegistryCredentials(ctx, registry)
	if err != nil {
		return "", nil, nil, err
	}

	var repo string

	if hash := strings.Split(entry, "@"); len(hash) > 1 {
		repo = hash[0]
	} else if tag := strings.Split(entry, ":"); len(tag) > 1 {
		repo = tag[0]
	}

	if repo == "" {
		return "", nil, nil, fmt.Errorf("image url %s is not valid, does not contain hash or tag", entry)
	}

	var registryAuths []crane.Option
	if auth.Auth != "" {
		registryAuths = append(registryAuths, crane.WithAuth(authn.FromConfig(authn.AuthConfig{Username: auth.Email, Auth: auth.Auth})))
	}

	manifest, err := crane.Manifest(entry, registryAuths...)
	if err != nil {
		return "", nil, nil, err
	}

	release := unstructured.Unstructured{}
	if err = json.Unmarshal(manifest, &release.Object); err != nil {
		return "", nil, nil, err
	}

	layers, _, err := unstructured.NestedSlice(release.Object, "layers")
	if err != nil {
		return "", nil, nil, err
	}
	digests := make([]string, len(layers))
	for i, layer := range layers {
		digests[i] = layer.(map[string]interface{})["digest"].(string)
	}

	return repo, digests, registryAuths, nil
}

func (r *registry) GetLayerByDigest(repo string, digest string, auth []crane.Option) (v1.Layer, error) {
	return crane.PullLayer(repo+"@"+digest, auth...)
}

func (r *registry) getHeaderFromLayer(layer v1.Layer, headerName string) (*unstructured.Unstructured, error) {

	targz, err := layer.Compressed()
	if err != nil {
		return nil, err
	}
	defer r.dclose(targz)

	gr, err := gzip.NewReader(targz)
	if err != nil {
		return nil, err
	}
	defer r.dclose(gr)

	tr := tar.NewReader(gr)

	for {
		header, err := tr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return nil, err
		}
		if header.Name == headerName {
			buff, err := io.ReadAll(tr)
			if err != nil {
				return nil, err
			}

			obj := unstructured.Unstructured{}

			if err = json.Unmarshal(buff, &obj.Object); err != nil {
				return nil, err
			}
			return &obj, nil
		}
	}

	return nil, fmt.Errorf("header %s not found in the layer", headerName)
}

func (r *registry) dclose(c io.Closer) {
	if err := c.Close(); err != nil {
		utils.WarnOnError(err)
	}
}
