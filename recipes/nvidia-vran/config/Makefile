nvidia-vran: 
	-kubectl delete configmap -n $(NAMESPACE) 0000-$@-mofed
	kubectl create configmap 0000-$@-mofed -n $(NAMESPACE) --from-file=recipes/$@/manifests/mofed
	kubectl patch configmap 0000-$@-mofed -n $(NAMESPACE)  --patch '$(shell cat recipes/$@/config/mofed-patch.json)'

	-kubectl delete configmap -n $(NAMESPACE) 1000-$@-gpudirect
	kubectl create configmap 1000-$@-gpudirect -n $(NAMESPACE) --from-file=recipes/$@/manifests/gpudirect
	kubectl patch configmap 1000-$@-gpudirect -n $(NAMESPACE)  --patch '$(shell cat recipes/$@/config/gpudirect-patch.json)'

	-kubectl delete configmap -n $(NAMESPACE) 2000-$@-gdrdrv
	kubectl create configmap 2000-$@-gdrdrv -n $(NAMESPACE) --from-file=recipes/$@/manifests/gdrdrv
	kubectl patch configmap 2000-$@-gdrdrv -n $(NAMESPACE)  --patch '$(shell cat recipes/$@/config/gdrdrv-patch.json)'
