module github.com/openshift-psap/special-resource-operator

go 1.16

require (
	github.com/go-logr/logr v0.4.0
	github.com/google/go-containerregistry v0.5.2-0.20210601193515-0ffa4a5c8691
	github.com/google/go-containerregistry/pkg/authn/k8schain v0.0.0-20210609162550-f0ce2270b3b4
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/maxbrunsfeld/counterfeiter/v6 v6.2.2 // indirect
	github.com/mitchellh/hashstructure/v2 v2.0.1
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.13.0
	github.com/openshift/api v0.0.0-20210409143810-a99ffa1cac67
	github.com/openshift/client-go v0.0.0-20210112165513-ebc401615f47
	github.com/openshift/library-go v0.0.0-20210205203934-9eb0d970f2f4
	github.com/openshift/machine-config-operator v0.0.1-0.20210514234214-c415ce6aed25
	github.com/pkg/errors v0.9.1
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.42.1
	github.com/prometheus/client_golang v1.11.0
	go.uber.org/multierr v1.6.0 // indirect
	helm.sh/helm/v3 v3.6.0
	k8s.io/api v0.21.1
	k8s.io/apimachinery v0.21.1
	k8s.io/client-go v0.21.1
	rsc.io/letsencrypt v0.0.3 // indirect
	sigs.k8s.io/controller-runtime v0.9.0
	sigs.k8s.io/yaml v1.2.0
)
