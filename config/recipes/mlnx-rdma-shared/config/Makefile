mlnx-rdma-shared: 
	-kubectl delete configmap -n $(NAMESPACE) $@-mofed  2>/dev/null | true
	kubectl create configmap $@-mofed -n $(NAMESPACE) --from-file=recipes/$@/manifests/mofed
	@$(TEMPLATE_CMD) recipes/$@/config/$@-mofed-cr.yaml | kubectl apply -f - 

	-kubectl delete configmap -n $(NAMESPACE) $@-gpudirect  2>/dev/null | true
	kubectl create configmap $@-gpudirect -n $(NAMESPACE) --from-file=recipes/$@/manifests/gpudirect
	@$(TEMPLATE_CMD) recipes/$@/config/$@-gpudirect-cr.yaml | kubectl apply -f - 

