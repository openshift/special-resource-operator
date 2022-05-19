package registry

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/openshift/special-resource-operator/pkg/clients"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	pullSecretNamespace          = "openshift-config"
	pullSecretName               = "pull-secret"
	pullSecretFileName           = ".dockerconfigjson"
	driverToolkitJSONFile        = "etc/driver-toolkit-release.json"
	releaseManifestImagesRefFile = "release-manifests/image-references"
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
	GetLayersDigests(context.Context, string) (string, []string, error)
	GetLayerByDigest(context.Context, string, string) (v1.Layer, error)
}

func NewRegistry(kubeClient clients.ClientsInterface, cw CraneWrapper) Registry {
	return &registry{
		cw:         cw,
		kubeClient: kubeClient,
	}
}

type registry struct {
	cw         CraneWrapper
	kubeClient clients.ClientsInterface
}

func (r *registry) LastLayer(ctx context.Context, image string) (v1.Layer, error) {
	repo, digests, err := r.GetLayersDigests(ctx, image)
	if err != nil {
		return nil, fmt.Errorf("failed to get layers digests of the image %s: %w", image, err)
	}
	return r.cw.PullLayer(ctx, repo+"@"+digests[len(digests)-1])
}

func (r *registry) ExtractToolkitRelease(layer v1.Layer) (*DriverToolkitEntry, error) {
	var err error
	var found bool
	dtk := &DriverToolkitEntry{}
	obj, err := r.getHeaderFromLayer(layer, driverToolkitJSONFile)
	if err != nil {
		return nil, fmt.Errorf("failed to find file %s in image layer: %w", driverToolkitJSONFile, err)
	}

	dtk.KernelFullVersion, found, err = unstructured.NestedString(obj.Object, "KERNEL_VERSION")
	if !found || err != nil {
		return nil, fmt.Errorf("failed to get KERNEL_VERSION from %s, found %t: %w", driverToolkitJSONFile, found, err)
	}

	dtk.RTKernelFullVersion, found, err = unstructured.NestedString(obj.Object, "RT_KERNEL_VERSION")
	if !found || err != nil {
		return nil, fmt.Errorf("failed to get RT_KERNEL_VERSION from %s, found %t: %w", driverToolkitJSONFile, found, err)
	}

	dtk.OSVersion, found, err = unstructured.NestedString(obj.Object, "RHEL_VERSION")
	if !found || err != nil {
		return nil, fmt.Errorf("failed to get RHEL_VERSION from %s, found %t: %w", driverToolkitJSONFile, found, err)
	}
	return dtk, nil
}

func (r *registry) ReleaseManifests(layer v1.Layer) (string, error) {
	obj, err := r.getHeaderFromLayer(layer, releaseManifestImagesRefFile)
	if err != nil {
		return "", fmt.Errorf("failed to find file %s in image layer: %w", releaseManifestImagesRefFile, err)
	}

	tags, found, err := unstructured.NestedSlice(obj.Object, "spec", "tags")
	if !found || err != nil {
		return "", fmt.Errorf("failed to find spec/tag in the %s, found %t: %w", releaseManifestImagesRefFile, found, err)
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
	return "", fmt.Errorf("failed to find driver-toolkit entry in the %s file", releaseManifestImagesRefFile)
}

func (r *registry) ReleaseImageMachineOSConfig(layer v1.Layer) (string, error) {
	obj, err := r.getHeaderFromLayer(layer, releaseManifestImagesRefFile)
	if err != nil {
		return "", fmt.Errorf("failed to find file %s in image layer: %w", releaseManifestImagesRefFile, err)
	}

	tags, found, err := unstructured.NestedSlice(obj.Object, "spec", "tags")
	if !found || err != nil {
		return "", fmt.Errorf("failed to find spec/tags in %s, found %t: %w", releaseManifestImagesRefFile, found, err)
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
	return "", fmt.Errorf("failed to find machine-os-content in the %s file", releaseManifestImagesRefFile)
}

func (r *registry) GetLayersDigests(ctx context.Context, image string) (string, []string, error) {
	var repo string

	if hash := strings.Split(image, "@"); len(hash) > 1 {
		repo = hash[0]
	} else if tag := strings.Split(image, ":"); len(tag) > 1 {
		repo = tag[0]
	}

	if repo == "" {
		return "", nil, fmt.Errorf("image url %s is not valid, does not contain hash or tag", image)
	}

	manifest, err := r.getManifestStreamFromImage(ctx, image, repo)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get manifest stream from image %s: %w", image, err)
	}

	digests, err := r.getLayersDigestsFromManifestStream(manifest)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get layers digests from manifest stream of image %s: %w", image, err)
	}

	return repo, digests, nil
}

func (r *registry) GetLayerByDigest(ctx context.Context, repo string, digest string) (v1.Layer, error) {
	return r.cw.PullLayer(ctx, repo+"@"+digest)
}

func (r *registry) getManifestStreamFromImage(ctx context.Context, image, repo string) ([]byte, error) {
	manifest, err := r.cw.Manifest(ctx, image)
	if err != nil {
		return nil, fmt.Errorf("failed to get crane manifest from image %s: %w", image, err)
	}

	release := unstructured.Unstructured{}
	if err = json.Unmarshal(manifest, &release.Object); err != nil {
		return nil, fmt.Errorf("failed to unmarshal crane manifest: %w", err)
	}

	imageMediaType, mediaTypeFound, err := unstructured.NestedString(release.Object, "mediaType")
	if err != nil {
		return nil, fmt.Errorf("unmarshalled manifests invalid format: %w", err)
	}
	if !mediaTypeFound {
		return nil, fmt.Errorf("mediaType is missing from the image %s manifest", image)
	}

	if strings.Contains(imageMediaType, "manifest.list") {
		archDigest, err := r.getImageDigestFromMultiImage(manifest)
		if err != nil {
			return nil, fmt.Errorf("failed to get arch digets from multi arch image: %w", err)
		}
		// get the manifest stream for the image of the architecture
		manifest, err = r.cw.Manifest(ctx, repo+"@"+archDigest)
		if err != nil {
			return nil, fmt.Errorf("failed to get crane manifest for the arch image: %w", err)
		}
	}
	return manifest, nil
}

func (r *registry) getLayersDigestsFromManifestStream(manifestStream []byte) ([]string, error) {
	manifest := v1.Manifest{}

	if err := json.Unmarshal(manifestStream, &manifest); err != nil {
		return nil, fmt.Errorf("failed to unmarshal manifest stream: %w", err)
	}

	digests := make([]string, len(manifest.Layers))
	for i, layer := range manifest.Layers {
		digests[i] = layer.Digest.Algorithm + ":" + layer.Digest.Hex
	}
	return digests, nil
}

func (r *registry) getHeaderFromLayer(layer v1.Layer, headerName string) (*unstructured.Unstructured, error) {

	targz, err := layer.Compressed()
	if err != nil {
		return nil, fmt.Errorf("failed to get targz from layer: %w", err)
	}
	// err ignored because we're only reading
	defer targz.Close()

	gr, err := gzip.NewReader(targz)
	if err != nil {
		return nil, fmt.Errorf("failed to create reader from targz: %w", err)
	}
	// err ignored because we're only reading
	defer gr.Close()

	tr := tar.NewReader(gr)

	for {
		header, err := tr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return nil, fmt.Errorf("failed to get next entry from targz: %w", err)
		}
		if header.Name == headerName {
			buff, err := io.ReadAll(tr)
			if err != nil {
				return nil, fmt.Errorf("failed to read tar entry: %w", err)
			}

			obj := unstructured.Unstructured{}

			if err = json.Unmarshal(buff, &obj.Object); err != nil {
				return nil, fmt.Errorf("failed to unmarshal tar entry: %w", err)
			}
			return &obj, nil
		}
	}

	return nil, fmt.Errorf("header %s not found in the layer", headerName)
}

func (r *registry) getImageDigestFromMultiImage(manifestListStream []byte) (string, error) {
	arch := runtime.GOARCH
	manifestList := v1.IndexManifest{}

	if err := json.Unmarshal(manifestListStream, &manifestList); err != nil {
		return "", fmt.Errorf("failed to unmarshal manifest stream: %w", err)
	}
	for _, manifest := range manifestList.Manifests {
		if manifest.Platform != nil && manifest.Platform.Architecture == arch {
			return manifest.Digest.Algorithm + ":" + manifest.Digest.Hex, nil
		}
	}
	return "", fmt.Errorf("Failed to find manifest for architecture %s", arch)
}
