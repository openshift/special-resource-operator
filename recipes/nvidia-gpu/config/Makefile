
nvidia-gpu:  
	-kubectl delete configmap -n $(NAMESPACE) 0000-$@
	kubectl create configmap 0000-$@ -n $(NAMESPACE) --from-file=recipes/$@/manifests
	kubectl patch configmap 0000-$@ -n $(NAMESPACE)  --patch '$(shell cat recipes/$@/config/patch.json)'

