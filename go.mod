module github.com/openshift-psap/special-resource-operator

go 1.14

require (
	github.com/go-logr/logr v0.2.1
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/onsi/ginkgo v1.12.1
	github.com/onsi/gomega v1.10.1
	github.com/openshift/api v0.0.0-20201005153912-821561a7f2a2
	github.com/openshift/client-go v0.0.0-20200827190008-3062137373b5
	github.com/openshift/cluster-node-tuning-operator v0.0.0-20201026145914-c8b2ed8012aa // indirect
	github.com/openshift/machine-config-operator v4.2.0-alpha.0.0.20190917115525-033375cbe820+incompatible // indirect
	github.com/pkg/errors v0.9.1
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.42.1
	github.com/prometheus/common v0.14.0 // indirect
	go4.org v0.0.0-20200411211856-f5505b9728dd // indirect
	golang.org/x/net v0.0.0-20201009032441-dbdefad45b89 // indirect
	k8s.io/api v0.19.2
	k8s.io/apimachinery v0.19.2
	k8s.io/client-go v0.19.0
	sigs.k8s.io/controller-runtime v0.6.3
	sigs.k8s.io/yaml v1.2.0
)
