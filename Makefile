SHELL             = /bin/bash
SPECIALRESOURCE  ?= driver-container-base
NAMESPACE        ?= openshift-special-resource-operator
PULLPOLICY       ?= IfNotPresent
TAG              ?= $(shell git branch --show-current)
IMAGE            ?= quay.io/openshift-psap/special-resource-operator:$(TAG)
CSPLIT           ?= csplit - --prefix="" --suppress-matched --suffix-format="%04d.yaml"  /---/ '{*}' --silent
YAMLFILES        ?= $(shell  find manifests${SUFFIX} charts -name "*.yaml"  -not \( -path "charts/lustre/lustre-aws-fsx-0.0.1/csi-driver/*" -prune \)  -not \( -path "charts/*/shipwright-*/*" -prune \) -not \( -path "charts/experimental/*" -prune \) )
PLATFORM         ?= ""
SUFFIX           ?= $(shell if [ ${PLATFORM} == "k8s" ]; then echo "-${PLATFORM}"; fi)

export PATH := go/bin:$(PATH)
include Makefile.specialresource.mk
include Makefile.helm.mk
include Makefile.helper.mk

patch:
	cp .patches/options.patch.go vendor/github.com/google/go-containerregistry/pkg/crane/.
	cp .patches/getter.patch.go vendor/helm.sh/helm/v3/pkg/getter/.
	cp .patches/action.patch.go vendor/helm.sh/helm/v3/pkg/action/.
	cp .patches/install.patch.go vendor/helm.sh/helm/v3/pkg/action/.
	OUT="$(shell patch -p1 -N -i .patches/helm.patch)" || echo "${OUT}" | grep "Skipping patch" -q || (echo $OUT && false)

kube-lint: kube-linter
	$(KUBELINTER) lint $(YAMLFILES)

lint: patch golangci-lint
	$(GOLANGCILINT) run -v --timeout 5m0s

verify: patch fmt vet
unit:
	@echo "TODO UNIT TEST"

CRDS := $(shell echo manifests/[0-9][0-9][0-9][0-9]_customresourcedefinition_*.yaml)

deploy-manifests: manifests$(SUFFIX)
	# First, create CRDs
	for c in $(CRDS); do kubectl apply -f $${c}; done
	# Wait for CRDs to be established in the API server, otherwise adding the corresponding CRs could fail
	for c in $(CRDS); do kubectl wait --for condition=established --timeout=60s -f $${c}; done
	# Apply all the rest
	kubectl apply --wait -f ./manifests$(SUFFIX)

undeploy-manifests: manifests$(SUFFIX)
	kubectl delete --wait --ignore-not-found -f ./manifests$(SUFFIX)

# TODO: obsolete targets; remove
go-deploy-manifests: deploy-manifests
go-undeploy-manifests: undeploy-manifests

test-e2e:
	for d in basic; do \
          KUBERNETES_CONFIG="$(KUBECONFIG)" go test -v -timeout 40m --tags=e2e ./test/e2e/$$d -ginkgo.v -ginkgo.noColor -ginkgo.failFast || exit; \
        done

# Current Operator version
VERSION ?= 0.0.1

CHANNELS="4.9"

# Default bundle image tag
BUNDLE_IMG ?= quay.io/openshift-psap/special-resource-operator-bundle:$(VERSION)
# Options for 'bundle-build'
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif

DEFAULT_CHANNEL="4.9"

ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:crdVersions=v1,trivialVersions=true"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# GENERATED all: manager
all: $(SPECIALRESOURCE)

# Run tests
test: patch # generate fmt vet manifests-gen
	go test ./... -coverprofile cover.out

# Build manager binary
manager: patch generate fmt vet
	go build -mod=vendor -o /tmp/bin/manager main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet manifests-gen
	go run -mod=vendor ./main.go

configure:
	# TODO kustomize cannot set name of namespace according to settings, hack TODO
	cd config/namespace && sed -i 's/name: .*/name: $(NAMESPACE)/g' namespace.yaml
	cd config/namespace && $(KUSTOMIZE) edit set namespace $(NAMESPACE)
	cd config/default && $(KUSTOMIZE) edit set namespace $(NAMESPACE)
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMAGE)

namespace: patch manifests$(SUFFIX)
	$(KUSTOMIZE) build config/namespace | kubectl apply -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: namespace
	$(KUSTOMIZE) build config/default$(SUFFIX) | kubectl apply -f -
	$(shell sleep 5)
	$(KUSTOMIZE) build config/cr | kubectl apply -f -


# If the CRD is deleted before the CRs the CRD finalizer will hang forever
# The specialresource finalizer will not execute either
undeploy: kustomize
	if [ ! -z "$$(kubectl get crd | grep specialresource)" ]; then         \
		kubectl delete --ignore-not-found sr --all;                    \
	fi;
	# Give SRO time to reconcile
	sleep 10
	$(KUSTOMIZE) build config/namespace | kubectl delete --ignore-not-found -f -
	$(KUSTOMIZE) build config/default$(SUFFIX) | kubectl delete --ignore-not-found -f -


# Generate manifests-gen e.g. CRD, RBAC etc.
manifests-gen: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases


manifests$(SUFFIX): manifests-gen kustomize configure
	cd $@; rm -f *.yaml
	cd $@; ( $(KUSTOMIZE) build ../config/namespace && echo "---" && $(KUSTOMIZE) build ../config/default$(SUFFIX) ) | $(CSPLIT)
	cd $@; bash ../scripts/rename.sh
	cd $@; $(KUSTOMIZE) build ../config/cr > 0016_specialresource_special-resource-preamble.yaml

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet --mod=vendor ./...

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

# Build the docker image
local-image-build: patch helm-lint helm-repo-index test generate manifests-gen
	podman build -f Dockerfile.ubi8 --no-cache . -t $(IMAGE)

# Push the docker image
local-image-push:
	podman push $(IMAGE)


# Generate bundle manifests-gen and metadata, then validate generated files.
.PHONY: bundle
bundle: manifests-gen
	operator-sdk generate kustomize manifests$(SUFFIX) -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMAGE)
	$(KUSTOMIZE) build config/manifests$(SUFFIX) | operator-sdk generate bundle -q --overwrite --verbose --version $(VERSION) $(BUNDLE_METADATA_OPTS)
	operator-sdk bundle validate ./bundle

# Build the bundle image.
.PHONY: bundle-build
bundle-build:
	podman build -f bundle.Dockerfile -t $(BUNDLE_IMG) .
