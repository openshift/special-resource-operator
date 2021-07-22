REPO ?= example
VERSION ?= 0.0.1

$(SPECIALRESOURCE):
	kubectl apply -f charts/$(REPO)/$(SPECIALRESOURCE)-$(VERSION)/$(SPECIALRESOURCE).yaml

assets:
	cd charts/$(REPO)/$(SPECIALRESOURCE)-$(VERSION) && $(KUSTOMIZE) edit set namespace $(SPECIALRESOURCE)
# kubectl create ns $(SPECIALRESOURCE) --dry-run=client -o yaml | kubectl apply -f -
	kubectl apply -k charts/$(REPO)/$(SPECIALRESOURCE)-$(VERSION)
