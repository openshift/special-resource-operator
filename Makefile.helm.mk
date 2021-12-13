HELM_CHARTS_DIR = charts
HELM_BUILD_ROOT_DIR = build
HELM_BUILD_DIR = $(HELM_BUILD_ROOT_DIR)/$(HELM_CHARTS_DIR)
HELM_REPOS = $(shell ls -d $(HELM_BUILD_DIR)/*/)


helm-lint: helm helm-copy-charts
	echo $(HELM_REPOS)
	@for repo in $(HELM_REPOS); do                          \
		cd $$repo;                              \
		helm lint -f ../global-values.yaml `ls -d */`; \
		cd ../../..;                                    \
	done

helm-repo-index: helm-lint
	@for repo in $(HELM_REPOS); do                  \
		cd $$repo;                              \
		helm package `ls -d */`;                \
		file_url=`echo $$repo |sed 's/$(HELM_BUILD_ROOT_DIR)\///g'`;   \
		helm repo index . --url=file:///$$file_url; \
		cd ../../..;	                        \
	done


helm-copy-charts:
	rm -rf $(HELM_BUILD_DIR)
	mkdir -p $(HELM_BUILD_DIR)
	cp -r $(HELM_CHARTS_DIR)/* $(HELM_BUILD_DIR)


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
HELM=$(shell which helm)
endif
