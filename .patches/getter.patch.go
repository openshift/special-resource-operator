package getter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"helm.sh/helm/v3/pkg/kube"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var fileProvider = Provider{
	Schemes: []string{"file"},
	New:     NewFileGetter,
}

type FileGetter struct {
}

func (g *FileGetter) Get(href string, option ...Option) (*bytes.Buffer, error) {

	ref := strings.TrimPrefix(href, "file://")

	if _, err := os.Stat(ref); err == nil {
		file, err := ioutil.ReadFile(ref)
		if err == nil {
			return bytes.NewBuffer(file), nil
		}

	} else if os.IsNotExist(err) {
		// path/to/whatever does *not* exist
		fmt.Printf("getter.go: ERROR FILE DOES NOT EXISTS %+v\n", err)
		os.Exit(1)

	} else {
		// Schrodinger: file may or may not exist. See err for details.
		// Therefore, do *NOT* use !os.IsNotExist(err) to test for file existence
		fmt.Printf("ERROR SCHROEDINGER FILE  %+v\n", err)
		os.Exit(1)
	}

	return nil, nil
}

func NewFileGetter(options ...Option) (Getter, error) {
	var client FileGetter

	return &client, nil
}

// ------------- ConfigMap Provider --------------------------------------------

var configMapProvider = Provider{
	Schemes: []string{"cm"},
	New:     NewConfigMapGetter,
}

type ConfigMapGetter struct {
	kc *kubernetes.Clientset
}

func (g *ConfigMapGetter) Get(href string, option ...Option) (*bytes.Buffer, error) {

	fmt.Printf("HREF %s\n", href)
	namespacedName := strings.TrimPrefix(href, "cm://")
	s := strings.Split(namespacedName, "/")
	if len(s) < 3 {
		return nil, errors.New("Malformed cm:// URL, cm://<NAMESPACE>/<NAME>")
	}

	opts := metav1.GetOptions{}

	chart, err := g.kc.CoreV1().ConfigMaps(s[0]).Get(context.TODO(), s[1], opts)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Cannot find ConfigMap with provided URL: cm://%s/%s", s[0], s[1]))
	}

	asciiData := chart.Data
	binaryData := chart.BinaryData

	if strings.Contains(s[2], "index.yaml") {

		if _, ok := asciiData["index.yaml"]; !ok {
			return nil, errors.New(fmt.Sprintf("Cannot find index.yaml in CM %+v\n", asciiData))
		}
		return bytes.NewBuffer([]byte(asciiData["index.yaml"])), nil
	}

	for k, v := range binaryData {
		if s[2] == k {
			return bytes.NewBuffer(v), nil
		}
	}

	return nil, errors.New(fmt.Sprintf("Cannot find any asciiData | binaryData in CM %+v\n", chart))

}

func NewConfigMapGetter(options ...Option) (Getter, error) {
	var client ConfigMapGetter
	var err error

	cl := kube.New(nil)
	client.kc, err = cl.Factory.KubernetesClientSet()
	if err != nil {
		panic(err)
	}

	return &client, nil
}
