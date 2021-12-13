REP ?= example
VERSION ?= 0.0.1
NS ?= $(SPECIALRESOURCE)
SR ?= $(SPECIALRESOURCE)

TMP := $(shell mktemp -d)

$(SPECIALRESOURCE):
	kubectl apply -f build/charts/$(REPO)/$(SR)-$(VERSION)/$(SR).yaml


chart: helm-repo-index
	@cp build/charts/$(REPO)/$(SR)-$(VERSION).tgz $(TMP)/.
	@helm repo index $(TMP) --url=cm://$(NS)/$(SR)-chart
	@kubectl create ns $(NS) --dry-run=client -o yaml | kubectl apply -f -
	@kubectl create cm $(SR)-chart --from-file=$(TMP)/index.yaml --from-file=$(TMP)/$(SR)-$(VERSION).tgz --dry-run=client -o yaml -n $(NS) | kubectl apply -f -



