driver-container-base: 
	-kubectl create ns $@ 2>/dev/null | true
	-kubectl delete configmap $@ -n $@  2>/dev/null | true
	kubectl create configmap $@ -n $@ --from-file=recipes/$@/manifests
	@$(TEMPLATE_CMD) recipes/$@/config/$@-cr.yaml | kubectl apply -f - 