package registry

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"github.com/openshift-psap/special-resource-operator/pkg/warn"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	pullSecretNamespace  = "openshift-config"
	pullSecretName       = "pull-secret"
	pullSecretFileName   = ".dockerconfigjson"
	dockerConfigFilePath = "/home/nonroot/.docker/config.json"
)

var log = zap.New(zap.UseDevMode(true)).WithName(color.Print("registry", color.Brown))

type DriverToolkitEntry struct {
	ImageURL            string `json:"imageURL"`
	KernelFullVersion   string `json:"kernelFullVersion"`
	RTKernelFullVersion string `json:"RTKernelFullVersion"`
	OSVersion           string `json:"OSVersion"`
}

func writeImageRegistryCredentials() error {
	_, err := clients.Interface.GetNamespace(context.TODO(), pullSecretNamespace, metav1.GetOptions{})
	if err != nil {
		log.Info("Can not find namespace for pull secrets, assuming vanilla k8s")
		return nil
	}

	s, err := clients.Interface.GetSecret(context.TODO(), pullSecretNamespace, pullSecretName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, "Can not retrieve pull secrets")
	}

	pullSecretData, ok := s.Data[pullSecretFileName]
	if !ok {
		return errors.New("Can not find data content in the secret")
	}

	err = os.WriteFile(dockerConfigFilePath, pullSecretData, 0644)
	if err != nil {
		return errors.Wrap(err, "Can not write pull secret file")
	}
	return nil
}

func LastLayer(entry string) (v1.Layer, error) {
	if err := writeImageRegistryCredentials(); err != nil {
		return nil, err
	}

	var repo string

	if hash := strings.Split(entry, "@"); len(hash) > 1 {
		repo = hash[0]
	} else if tag := strings.Split(entry, ":"); len(tag) > 1 {
		repo = tag[0]
	}

	options := crane.NilOption

	manifest, err := crane.Manifest(entry, options)
	if err != nil {
		warn.OnError(fmt.Errorf("cannot extract manifest: %v", err))
		return nil, nil
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

	return crane.PullLayer(repo+"@"+digest, options)
}

func ExtractToolkitRelease(layer v1.Layer) (DriverToolkitEntry, error) {
	var dtk DriverToolkitEntry

	targz, err := layer.Compressed()
	if err != nil {
		return dtk, err
	}
	defer dclose(targz)

	gr, err := gzip.NewReader(targz)
	if err != nil {
		return dtk, err
	}
	defer dclose(gr)

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
			log.Info("DTK", "kernel-version", entry)
			dtk.KernelFullVersion = entry

			entry, _, err = unstructured.NestedString(obj.Object, "RT_KERNEL_VERSION")
			if err != nil {
				return dtk, err
			}
			log.Info("DTK", "rt-kernel-version", entry)
			dtk.RTKernelFullVersion = entry

			entry, _, err = unstructured.NestedString(obj.Object, "RHEL_VERSION")
			if err != nil {
				return dtk, err
			}
			log.Info("DTK", "rhel-version", entry)
			dtk.OSVersion = entry

			return dtk, err
		}

	}

	return dtk, errors.New("Missing driver toolkit entry: /etc/driver-toolkit-release.json")
}

func ReleaseManifests(layer v1.Layer) (string, string, error) {

	targz, err := layer.Compressed()
	if err != nil {
		return "", "", err
	}
	defer dclose(targz)

	gr, err := gzip.NewReader(targz)
	if err != nil {
		return "", "", err
	}
	defer dclose(gr)

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

func dclose(c io.Closer) {
	if err := c.Close(); err != nil {
		warn.OnError(err)
		//log.Error(err)
	}
}
