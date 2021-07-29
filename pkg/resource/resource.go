package resource

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"github.com/openshift-psap/special-resource-operator/pkg/exit"
	"github.com/openshift-psap/special-resource-operator/pkg/filter"
	"github.com/openshift-psap/special-resource-operator/pkg/hash"
	"github.com/openshift-psap/special-resource-operator/pkg/poll"
	"github.com/openshift-psap/special-resource-operator/pkg/proxy"
	"helm.sh/helm/v3/pkg/kube"

	"github.com/openshift-psap/special-resource-operator/pkg/kernel"
	"github.com/openshift-psap/special-resource-operator/pkg/yamlutil"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/yaml"
)

var (
	log           logr.Logger
	HelmClient    kube.Interface
	RuntimeScheme *runtime.Scheme
	UpdateVendor  string
)

func init() {
	log = zap.New(zap.UseDevMode(true)).WithName(color.Print("resource", color.Blue))
	customCallback = make(resourceCallbacks)
}

func IsNamespaced(kind string) bool {
	if kind == "Namespace" ||
		kind == "ClusterRole" ||
		kind == "ClusterRoleBinding" ||
		kind == "SecurityContextConstraint" ||
		kind == "SpecialResource" {
		return false
	}
	return true
}

func IsNotUpdateable(kind string) bool {
	// ServiceAccounts cannot be updated, maybe delete and create?
	if kind == "ServiceAccount" || kind == "Pod" {
		return true
	}
	return false
}

// Some resources need an updated resourceversion, during updates
func NeedsResourceVersionUpdate(kind string) bool {
	if kind == "SecurityContextConstraints" ||
		kind == "Service" ||
		kind == "ServiceMonitor" ||
		kind == "Route" ||
		kind == "Build" ||
		kind == "BuildRun" ||
		kind == "BuildConfig" ||
		kind == "ImageStream" ||
		kind == "PrometheusRule" ||
		kind == "CSIDriver" ||
		kind == "Issuer" ||
		kind == "CustomResourceDefinition" ||
		kind == "Certificate" ||
		kind == "SpecialResource" ||
		kind == "OperatorGroup" ||
		kind == "CertManager" ||
		kind == "MutatingWebhookConfiguration" ||
		kind == "ValidatingWebhookConfiguration" ||
		kind == "Deployment" ||
		kind == "ImagePolicy" {
		return true
	}
	return false

}

func UpdateResourceVersion(req *unstructured.Unstructured, found *unstructured.Unstructured) error {

	kind := found.GetKind()

	if NeedsResourceVersionUpdate(kind) {
		version, fnd, err := unstructured.NestedString(found.Object, "metadata", "resourceVersion")
		exit.OnErrorOrNotFound(fnd, err)

		if err := unstructured.SetNestedField(req.Object, version, "metadata", "resourceVersion"); err != nil {
			return errors.Wrap(err, "Couldn't update ResourceVersion")
		}

	}
	if kind == "Service" {
		clusterIP, fnd, err := unstructured.NestedString(found.Object, "spec", "clusterIP")
		exit.OnErrorOrNotFound(fnd, err)

		if err := unstructured.SetNestedField(req.Object, clusterIP, "spec", "clusterIP"); err != nil {
			return errors.Wrap(err, "Couldn't update clusterIP")
		}
		return nil
	}
	return nil
}

func SetNodeSelectorTerms(obj *unstructured.Unstructured, terms map[string]string) error {

	if strings.Compare(obj.GetKind(), "DaemonSet") == 0 ||
		strings.Compare(obj.GetKind(), "Deployment") == 0 ||
		strings.Compare(obj.GetKind(), "Statefulset") == 0 {
		if err := nodeSelectorTerms(terms, obj, "spec", "template", "spec", "nodeSelector"); err != nil {
			return errors.Wrap(err, "Cannot setup "+obj.GetKind()+" nodeSelector")
		}
	}
	if strings.Compare(obj.GetKind(), "Pod") == 0 {
		if err := nodeSelectorTerms(terms, obj, "spec", "nodeSelector"); err != nil {
			return errors.Wrap(err, "Cannot setup Pod nodeSelector")
		}
	}
	if strings.Compare(obj.GetKind(), "BuildConfig") == 0 {
		if err := nodeSelectorTerms(terms, obj, "spec", "nodeSelector"); err != nil {
			return errors.Wrap(err, "Cannot setup BuildConfig nodeSelector")
		}
	}

	return nil
}

func nodeSelectorTerms(terms map[string]string, obj *unstructured.Unstructured, fields ...string) error {

	nodeSelector, found, err := unstructured.NestedMap(obj.Object, fields...)
	exit.OnError(err)

	if !found {
		nodeSelector = make(map[string]interface{})
	}

	for k, v := range terms {
		nodeSelector[k] = v
	}

	if err := unstructured.SetNestedMap(obj.Object, nodeSelector, fields...); err != nil {
		return errors.Wrap(err, "Cannot update nodeSelector for: "+obj.GetName())
	}

	return nil
}

func CreateFromYAML(yamlFile []byte,
	releaseInstalled bool,
	owner v1.Object,
	name string,
	namespace string,
	nodeSelector map[string]string,
	kernelFullVersion string,
	operatingSystemMajorMinor string) error {

	scanner := yamlutil.NewYAMLScanner(yamlFile)

	for scanner.Scan() {

		yamlSpec := scanner.Bytes()

		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{},
		}

		jsonSpec, err := yaml.YAMLToJSON(yamlSpec)
		if err != nil {
			return errors.Wrap(err, "Could not convert yaml file to json"+string(yamlSpec))
		}

		err = obj.UnmarshalJSON(jsonSpec)
		exit.OnError(errors.Wrap(err, "Cannot unmarshall json spec, check your manifest: "+string(jsonSpec)))

		//  Do not override the namespace if alreayd set
		if IsNamespaced(obj.GetKind()) && obj.GetNamespace() == "" {
			log.Info("Namespace empty settting", "namespace", namespace)
			obj.SetNamespace(namespace)
		}

		// We used this for predicate filtering, we're watching a lot of
		// API Objects we want to ignore all objects that do not have this
		// label.
		filter.SetLabel(obj)

		// kernel affinity related attributes only set if there is an
		// annotation specialresource.openshift.io/kernel-affine: true
		if kernel.IsObjectAffine(obj) {
			err := kernel.SetAffineAttributes(obj, kernelFullVersion,
				operatingSystemMajorMinor)
			exit.OnError(errors.Wrap(err, "Cannot set kernel affine attributes"))
		}

		// Add nodeSelector terms for the specialresource
		// we do not want to spread HW enablement stacks on all nodes
		err = SetNodeSelectorTerms(obj, nodeSelector)
		exit.OnError(errors.Wrap(err, "setting NodeSelectorTerms failed"))

		// We are only building a driver-container if we cannot pull the image
		// We are asuming that vendors provide pre compiled DriverContainers
		// If err == nil, build a new container, if err != nil skip it
		if err := rebuildDriverContainer(obj); err != nil {
			log.Info("Skipping building driver-container", "Name", obj.GetName())
			return nil
		}

		// Callbacks before CRUD will update the manifests
		if err := BeforeCRUD(obj, owner); err != nil {
			return errors.Wrap(err, "Before CRUD hooks failed")
		}
		// Create Update Delete Patch resources
		err = CRUD(obj, releaseInstalled, owner, name, namespace)
		// The mutating webhook needs a couple of secs to be ready
		// sleep for 5 secs and requeue
		if err != nil && strings.Contains(err.Error(), "failed calling webhook") {
			return errors.Wrap(err, "Webhook not ready, requeue")
		}
		exit.OnError(errors.Wrapf(err, "CRUD exited non-zero on Object: %+v", obj))

		// Callbacks after CRUD will wait for ressource and check status
		if err := AfterCRUD(obj, namespace); err != nil {
			return errors.Wrap(err, "After CRUD hooks failed")
		}

	}

	if err := scanner.Err(); err != nil {
		return errors.Wrap(err, "Failed to scan manifest")
	}
	return nil
}

func IsOneTimer(obj *unstructured.Unstructured) bool {

	// We are not recreating Pods that have restartPolicy: Never
	if obj.GetKind() == "Pod" {
		restartPolicy, found, err := unstructured.NestedString(obj.Object, "spec", "restartPolicy")
		exit.OnErrorOrNotFound(found, err)
		if restartPolicy == "Never" {
			return true
		}
	}
	return false
}

// CRUD Create Update Delete Resource
func CRUD(obj *unstructured.Unstructured, releaseInstalled bool, owner v1.Object, name string, namespace string) error {

	var logg logr.Logger
	if IsNamespaced(obj.GetKind()) {
		logg = log.WithValues("Kind", obj.GetKind()+": "+obj.GetNamespace()+"/"+obj.GetName())
	} else {
		logg = log.WithValues("Kind", obj.GetKind()+": "+obj.GetName())
	}

	// SpecialResource is the parent, all other objects are childs and need a reference
	// but only set the ownerreference if created by SRO do not set ownerreference per default
	if obj.GetKind() != "SpecialResource" && obj.GetKind() != "Namespace" {
		err := controllerutil.SetControllerReference(owner, obj, RuntimeScheme)
		exit.OnError(err)

		SetMetaData(obj, name, namespace)
	}

	found := obj.DeepCopy()

	key := types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}

	err := clients.Interface.Get(context.TODO(), key, found)

	if apierrors.IsNotFound(err) {
		// We are not recreating all objects if a release is already installed
		if releaseInstalled && IsOneTimer(obj) {
			logg.Info("Skipping creation")
			return nil
		}

		logg.Info("Not found, creating")

		logg.Info("Release", "Installed", releaseInstalled)
		logg.Info("Is", "OneTimer", IsOneTimer(obj))

		hash.Annotate(obj)

		// If we create the resource set the owner reference
		err := controllerutil.SetControllerReference(owner, obj, RuntimeScheme)
		exit.OnError(err)

		SetMetaData(obj, name, namespace)

		if err := clients.Interface.Create(context.TODO(), obj); err != nil {
			if apierrors.IsForbidden(err) {
				return errors.Wrap(err, "API error is forbidden")
			}
			return errors.Wrap(err, "Unknown error")
		}

		return nil
	}

	if apierrors.IsForbidden(err) {
		return errors.Wrap(err, "Forbidden check Role, ClusterRole and Bindings for operator")
	}

	if err != nil {
		return errors.Wrap(err, "Unexpected error")
	}

	// Not updating Pod because we can only update image and some other
	// specific minor fields.
	if IsNotUpdateable(obj.GetKind()) {
		logg.Info("Not Updateable", "Resource", obj.GetKind())
		return nil
	}

	if hash.AnnotationEqual(found, obj) {
		logg.Info("Found, not updating, hash the same: " + found.GetKind() + "/" + found.GetName())
		return nil
	}

	logg.Info("Found, updating")
	required := obj.DeepCopy()

	hash.Annotate(required)

	// required.ResourceVersion = found.ResourceVersion this is only needed
	// before we update a resource, we do not care when creating, hence
	// !leave this here!
	if err := UpdateResourceVersion(required, found); err != nil {
		return errors.Wrap(err, "Couldn't Update ResourceVersion")
	}

	if err := clients.Interface.Update(context.TODO(), required); err != nil {
		return errors.Wrap(err, "Couldn't Update Resource")
	}

	return nil
}

func rebuildDriverContainer(obj *unstructured.Unstructured) error {

	logger := log.WithValues("Kind", obj.GetKind(), "Namespace", obj.GetNamespace(), "Name", obj.GetName())
	// BuildConfig are currently not triggered by an update need to delete first
	if obj.GetKind() == "BuildConfig" {
		annotations := obj.GetAnnotations()
		if vendor, ok := annotations["specialresource.openshift.io/driver-container-vendor"]; ok {
			logger.Info("driver-container-vendor", "vendor", vendor)
			if vendor == UpdateVendor {
				logger.Info("vendor == updateVendor", "vendor", vendor, "updateVendor", UpdateVendor)
				return nil
			}
			logger.Info("vendor != updateVendor", "vendor", vendor, "updateVendor", UpdateVendor)
			return errors.New("vendor != updateVendor")
		}
		logger.Info("No annotation driver-container-vendor found, not skipping")
		return nil
	}

	return nil
}

func SetMetaData(obj *unstructured.Unstructured, nm string, ns string) {

	annotations := obj.GetAnnotations()

	if annotations == nil {
		annotations = make(map[string]string)
	}

	annotations["meta.helm.sh/release-name"] = nm
	annotations["meta.helm.sh/release-namespace"] = ns

	obj.SetAnnotations(annotations)

	labels := obj.GetLabels()

	if labels == nil {
		labels = make(map[string]string)
	}

	labels["app.kubernetes.io/managed-by"] = "Helm"

	obj.SetLabels(labels)

}

type resourceCallbacks map[string]func(obj *unstructured.Unstructured, sr interface{}) error

var customCallback resourceCallbacks

func BeforeCRUD(obj *unstructured.Unstructured, sr interface{}) error {

	var found bool
	todo := ""
	annotations := obj.GetAnnotations()

	if valid, found := annotations["specialresource.openshift.io/proxy"]; found && valid == "true" {
		if err := proxy.Setup(obj); err != nil {
			return errors.Wrap(err, "Could not setup Proxy")
		}
	}

	if todo, found = annotations["specialresource.openshift.io/callback"]; !found {
		return nil
	}

	if prefix, ok := customCallback[todo]; ok {
		if err := prefix(obj, sr); err != nil {
			return errors.Wrap(err, "Could not run prefix callback")
		}
	}
	return nil
}

func AfterCRUD(obj *unstructured.Unstructured, namespace string) error {

	annotations := obj.GetAnnotations()
	clients.Namespace = namespace

	if state, found := annotations["specialresource.openshift.io/state"]; found && state == "driver-container" {
		log.Info("specialresource.openshift.io/state")
		if err := checkForImagePullBackOff(obj, namespace); err != nil {
			return errors.Wrap(err, "Cannot check for ImagePullBackOff")
		}
	}

	if wait, found := annotations["specialresource.openshift.io/wait"]; found && wait == "true" {
		log.Info("specialresource.openshift.io/wait")
		if err := poll.ForResource(obj); err != nil {
			return errors.Wrap(err, "Could not wait for resource")
		}
	}

	if pattern, found := annotations["specialresource.openshift.io/wait-for-logs"]; found && len(pattern) > 0 {
		log.Info("specialresource.openshift.io/wait-for-logs")
		if err := poll.ForDaemonSetLogs(obj, pattern); err != nil {
			return errors.Wrap(err, "Could not wait for DaemonSet logs")
		}
	}

	if _, found := annotations["helm.sh/hook"]; found {
		// In the case of hooks we're always waiting for all ressources
		if err := poll.ForResource(obj); err != nil {
			return errors.Wrap(err, "Could not wait for resource")
		}
	}

	// Always wait for CRDs to be present
	if obj.GetKind() == "CustomResourceDefinition" {
		if err := poll.ForResource(obj); err != nil {
			return errors.Wrap(err, "Could not wait for CRD")
		}
	}

	return nil
}

func checkForImagePullBackOff(obj *unstructured.Unstructured, namespace string) error {

	if err := poll.ForDaemonSet(obj); err == nil {
		return nil
	}

	labels := obj.GetLabels()
	value := labels["app"]

	find := make(map[string]string)
	find["app"] = value

	// DaemonSet is not coming up, lets check if we have to rebuild
	pods := &unstructured.UnstructuredList{}
	pods.SetAPIVersion("v1")
	pods.SetKind("PodList")

	log.Info("checkForImagePullBackOff get PodList from: " + namespace)

	opts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels(find),
	}

	err := clients.Interface.List(context.TODO(), pods, opts...)
	if err != nil {
		log.Error(err, "Could not get PodList")
		return err
	}

	if len(pods.Items) == 0 {
		return fmt.Errorf("no Pods found, reconciling")
	}

	var reason string

	for _, pod := range pods.Items {
		log.Info("checkForImagePullBackOff", "PodName", pod.GetName())

		var err error
		var found bool
		var containerStatuses []interface{}

		if containerStatuses, found, err = unstructured.NestedSlice(pod.Object, "status", "containerStatuses"); !found || err != nil {
			phase, found, err := unstructured.NestedString(pod.Object, "status", "phase")
			exit.OnErrorOrNotFound(found, err)
			log.Info("Pod is in phase: " + phase)
			continue
		}

		for _, containerStatus := range containerStatuses {
			switch containerStatus := containerStatus.(type) {
			case map[string]interface{}:
				reason, _, _ = unstructured.NestedString(containerStatus, "state", "waiting", "reason")
				log.Info("Reason", "reason", reason)
			default:
				log.Info("checkForImagePullBackOff", "DEFAULT NOT THE CORRECT TYPE", containerStatus)
			}
			break
		}

		if reason == "ImagePullBackOff" || reason == "ErrImagePull" {
			annotations := obj.GetAnnotations()
			if vendor, ok := annotations["specialresource.openshift.io/driver-container-vendor"]; ok {
				UpdateVendor = vendor
				return errors.New("ImagePullBackOff need to rebuild " + UpdateVendor + " driver-container")
			}
		}

		log.Info("Unsetting updateVendor, Pods not in ImagePullBackOff or ErrImagePull")
		UpdateVendor = ""
		return nil
	}

	return errors.New("Unexpected Phase of Pods in DameonSet: " + obj.GetName())
}
