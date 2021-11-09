module github.com/openshift-psap/special-resource-operator

go 1.16

require (
	github.com/go-logr/logr v0.4.0
	github.com/google/go-containerregistry v0.5.2-0.20210601193515-0ffa4a5c8691
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/mitchellh/hashstructure/v2 v2.0.1
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.13.0
	github.com/openshift/api v0.0.0-20210924154557-a4f696157341
	github.com/openshift/client-go v0.0.0-20210916133943-9acee1a0fb83
	github.com/openshift/library-go v0.0.0-20211103140146-29c9bb8362e2
	github.com/openshift/machine-config-operator v0.0.1-0.20210514234214-c415ce6aed25
	github.com/pkg/errors v0.9.1
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.42.1
	github.com/prometheus/client_golang v1.11.0
	github.com/stretchr/testify v1.7.0
	golang.org/x/oauth2 v0.0.0-20210427180440-81ed05c6b58c // indirect
	gopkg.in/yaml.v2 v2.4.0
	helm.sh/helm/v3 v3.6.0
	k8s.io/api v0.22.1
	k8s.io/apimachinery v0.22.1
	k8s.io/cli-runtime v0.21.0
	k8s.io/client-go v0.22.1
	rsc.io/letsencrypt v0.0.3 // indirect
	sigs.k8s.io/controller-runtime v0.9.0
	sigs.k8s.io/yaml v1.2.0
)
