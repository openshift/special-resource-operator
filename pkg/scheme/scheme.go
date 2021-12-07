package scheme

import (
	"fmt"
	"reflect"
	"runtime"

	buildV1 "github.com/openshift/api/build/v1"
	ocpconfigv1 "github.com/openshift/api/config/v1"
	imageV1 "github.com/openshift/api/image/v1"
	routev1 "github.com/openshift/api/route/v1"
	secv1 "github.com/openshift/api/security/v1"
	monitoringV1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
)

// AddToScheme Adds 3rd party resources To the operator
func AddToScheme(scheme *k8sruntime.Scheme) error {
	installers := []func(s *k8sruntime.Scheme) error{
		ocpconfigv1.Install,
		routev1.Install,
		secv1.Install,
		buildV1.Install,
		imageV1.Install,
		monitoringV1.AddToScheme,
	}

	for _, i := range installers {
		if err := i(scheme); err != nil {
			funcName := runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
			return fmt.Errorf("error adding scheme with %s: %w", funcName, err)
		}
	}

	return nil
}
