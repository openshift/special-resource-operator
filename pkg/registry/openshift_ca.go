package registry

import (
	"context"
	"fmt"

	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

type openShiftCAGetter struct {
	kubeClient *clients.ClientsInterface
}

//go:generate mockgen -source=openshift_ca.go -package=registry -destination=mock_openshift_ca_api.go

type OpenShiftCAGetter interface {
	AdditionalTrustedCAs(ctx context.Context) (map[string][]byte, error)
	CABundle(ctx context.Context) ([]byte, error)
}

func NewOpenShiftCAGetter(kubeClient *clients.ClientsInterface) OpenShiftCAGetter {
	return &openShiftCAGetter{kubeClient: kubeClient}
}

func (ocg *openShiftCAGetter) AdditionalTrustedCAs(ctx context.Context) (map[string][]byte, error) {
	logger := ctrl.LoggerFrom(ctx)

	var certs map[string][]byte

	const imageClusterName = "cluster"

	logger.Info("Getting image config", "name", imageClusterName)

	img, err := ocg.kubeClient.ConfigV1Client.Images().Get(ctx, imageClusterName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("could not retrieve image.config.openshift.io/%s: %w", imageClusterName, err)
	}

	if cmName := img.Spec.AdditionalTrustedCA.Name; cmName != "" {
		logger.Info("Getting ConfigMap", "cm-name", cmName, "cm-namespace", configNamespace)

		cm, err := ocg.kubeClient.CoreV1().ConfigMaps(configNamespace).Get(ctx, cmName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("could not retrieve configmap %s/%s: %w", configNamespace, cmName, err)
		}

		certs = make(map[string][]byte, len(cm.Data))

		for registry, cert := range cm.Data {
			certs[registry] = []byte(cert)
		}
	}

	return certs, nil

}

func (ocg *openShiftCAGetter) CABundle(ctx context.Context) ([]byte, error) {
	logger := ctrl.LoggerFrom(ctx)

	const userCABundleCMName = "user-ca-bundle"

	cmLogger := logger.WithValues("cm-name", userCABundleCMName, "cm-namespace", configNamespace)

	cmLogger.Info("Getting ConfigMap")

	userCABundle, err := ocg.kubeClient.CoreV1().ConfigMaps(configNamespace).Get(ctx, userCABundleCMName, metav1.GetOptions{})
	if err != nil {
		// It's okay if that ConfigMap does not exist.
		if !k8serrors.IsNotFound(err) {
			return nil, fmt.Errorf("could not get ConfigMap %s/%s: %v", configNamespace, userCABundleCMName, err)
		}

		cmLogger.Info("No such ConfigMap; skipping")
		return nil, nil
	}

	const caBundleKey = "ca-bundle.crt"

	if data, ok := userCABundle.Data[caBundleKey]; ok {
		return []byte(data), nil
	}

	cmLogger.Info("No such key; skipping", "key", caBundleKey)
	return nil, nil
}
