package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func getLogger() *log.Logger {
	w := io.Discard

	if os.Getenv("HELM_DEBUG") == "true" {
		w = os.Stderr
	}

	return log.New(w, os.Getenv("HELM_PLUGIN_NAME"), log.LstdFlags)
}

type ConfigMapGetter struct {
	kubeClient kubernetes.Interface
	logger     *log.Logger
}

func NewConfigMapProvider(logger *log.Logger, kubeConfigPath string) (*ConfigMapGetter, error) {
	kubeClient, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if err != nil {
		return nil, fmt.Errorf("could not create a Kubernetes client config: %v. Try setting the KUBECONFIG environment variable", err)
	}

	clientSet, err := kubernetes.NewForConfig(kubeClient)
	if err != nil {
		return nil, fmt.Errorf("could not create a new ClientSet: %v", err)
	}

	return &ConfigMapGetter{kubeClient: clientSet, logger: logger}, nil
}

func (cmg *ConfigMapGetter) Get(ctx context.Context, u *url.URL) ([]byte, error) {
	namespace := u.Host

	cmg.logger.Printf("Namespace: %v", namespace)

	pathElements := strings.Split(u.Path, "/")

	// pathElements should look like /<resource>/<element>
	// i.e. /cm-name/index.yaml (if Helm is trying to find the index file)
	// len(pathElements) should be 3 because of the leading slash
	if len(pathElements) != 3 {
		return nil, fmt.Errorf("%s: invalid path, should be NAMESPACE/NAME/ELEMENT", u.Path)
	}

	resourceName := pathElements[1]

	cmg.logger.Printf("CM name: %v", resourceName)

	elem := pathElements[2]

	cmg.logger.Printf("Element: %v", elem)

	cm, err := cmg.kubeClient.CoreV1().ConfigMaps(namespace).Get(ctx, resourceName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("could not GET ConfigMap %s/%s: %v", namespace, resourceName, err)
	}

	if elem == "index.yaml" {
		return []byte(cm.Data[elem]), nil
	}

	return cm.BinaryData[elem], nil
}

func main() {
	debugLogger := getLogger()

	debugLogger.Print("Environment:")

	for _, e := range os.Environ() {
		debugLogger.Print(e)
	}

	flag.Parse()

	debugLogger.Printf("Args: %v", flag.Args())

	urlArg := flag.Arg(flag.NArg() - 1)

	debugLogger.Printf("URL: %s", urlArg)

	u, err := url.Parse(urlArg)
	if err != nil {
		log.Fatalf("Cannot parse URL %q: %v", urlArg, err)
	}

	g, err := NewConfigMapProvider(debugLogger, os.Getenv("KUBECONFIG"))
	if err != nil {
		log.Fatalf("Could not create a ConfigMap getter: %v", err)
	}

	output, err := g.Get(context.Background(), u)
	if err != nil {
		log.Fatalf("Could not get %s: %v", u, err)
	}

	if _, err = os.Stdout.Write(output); err != nil {
		log.Fatalf("Error writing to the standard output: %v", err)
	}
}
