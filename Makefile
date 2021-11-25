include Makefile.specialresource.mk
include Makefile.helm.mk
# For SRO specific options see:
include Makefile.helper.mk

VERSION ?= 0.0.1

# CHANNELS define the bundle channels used in the bundle.
# Add a new line here if you would like to change its default config. (E.g CHANNELS = "candidate,fast,stable")
# To re-generate a bundle for other specific channels without changing the standard setup, you can:
# - use the CHANNELS as arg of the bundle target (e.g make bundle CHANNELS=candidate,fast,stable)
# - use environment variables to overwrite this value (e.g export CHANNELS="candidate,fast,stable")
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif

# DEFAULT_CHANNEL defines the default channel used in the bundle.
# Add a new line here if you would like to change its default config. (E.g DEFAULT_CHANNEL = "stable")
# To re-generate a bundle for any other default channel without changing the default setup, you can:
# - use the DEFAULT_CHANNEL as arg of the bundle target (e.g make bundle DEFAULT_CHANNEL=stable)
# - use environment variables to overwrite this value (e.g export DEFAULT_CHANNEL="stable")
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# IMAGE_TAG_BASE defines the docker.io namespace and part of the image name for remote images.
# This variable is used to construct full image tags for bundle and catalog images.
#
# For example, running 'make bundle-build bundle-push catalog-build catalog-push' will build and push both
# my.domain/ostest-bundle:$VERSION and my.domain/ostest-catalog:$VERSION.
IMAGE_TAG_BASE ?= quay.io/openshift-psap/special-resource-operator

# BUNDLE_IMG defines the image:tag used for the bundle.
# You can use it as an arg. (E.g make bundle-build BUNDLE_IMG=<some-registry>/<project-name-bundle>:<tag>)
BUNDLE_IMG ?= $(IMAGE_TAG_BASE)-bundle:v$(VERSION)

# Image URL to use all building/pushing image targets
IMG ?= quay.io/openshift-psap/special-resource-operator:$(TAG)
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true,preserveUnknownFields=false"
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.21

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL             = /usr/bin/env bash -o pipefail
.SHELLFLAGS       = -ec

# GENERATED all: manager
all: $(SPECIALRESOURCE)

namespace: patch manifests kustomize
	$(KUSTOMIZE) build config/namespace | kubectl apply -f -

##@ Development

manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

fmt: ## Run go fmt against code.
	go fmt ./...

vet: ## Run go vet against code.
	go vet --mod=vendor ./...

test: patch manifests generate fmt envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test ./... -coverprofile cover.out

##@ Build

manager: patch generate fmt ## Build manager binary.
	go build -mod=vendor -o /tmp/bin/manager main.go

run: manifests generate fmt ## Run against the configured Kubernetes cluster in ~/.kube/config
	go run -mod=vendor ./main.go

local-image-build: patch helm-lint helm-repo-index generate manifests-gen ## Build container image with the manager.
	$(CONTAINER_COMMAND) build -t $(IMG) -f Dockerfile.ubi8 --no-cache .

local-image-push: ## Push docker image with the manager.
	$(CONTAINER_COMMAND) push $(IMG)

generate-mocks:
	$(shell find . -name "mock_*.go" | grep -v vendor | xargs rm -f)
	go generate $(shell go list ./... | grep -v 'vendor\|charts')

##@ Deployment

install: manifests kustomize  ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

uninstall: manifests kustomize  ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

deploy: namespace ## Deploy controller in the configured Kubernetes cluster in ~/.kube/config
	$(KUSTOMIZE) build config/default$(SUFFIX) | kubectl apply -f -
	$(shell sleep 5)
	$(KUSTOMIZE) build config/cr | kubectl apply -f -

# If the CRD is deleted before the CRs the CRD finalizer will hang forever
# The specialresource finalizer will not execute either
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	if [ ! -z "$$(kubectl get crd | grep specialresource)" ]; then         \
		kubectl delete --ignore-not-found sr --all;                    \
	fi;
	# Give SRO time to reconcile
	sleep 10
	$(KUSTOMIZE) build config/namespace | kubectl delete --ignore-not-found -f -
	$(KUSTOMIZE) build config/default$(SUFFIX) | kubectl delete --ignore-not-found -f -


##@ custom targets

MANIFEST_DIR = manifests$(SUFFIX)
MANIFEST_BUNDLE := $(shell mktemp)

# Populate manifests dir, and SRO specific customizations
manifests-gen: manifests kustomize configure
	mkdir -p $(MANIFEST_DIR)
	rm -f $(MANIFEST_DIR)/*.yaml

	$(KUSTOMIZE) build config/namespace > $(MANIFEST_BUNDLE)
	echo '---' >> $(MANIFEST_BUNDLE)
	$(KUSTOMIZE) build config/default$(SUFFIX) >> $(MANIFEST_BUNDLE)
	echo '---' >> $(MANIFEST_BUNDLE)
	$(KUSTOMIZE) build config/cr >> $(MANIFEST_BUNDLE)

	cd $(MANIFEST_DIR); $(CSPLIT) < $(MANIFEST_BUNDLE)
	cd $(MANIFEST_DIR); bash ../scripts/rename.sh

	rm $(MANIFEST_BUNDLE)

# SRO specific configuration to set namespace of all manifests
configure:
	# TODO kustomize cannot set name of namespace according to settings, hack TODO
	cd config/namespace && sed -i 's/name: .*/name: $(NAMESPACE)/g' namespace.yaml
	cd config/namespace && $(KUSTOMIZE) edit set namespace $(NAMESPACE)
	cd config/default && $(KUSTOMIZE) edit set namespace $(NAMESPACE)
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)


CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.6.1)

KUSTOMIZE = $(shell pwd)/bin/kustomize
kustomize: ## Download kustomize locally if necessary.
	$(call go-get-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v3@v3.8.7)

ENVTEST = $(shell pwd)/bin/setup-envtest
envtest: ## Download envtest-setup locally if necessary.
	$(call go-get-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest@latest)

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go get $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef

.PHONY: bundle
bundle: manifests kustomize ## Generate bundle manifests and metadata, then validate generated files.
	operator-sdk generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/manifests | operator-sdk generate bundle -q --overwrite --verbose --version $(VERSION) $(BUNDLE_METADATA_OPTS)
	operator-sdk bundle validate ./bundle

.PHONY: bundle-build
bundle-build: ## Build the bundle image.
	docker build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

.PHONY: bundle-push
bundle-push: ## Push the bundle image.
	$(MAKE) docker-push IMG=$(BUNDLE_IMG)

.PHONY: opm
OPM = ./bin/opm
opm: ## Download opm locally if necessary.
ifeq (,$(wildcard $(OPM)))
ifeq (,$(shell which opm 2>/dev/null))
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPM)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/v1.15.1/$${OS}-$${ARCH}-opm ;\
	chmod +x $(OPM) ;\
	}
else
OPM = $(shell which opm)
endif
endif

# The image tag given to the resulting catalog image (e.g. make catalog-build CATALOG_IMG=example.com/operator-catalog:v0.2.0).
CATALOG_IMG ?= quay.io/openshift-psap/sro-catalog:v$(VERSION)

# Set CATALOG_BASE_IMG to an existing catalog image tag to add $BUNDLE_IMGS to that image.
ifneq ($(origin CATALOG_BASE_IMG), undefined)
FROM_INDEX_OPT := --from-index $(CATALOG_BASE_IMG)
endif

# Build a catalog image by adding bundle images to an empty catalog using the operator package manager tool, 'opm'.
# This recipe invokes 'opm' in 'semver' bundle add mode. For more information on add modes, see:
# https://github.com/operator-framework/community-operators/blob/7f1438c/docs/packaging-operator.md#updating-your-existing-operator
.PHONY: catalog-build
catalog-build: opm ## Build a catalog image.
	$(OPM) index add --container-tool $(CONTAINER_COMMAND) --mode semver --tag $(CATALOG_IMG) --bundles $(BUNDLE_IMGS) $(FROM_INDEX_OPT)

# Push the catalog image.
.PHONY: catalog-push
catalog-push: ## Push a catalog image.
	$(MAKE) local-image-push IMG=$(CATALOG_IMG)
