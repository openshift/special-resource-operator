SPECIALRESOURCE  ?= driver-container-base
NAMESPACE        ?= openshift-sro
PULLPOLICY       ?= IfNotPresent
REGISTRY         ?= quay.io
ORG              ?= openshift-psap
TAG              ?= $(shell git branch | grep \* | cut -d ' ' -f2)
IMAGE            ?= $(REGISTRY)/$(ORG)/special-resource-operator:$(TAG)
CSPLIT           ?= csplit - --prefix="" --suppress-matched --suffix-format="%04d_specialresource_cvo_manifest.yaml"  /---/ '{*}' 1>/dev/null
export PATH := go/bin:$(PATH)
include config/recipes/Makefile

verify: fmt
unit: 
	@echo "##################### TODO UNIT TEST"

test-e2e:
	for d in basic; do \
	  KUBERNETES_CONFIG="$(KUBECONFIG)" go test -v -timeout 40m ./test/e2e/$$d -ginkgo.v -ginkgo.noColor -ginkgo.failFast || exit; \
	done


# Current Operator version
VERSION ?= v0.0.1
# Default bundle image tag
BUNDLE_IMG ?= controller-bundle:$(VERSION)
# Options for 'bundle-build'
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# Image URL to use all building/pushing image targets
IMG ?= $(IMAGE)
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
test: # generate fmt vet manifests
	go test ./... -coverprofile cover.out

# Build manager binary
manager: generate fmt vet
	go build -mod=vendor -o bin/manager main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet manifests-gen
	go run -mod=vendor ./main.go

# Install CRDs into a cluster
install: manifests-gen kustomize
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

# Uninstall CRDs from a cluster
uninstall: manifests-gen kustomize
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

configure: 
	# TODO kustomize cannot set name of namespace according to settings, hack TODO
	cd config/namespace && sed -i 's/name: .*/name: ${NAMESPACE}/g' namespace.yaml
	cd config/namespace && $(KUSTOMIZE) edit set namespace ${NAMESPACE}
	#cd config/default && $(KUSTOMIZE) edit set namespace ${NAMESPACE}
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}

manifests: manifests-gen kustomize configure 
	cd $@; $(KUSTOMIZE) build ../config/namespace | $(CSPLIT)
	
# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	$(KUSTOMIZE) build config/namespace | kubectl apply -f -

undeploy: kustomize
	$(KUSTOMIZE) build config/default | kubectl delete -f -


# Generate manifests e.g. CRD, RBAC etc.
manifests-gen: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

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
local-image-build: test manifests
	podman build --no-cache . -t ${IMG}

# Push the docker image
local-image-push:
	podman push ${IMG}

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.3.0 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

kustomize:
ifeq (, $(shell which kustomize))
	@{ \
	set -e ;\
	KUSTOMIZE_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$KUSTOMIZE_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/kustomize/kustomize/v3@v3.5.4 ;\
	rm -rf $$KUSTOMIZE_GEN_TMP_DIR ;\
	}
KUSTOMIZE=$(GOBIN)/kustomize
else
KUSTOMIZE=$(shell which kustomize)
endif

# Generate bundle manifests and metadata, then validate generated files.
.PHONY: bundle
bundle: manifests-gen
	operator-sdk generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/manifests | operator-sdk generate bundle -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)
	operator-sdk bundle validate ./bundle

# Build the bundle image.
.PHONY: bundle-build
bundle-build:
	podman build -f bundle.Dockerfile -t $(BUNDLE_IMG) .
