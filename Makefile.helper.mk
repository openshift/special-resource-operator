
# SRO-specific options

SPECIALRESOURCE  ?= driver-container-base
NAMESPACE        ?= openshift-special-resource-operator
PULLPOLICY       ?= IfNotPresent
TAG              ?= $(shell git branch --show-current)
CSPLIT           ?= csplit - --prefix="" --suppress-matched --suffix-format="%04d.yaml"  /^---/ '{*}' --silent
YAMLFILES        ?= $(shell  find manifests charts -name "*.yaml")
CONTAINER_COMMAND := $(or ${CONTAINER_COMMAND},podman)
BUNDLE_CONTAINER_COMMAND := $(or ${BUNDLE_CONTAINER_COMMAND},docker)
CLUSTER_CLIENT := $(or ${CLUSTER_CLIENT},oc)
KUBECONFIG       ?= ${HOME}/.kube/config

export PATH := go/bin:$(PATH)

kube-lint: kube-linter
	$(KUBELINTER) lint $(YAMLFILES)

lint: golangci-lint
	$(GOLANGCILINT) run -v --timeout 5m0s

verify: vet
	if [ `gofmt -l . | grep -v vendor | wc -l` -ne 0 ]; then \
		echo There are some malformated files, please make sure to run \'make fmt\'; \
		exit 1; \
	fi

go-deploy-manifests: manifests-gen
	go run test/deploy/deploy.go -path ./manifests

go-undeploy-manifests:
	go run test/undeploy/undeploy.go -path ./manifests

e2e-test-upgrade: go-deploy-manifests

e2e-test:
	$(CLUSTER_CLIENT) create namespace ping-pong
	$(CLUSTER_CLIENT) create namespace simple-kmod
	./scripts/make-cm-recipe charts/example/ping-pong-0.0.1/ ping-pong-chart ping-pong
	./scripts/make-cm-recipe charts/example/simple-kmod-0.0.1/ simple-kmod-chart simple-kmod
	KUBERNETES_CONFIG="$(KUBECONFIG)" NAMESPACE=$(NAMESPACE) go test -v -timeout 40m ./test/e2e/basic -ginkgo.v -ginkgo.noColor -ginkgo.failFast || exit;
	$(CLUSTER_CLIENT) delete namespace ping-pong
	$(CLUSTER_CLIENT) delete namespace simple-kmod

# Download kube-linter locally if necessary
KUBELINTER = $(shell pwd)/bin/kube-linter
kube-linter:
	$(call go-get-tool,$(KUBELINTER),golang.stackrox.io/kube-linter/cmd/kube-linter@v0.0.0-20210328011908-cb34f2cc447f)

# Download golangci-lint locally if necessary
GOLANGCILINT = $(shell pwd)/bin/golangci-lint
golangci-lint:
	$(call go-get-tool,$(GOLANGCILINT),github.com/golangci/golangci-lint/cmd/golangci-lint@v1.43.0)

# Additional bundle options for ART
DEFAULT_CHANNEL="4.11"
CHANNELS="4.11"

update-bundle:
	mv $$(find bundle -name image-references) bundle/image-references
	rm -rf bundle/$(DEFAULT_CHANNEL)/manifests bundle/$(DEFAULT_CHANNEL)/metadata
	$(MAKE) bundle DEFAULT_CHANNEL=$(DEFAULT_CHANNEL) VERSION=$(VERSION) IMG=$(IMG)
	mv bundle/manifests/openshift-special-resource-operator.clusterserviceversion.yaml bundle/manifests/openshift-special-resource-operator.v$(VERSION).clusterserviceversion.yaml
	mv bundle/manifests bundle/$(DEFAULT_CHANNEL)/manifests
	mv bundle/metadata bundle/$(DEFAULT_CHANNEL)/metadata
	sed 's#bundle/##g' bundle.Dockerfile | head -n -1 > bundle/$(DEFAULT_CHANNEL)/bundle.Dockerfile
	mv bundle/image-references bundle/$(DEFAULT_CHANNEL)/manifests/image-references
