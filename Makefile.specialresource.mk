$(SPECIALRESOURCE):
	kubectl apply -f charts/$(SPECIALRESOURCE)/$(SPECIALRESOURCE).yaml

assets:
	cd charts/$(SPECIALRESOURCE)/templates && $(KUSTOMIZE) edit set namespace $(SPECIALRESOURCE)
	kubectl create ns $(SPECIALRESOURCE) --dry-run=client -o yaml | kubectl apply -f -
	kubectl apply -k charts/$(SPECIALRESOURCE)/templates
