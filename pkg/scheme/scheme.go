package scheme

import (
	buildV1 "github.com/openshift/api/build/v1"
	ocpconfigv1 "github.com/openshift/api/config/v1"
	imageV1 "github.com/openshift/api/image/v1"
	routev1 "github.com/openshift/api/route/v1"
	secv1 "github.com/openshift/api/security/v1"
	monitoringV1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

// AddToScheme Adds 3rd party resources To the operator
func AddToScheme(scheme *runtime.Scheme) error {

	utilruntime.Must(ocpconfigv1.AddToScheme(scheme))
	utilruntime.Must(routev1.AddToScheme(scheme))
	utilruntime.Must(secv1.AddToScheme(scheme))
	utilruntime.Must(buildV1.AddToScheme(scheme))
	utilruntime.Must(imageV1.AddToScheme(scheme))
	utilruntime.Must(monitoringV1.AddToScheme(scheme))

	return nil
}
