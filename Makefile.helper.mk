
# SRO-specific options

SPECIALRESOURCE  ?= driver-container-base
NAMESPACE        ?= special-resource-operator
PULLPOLICY       ?= IfNotPresent
TAG              ?= $(shell git rev-parse --abbrev-ref HEAD)
CSPLIT           ?= csplit - --prefix="" --suppress-matched --suffix-format="%04d.yaml"  /---/ '{*}' --silent
YAMLFILES        ?= $(shell  find manifests charts -name "*.yaml"  -not \( -path "charts/lustre/lustre-aws-fsx-0.0.1/csi-driver/*" -prune \)  -not \( -path "charts/*/shipwright-*/*" -prune \) -not \( -path "charts/experimental/*" -prune \) )
CONTAINER_COMMAND := $(or ${CONTAINER_COMMAND},podman)
CLUSTER_CLIENT := $(or ${CLUSTER_CLIENT},kubectl)
KUBECONFIG       ?= ${HOME}/.kube/config

export PATH := go/bin:$(PATH)

kube-lint: kube-linter
	$(KUBELINTER) lint $(YAMLFILES)

lint: golangci-lint
	$(GOLANGCILINT) run --modules-download-mode readonly -v --timeout 5m0s
	shellcheck helm-plugins/file-getter/cat-wrapper

verify: vet
	if [ `gofmt -l . | wc -l` -ne 0 ]; then \
		echo There are some malformated files, please make sure to run \'make fmt\'; \
		exit 1; \
	fi

e2e-test:
	for d in basic; do \
          KUBERNETES_CONFIG="$(KUBECONFIG)" NAMESPACE=$(NAMESPACE) go test -v -timeout 40m ./test/e2e/$$d -ginkgo.v -ginkgo.noColor -ginkgo.failFast || exit; \
        done

# Download kube-linter locally if necessary
KUBELINTER = $(shell pwd)/bin/kube-linter
kube-linter:
	$(call go-get-tool,$(KUBELINTER),golang.stackrox.io/kube-linter/cmd/kube-linter@v0.0.0-20210328011908-cb34f2cc447f)

# Download golangci-lint locally if necessary
GOLANGCILINT = $(shell pwd)/bin/golangci-lint
golangci-lint:
	$(call go-get-tool,$(GOLANGCILINT),github.com/golangci/golangci-lint/cmd/golangci-lint@v1.43.0)

# Additional bundle options for ART
DEFAULT_CHANNEL="4.9"
CHANNELS="4.9"

update-bundle:
	mv $$(find bundle -name image-references) bundle/image-references
	rm -rf bundle/4.*/manifests bundle/4.*/metadata
	$(MAKE) bundle DEFAULT_CHANNEL=$(DEFAULT_CHANNEL) VERSION=$(VERSION) IMAGE=$(IMG)
	mv bundle/manifests/special-resource-operator.clusterserviceversion.yaml bundle/manifests/special-resource-operator.v$(VERSION).clusterserviceversion.yaml
	mv bundle/manifests bundle/$(DEFAULT_CHANNEL)/manifests
	mv bundle/metadata bundle/$(DEFAULT_CHANNEL)/metadata
	sed 's#bundle/##g' bundle.Dockerfile | head -n -1 > bundle/$(DEFAULT_CHANNEL)/bundle.Dockerfile
	mv bundle/image-references bundle/$(DEFAULT_CHANNEL)/manifests/image-references
