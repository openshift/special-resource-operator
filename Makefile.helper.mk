
# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.5.0 ;\
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


golangci-lint:
ifeq (, $(shell which golangci-lint))
	@{ \
	set -e ;\
	GOLINT_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$GOLINT_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get github.com/golangci/golangci-lint/cmd/golangci-lint@v1.33.0 ;\
	rm -rf $$GOLINT_GEN_TMP_DIR ;\
	}
GOLANGCILINT=$(GOBIN)/golangci-lint
else
GOLANGCILINT=$(shell which golangci-lint)
endif

kube-linter:
ifeq (, $(shell which kube-linter))
	@{ \
	set -e ;\
	KUBELINTER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$KUBELINTER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get golang.stackrox.io/kube-linter/cmd/kube-linter@v0.0.0-20210328011908-cb34f2cc447f ;\
	rm -rf $$KUBELINTER_GEN_TMP_DIR ;\
	}
KUBELINTER=$(GOBIN)/kube-linter
else
KUBELINTER=$(shell which kube-linter)
endif

update-bundle: 
	rm -rf bundle/4.*/manifests bundle/4.*/metadata
	$(MAKE) bundle DEFAULT_CHANNEL=$(DEFAULT_CHANNEL) VERSION=$(VERSION) IMAGE=$(IMAGE)
	mv bundle/manifests/special-resource-operator.clusterserviceversion.yaml bundle/manifests/special-resource-operator.v$(VERSION).clusterserviceversion.yaml
	mv bundle/manifests bundle/$(DEFAULT_CHANNEL)/manifests
	mv bundle/metadata bundle/$(DEFAULT_CHANNEL)/metadata
	sed 's#bundle/##g' bundle.Dockerfile | head -n -1 > bundle/$(DEFAULT_CHANNEL)/bundle.Dockerfile
	rm -f bundle.Dockerfile

