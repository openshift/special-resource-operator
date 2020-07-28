nvidia-vran: 
	-kubectl delete configmap -n $(NAMESPACE) $@-mofed  2>/dev/null | true
	kubectl create configmap $@-mofed -n $(NAMESPACE) --from-file=recipes/$@/manifests/mofed
	@$(TEMPLATE_CMD) recipes/$@/config/$@-mofed_specialresource_cr.yaml | kubectl apply -f - 

	-kubectl delete configmap -n $(NAMESPACE) $@-gpudirect  2>/dev/null | true
	kubectl create configmap $@-gpudirect -n $(NAMESPACE) --from-file=recipes/$@/manifests/gpudirect
	@$(TEMPLATE_CMD) recipes/$@/config/$@-gpudirect_specialresource_cr.yaml | kubectl apply -f - 

	-kubectl delete configmap -n $(NAMESPACE) $@-gdrdrv  2>/dev/null | true
	kubectl create configmap $@-gdrdrv -n $(NAMESPACE) --from-file=recipes/$@/manifests/gdrdrv
	@$(TEMPLATE_CMD) recipes/$@/config/$@-gdrdrv_specialresource_cr.yaml | kubectl apply -f - 