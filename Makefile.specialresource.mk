REP ?= example
VERSION ?= 0.0.1
NS ?= $(SPECIALRESOURCE)
SR ?= $(SPECIALRESOURCE)

TMP := $(shell mktemp -d)

$(SPECIALRESOURCE):
	$(CLUSTER_CLIENT) apply -f build/charts/$(REPO)/$(SR)-$(VERSION)/$(SR).yaml


chart: helm-repo-index
	@cp build/charts/$(REPO)/$(SR)-$(VERSION).tgz $(TMP)/.
	@helm repo index $(TMP) --url=cm://$(NS)/$(SR)-chart
	@$(CLUSTER_CLIENT) create ns $(NS) --dry-run=client -o yaml | $(CLUSTER_CLIENT) apply -f -
	@$(CLUSTER_CLIENT) create cm $(SR)-chart --from-file=$(TMP)/index.yaml --from-file=$(TMP)/$(SR)-$(VERSION).tgz --dry-run=client -o yaml -n $(NS) | $(CLUSTER_CLIENT) apply -f -



