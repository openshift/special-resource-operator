package registry

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/authn/k8schain"
	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"github.com/openshift-psap/special-resource-operator/pkg/exit"
	"github.com/openshift-psap/special-resource-operator/pkg/warn"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	log logr.Logger
)

func init() {
	log = zap.New(zap.UseDevMode(true)).WithName(color.Print("registry", color.Brown))
}

type DriverToolkitEntry struct {
	ImageURL            string
	KernelFullVersion   string
	RTKernelFullVersion string
	OSVersion           string
}

func LastLayer(entry string) v1.Layer {

	var repo string

	if hash := strings.Split(entry, "@"); len(hash) > 1 {
		repo = hash[0]
	} else if tag := strings.Split(entry, ":"); len(tag) > 1 {
		repo = tag[0]
	}

	err := setAuthnKeychain()
	exit.OnError(err)

	options := crane.NilOption

	manifest, err := crane.Manifest(entry, options)
	if err != nil {
		warn.OnError(errors.Wrap(err, "Cannot extract manifest"))
		return nil
	}

	release := unstructured.Unstructured{}
	err = json.Unmarshal(manifest, &release.Object)
	exit.OnError(err)

	layers, _, err := unstructured.NestedSlice(release.Object, "layers")
	exit.OnError(err)

	last := layers[len(layers)-1]

	digest := last.(map[string]interface{})["digest"].(string)

	layer, err := crane.PullLayer(repo+"@"+digest, options)
	exit.OnError(err)

	return layer
}

func ExtractToolkitRelease(layer v1.Layer) (DriverToolkitEntry, error) {

	targz, err := layer.Compressed()
	defer dclose(targz)
	exit.OnError(err)

	gr, err := gzip.NewReader(targz)
	defer dclose(gr)
	exit.OnError(err)

	tr := tar.NewReader(gr)

	var dtk DriverToolkitEntry

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}

		if header.Name == "etc/driver-toolkit-release.json" {
			buff, err := io.ReadAll(tr)
			exit.OnError(err)

			obj := unstructured.Unstructured{}

			err = json.Unmarshal(buff, &obj.Object)
			exit.OnError(err)

			entry, _, err := unstructured.NestedString(obj.Object, "KERNEL_VERSION")
			exit.OnError(err)
			log.Info("DTK", "kernel-version", entry)
			dtk.KernelFullVersion = entry

			entry, _, err = unstructured.NestedString(obj.Object, "RT_KERNEL_VERSION")
			exit.OnError(err)
			log.Info("DTK", "rt-kernel-version", entry)
			dtk.RTKernelFullVersion = entry

			entry, _, err = unstructured.NestedString(obj.Object, "RHEL_VERSION")
			exit.OnError(err)
			log.Info("DTK", "rhel-version", entry)
			dtk.OSVersion = entry

			return dtk, err
		}

	}

	return dtk, errors.New("Missing driver toolkit entry: /etc/driver-toolkit-release.json")
}

func ReleaseManifests(layer v1.Layer) (key string, value string) {

	targz, err := layer.Compressed()
	defer dclose(targz)
	exit.OnError(err)

	gr, err := gzip.NewReader(targz)
	defer dclose(gr)
	exit.OnError(err)

	tr := tar.NewReader(gr)

	version := ""
	imageURL := ""

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}

		if header.Name == "release-manifests/image-references" {

			buff, err := io.ReadAll(tr)
			exit.OnError(err)

			obj := unstructured.Unstructured{}

			err = json.Unmarshal(buff, &obj.Object)
			exit.OnError(err)

			tags, _, err := unstructured.NestedSlice(obj.Object, "spec", "tags")
			exit.OnError(err)

			for _, tag := range tags {
				if tag.(map[string]interface{})["name"] == "driver-toolkit" {
					from := tag.(map[string]interface{})["from"]
					imageURL = from.(map[string]interface{})["name"].(string)
				}
			}

		}

		if header.Name == "release-manifests/release-metadata" {

			buff, err := io.ReadAll(tr)
			exit.OnError(err)

			obj := unstructured.Unstructured{}

			err = json.Unmarshal(buff, &obj.Object)
			exit.OnError(err)

			version, _, err = unstructured.NestedString(obj.Object, "version")
			exit.OnError(err)
		}

		if version != "" && imageURL != "" {
			break
		}

	}

	return version, imageURL
}

func dclose(c io.Closer) {
	if err := c.Close(); err != nil {
		warn.OnError(err)
		//log.Error(err)
	}
}

func setAuthnKeychain() error {
	var err error
	pullSecretNamespace := "openshift-config"

	_, err = clients.Interface.CoreV1().Namespaces().Get(context.TODO(), pullSecretNamespace, metav1.GetOptions{})

	if err != nil {
		log.Info("Cannot find namespace for pull-secret, assuming vanilla k8s")
		return nil
	} else {
		authn.DefaultKeychain, err = k8schain.NewInCluster(context.TODO(), k8schain.Options{
			Namespace:          "openshift-config",
			ServiceAccountName: "default",
			ImagePullSecrets: []string{
				"pull-secret",
			},
		})
		if err != nil {
			return errors.Wrap(err, "Cannot set authn.DefaultKeychain")
		}
	}
	return nil
}
