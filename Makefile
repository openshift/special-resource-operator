REGISTRY       ?= quay.io
ORG            ?= openshift-psap
TAG            ?= $(shell git branch | grep \* | cut -d ' ' -f2)
IMAGE          ?= $(REGISTRY)/$(ORG)/special-resource-operator:$(TAG)
NAMESPACE      ?= openshift-sro
PULLPOLICY     ?= IfNotPresent
TEMPLATE_CMD    = sed 's+REPLACE_IMAGE+$(IMAGE)+g; s+REPLACE_NAMESPACE+$(NAMESPACE)+g; s+Always+$(PULLPOLICY)+'
DEPLOY_OBJECTS  = namespace.yaml service_account.yaml role.yaml role_binding.yaml operator.yaml
DEPLOY_CRD      = crds/sro.openshift.io_specialresources_crd.yaml 
DEPLOY_CR       = crds/sro_v1alpha1_specialresource_cr.yaml

PACKAGE         = github.com/openshift-psap/special-resource-operator
MAIN_PACKAGE    = $(PACKAGE)/cmd/manager

DOCKERFILE      = Dockerfile
ENVVAR          = GOOS=linux CGO_ENABLED=0
GOOS            = linux
GO111MODULE     = auto
GO_BUILD_RECIPE = GO111MODULE=$(GO111MODULE) GOOS=$(GOOS) go build -mod=vendor -o $(BIN) $(MAIN_PACKAGE)

TEST_RESOURCES  = $(shell mktemp -d)/test-init.yaml

BIN=$(lastword $(subst /, ,$(PACKAGE)))

GOFMT_CHECK=$(shell find . -not \( \( -wholename './.*' -o -wholename '*/vendor/*' \) -prune \) -name '*.go' | sort -u | xargs gofmt -s -l)

all: build

build:
	$(GO_BUILD_RECIPE)

test: verify
	go test ./cmd/... ./pkg/... -coverprofile cover.out

test-e2e: 
	@$(TEMPLATE_CMD) manifests/service_account.yaml > $(TEST_RESOURCES)
	echo -e "\n---\n" >> $(TEST_RESOURCES)
	@$(TEMPLATE_CMD) manifests/role.yaml >> $(TEST_RESOURCES)
	echo -e "\n---\n" >> $(TEST_RESOURCES)
	@$(TEMPLATE_CMD) manifests/role_binding.yaml >> $(TEST_RESOURCES)
	echo -e "\n---\n" >> $(TEST_RESOURCES)
	@$(TEMPLATE_CMD) manifests/operator.yaml >> $(TEST_RESOURCES)

	go test -v ./test/e2e/... -root $(PWD) -kubeconfig=$(KUBECONFIG) -tags e2e  -globalMan $(DEPLOY_CRD) -namespacedMan $(TEST_RESOURCES)

$(DEPLOY_CRD):
	@$(TEMPLATE_CMD) deploy/$@ | kubectl apply -f -

deploy-crd: $(DEPLOY_CRD) 
	@sleep 1 

deploy-objects: deploy-crd
	@for obj in $(DEPLOY_OBJECTS) $(DEPLOY_CR); do               \
		$(TEMPLATE_CMD) deploy/$$obj | kubectl apply -f - ; \
	done 

deploy: deploy-objects
	kubectl create configmap special-resource-operator-states -n $(NAMESPACE) --from-file=assets/
	@$(TEMPLATE_CMD) deploy/$(DEPLOY_CR) | kubectl apply -f -

undeploy:
	@for obj in $(DEPLOY_CRD) $(DEPLOY_CR) $(DEPLOY_OBJECTS); do  \
		$(TEMPLATE_CMD) deploy/$$obj | kubectl delete -f - ; \
	done	

verify:	verify-gofmt

verify-gofmt:
ifeq (, $(GOFMT_CHECK))
	@echo "verify-gofmt: OK"
else
	@echo "verify-gofmt: ERROR: gofmt failed on the following files:"
	@echo "$(GOFMT_CHECK)"
	@echo ""
	@echo "For details, run: gofmt -d -s $(GOFMT_CHECK)"
	@echo ""
	@exit 1
endif

clean:
	go clean
	rm -f $(BIN)

local-image:
	podman build --no-cache -t $(IMAGE) -f $(DOCKERFILE) .

local-image-push:
	podman push $(IMAGE) 

.PHONY: all build generate verify verify-gofmt clean test test-e2e local-image local-image-push $(DEPLOY_CRDS) grafana
