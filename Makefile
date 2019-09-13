REGISTRY       ?= quay.io
ORG            ?= zvonkok
TAG            ?= latest #$(shell git rev-parse --short HEAD)
IMAGE          ?= ${REGISTRY}/${ORG}/special-resource-operator:${TAG}
NAMESPACE      ?= openshift-sro-operator
PULLPOLICY     ?= IfNotPresent
TEMPLATE_CMD    = sed 's+REPLACE_IMAGE+${IMAGE}+g; s+REPLACE_NAMESPACE+${NAMESPACE}+g; s+IfNotPresent+${PULLPOLICY}+'
DEPLOY_SCC_RO   = manifests/0310_readonlyfs_scc.yaml
DEPLOY_OBJECTS  = manifests/0000_namespace.yaml manifests/0010_namespace.yaml manifests/0100_service_account.yaml manifests/0200_role.yaml manifests/0300_role_binding.yaml manifests/0400_operator.yaml
DEPLOY_CRDS     = manifests/0500_sro_crd.yaml
DEPLOY_CRS_NONE = manifests/0600_sro_cr_sched_none.yaml
DEPLOY_CRS_PRIO = manifests/0600_sro_cr_sched_priority_preemption.yaml
DEPLOY_CRS_TTOL = manifests/0600_sro_cr_sched_taints_tolerations.yaml

PROM_URL       = $(shell oc get secrets -n openshift-monitoring grafana-datasources -o go-template='{{index .data "prometheus.yaml"}}' | base64 --decode | jq '.datasources[0].url')
PROM_USER      ?= internal
PROM_PASS      = $(shell oc get secrets -n openshift-monitoring grafana-datasources -o go-template='{{index .data "prometheus.yaml"}}' | base64 --decode | jq '.datasources[0].basicAuthPassword')
GRAFANA_CMD     = sed 's|REPLACE_PROM_URL|${PROM_URL}|g; s|REPLACE_PROM_USER|${PROM_USER}|g; s|REPLACE_PROM_PASS|${PROM_PASS}|g;'

PACKAGE=github.com/zvonkok/special-resource-operator
MAIN_PACKAGE=$(PACKAGE)/cmd/manager

BIN=$(lastword $(subst /, ,$(PACKAGE)))
BINDATA=pkg/manifests/bindata.go

GOFMT_CHECK=$(shell find . -not \( \( -wholename './.*' -o -wholename '*/vendor/*' \) -prune \) -name '*.go' | sort -u | xargs gofmt -s -l)

DOCKERFILE=Dockerfile
IMAGE_TAG=zvonkok/special-resource-operator
IMAGE_REGISTRY=quay.io

ENVVAR=GOOS=linux CGO_ENABLED=0
GOOS=linux
GO_BUILD_RECIPE=GOOS=$(GOOS) go build -o $(BIN) $(MAIN_PACKAGE)

all: build

grafana:
	kubectl apply -f assets/state-grafana/0100_grafana_service.yaml   
	oc apply -f assets/state-grafana/0200_grafana_route.yaml
	${GRAFANA_CMD}   assets/state-grafana/0300_grafana_configmap.yaml  | kubectl apply -f -
	kubectl apply -f assets/state-grafana/0400_grafana_deployment.yaml 

build:
	$(GO_BUILD_RECIPE)

test-e2e: 
	@${TEMPLATE_CMD} manifests/0110_namespace.yaml > manifests/operator-init.yaml
	echo -e "\n---\n" >> manifests/operator-init.yaml
	@${TEMPLATE_CMD} manifests/0200_service_account.yaml >> manifests/operator-init.yaml
	echo -e "\n---\n" >> manifests/operator-init.yaml
	@${TEMPLATE_CMD} manifests/0300_cluster_role.yaml >> manifests/operator-init.yaml
	echo -e "\n---\n" >> manifests/operator-init.yaml
	@${TEMPLATE_CMD} manifests/0600_operator.yaml >> manifests/operator-init.yaml

	go test -v ./test/e2e/... -root $(PWD) -kubeconfig=$(KUBECONFIG) -tags e2e  -globalMan $(DEPLOY_CRDS) -namespacedMan manifests/operator-init.yaml 

$(DEPLOY_CRDS):
	@${TEMPLATE_CMD} $@ | kubectl apply -f -

deploy-crds: $(DEPLOY_CRDS) 
	sleep 1

deploy-objects: deploy-crds
	for obj in $(DEPLOY_OBJECTS); do \
		$(TEMPLATE_CMD) $$obj | kubectl apply -f - ;\
	done	

deploy: deploy-objects
	@${TEMPLATE_CMD} $(DEPLOY_CRS_NONE) | kubectl apply -f -

deploy-prio: deploy-objects
	@${TEMPLATE_CMD} $(DEPLOY_CRS_PRIO) | kubectl apply -f -

deploy-ttol: deploy-objects
	@${TEMPLATE_CMD} $(DEPLOY_CRS_TTOL) | kubectl apply -f -

undeploy:
	for obj in $(DEPLOY_OBJECTS) $(DEPLOY_CRDS) $(DEPLOY_CRS_NONE) $(DEPLOY_CRS_PRIO) $(DEPLOY_CRS_TTOL); do \
		$(TEMPLATE_CMD) $$obj | kubectl delete -f - ;\
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
ifdef USE_BUILDAH
	buildah bud $(BUILDAH_OPTS) -t $(IMAGE_TAG) -f $(DOCKERFILE) .
else
	sudo docker build -t $(IMAGE_TAG) -f $(DOCKERFILE) .
endif

test:
	go test ./cmd/... ./pkg/... -coverprofile cover.out

local-image-push:
ifdef USE_BUILDAH
	buildah push $(BUILDAH_OPTS) $(IMAGE_TAG) $(IMAGE_REGISTRY)/$(IMAGE_TAG)
else
	sudo docker tag $(IMAGE_TAG) $(IMAGE_REGISTRY)/$(IMAGE_TAG)
	sudo docker push $(IMAGE_REGISTRY)/$(IMAGE_TAG)
endif

.PHONY: all build generate verify verify-gofmt clean local-image local-image-push $(DEPLOY_CRDS) grafana

