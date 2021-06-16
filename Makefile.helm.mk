HELM_REPOS = $(shell ls -d charts/*/)


helm-lint: helm
	echo $(HELM_REPOS)
	@for repo in $(HELM_REPOS); do                          \
		cd $$repo;                              \
		helm lint -f ../global-values.yaml `ls -d */`; \
		cd ../..;                                      \
	done

helm-repo-index: helm-lint
	@for repo in $(HELM_REPOS); do                          \
		cd $$repo;                              \
		helm package `ls -d */`;                       \
		helm repo index . --url=file:///$$repo; \
		cd ../..;                                      \
	done



helm:
ifeq (, $(shell which helm))
	@{ \
	set -e ;\
	HELM_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$HELM_GEN_TMP_DIR ;\
	curl https://get.helm.sh/helm-v3.6.0-linux-amd64.tar.gz -o helm.tar.gz ;\
	tar xvfpz helm.tar.gz ;\
	mv linux-amd64/helm /usr/local/bin ;\
	chmod +x /usr/local/bin/helm ;\
	rm -rf $$HELM_GEN_TMP_DIR ;\
	}
HELM=/usr/local/bin/helm
else
KUSTOMIZE=$(shell which helm)
endif
