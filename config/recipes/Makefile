$(SPECIALRESOURCE): 
	kubectl apply -f config/recipes/$(SPECIALRESOURCE)/0000-$(SPECIALRESOURCE)-cr.yaml

assets:
	cd config/recipes/$(SPECIALRESOURCE)/manifests && $(KUSTOMIZE) edit set namespace $(SPECIALRESOURCE)
	kubectl create ns $(SPECIALRESOURCE) --dry-run=client -o yaml | kubectl apply -f -
	kubectl apply -k config/recipes/$(SPECIALRESOURCE)/manifests
