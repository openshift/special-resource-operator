package resource

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"

	"github.com/openshift/special-resource-operator/internal/resourcehelper"
	"github.com/openshift/special-resource-operator/pkg/clients"
	"github.com/openshift/special-resource-operator/pkg/kernel"
	"github.com/openshift/special-resource-operator/pkg/lifecycle"
	"github.com/openshift/special-resource-operator/pkg/metrics"
	"github.com/openshift/special-resource-operator/pkg/poll"
	"github.com/openshift/special-resource-operator/pkg/proxy"
	"github.com/openshift/special-resource-operator/pkg/utils"
	"github.com/openshift/special-resource-operator/pkg/yamlutil"
)

var (
	UpdateVendor string
)

//go:generate mockgen -source=resource.go -package=resource -destination=mock_resource_api.go

type ResourceAPI interface {
	CreateFromYAML(context.Context, []byte, bool, v1.Object, string, string, map[string]string, string, string, string) error
	GetObjectsFromYAML([]byte) (*unstructured.UnstructuredList, error)
}

type resource struct {
	kubeClient    clients.ClientsInterface
	lc            lifecycle.Lifecycle
	metricsClient metrics.Metrics
	pollActions   poll.PollActions
	kernelAPI     kernel.KernelData
	proxyAPI      proxy.ProxyAPI
	scheme        *runtime.Scheme
	helper        resourcehelper.Helper
}

func NewResourceAPI(
	kubeClient clients.ClientsInterface,
	metricsClient metrics.Metrics,
	pollActions poll.PollActions,
	kernelAPI kernel.KernelData,
	scheme *runtime.Scheme,
	lc lifecycle.Lifecycle,
	proxyAPI proxy.ProxyAPI,
	resHelper resourcehelper.Helper,
) ResourceAPI {
	return &resource{
		kubeClient:    kubeClient,
		lc:            lc,
		metricsClient: metricsClient,
		pollActions:   pollActions,
		kernelAPI:     kernelAPI,
		scheme:        scheme,
		proxyAPI:      proxyAPI,
		helper:        resHelper,
	}
}

func (r *resource) afterCRUD(ctx context.Context, obj *unstructured.Unstructured, namespace string) error {

	logger := ctrl.LoggerFrom(ctx, "objKind", obj.GetObjectKind().GroupVersionKind().Kind, "objNamespace", obj.GetNamespace(), "objName", obj.GetName())

	annotations := obj.GetAnnotations()
	clients.Namespace = namespace

	if state, found := annotations["specialresource.openshift.io/state"]; found && state == "driver-container" {
		logger.Info("Handling annotation specialresource.openshift.io/state")
		if err := r.checkForImagePullBackOff(ctx, obj, namespace); err != nil {
			return fmt.Errorf("failed to check for ImagePullBackOff of object %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
		}
	}

	if wait, found := annotations["specialresource.openshift.io/wait"]; found && wait == "true" {
		logger.Info("Handling annotation specialresource.openshift.io/wait")
		if err := r.pollActions.ForResource(ctx, obj); err != nil {
			return fmt.Errorf("failed to wait for resource, object %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
		}
	}

	if pattern, found := annotations["specialresource.openshift.io/wait-for-logs"]; found && len(pattern) > 0 {
		logger.Info("Handling annotation specialresource.openshift.io/wait-for-logs")
		if err := r.pollActions.ForDaemonSetLogs(ctx, obj, pattern); err != nil {
			return fmt.Errorf("failed to wait for DaemonSet %s/%s logs: %w", obj.GetNamespace(), obj.GetName(), err)
		}
	}

	if _, found := annotations["helm.sh/hook"]; found {
		logger.Info("Handling annotation helm.sh/hook")
		// In the case of hooks we're always waiting for all ressources
		if err := r.pollActions.ForResource(ctx, obj); err != nil {
			return fmt.Errorf("failed to wait for resource object %s/%s after helm hook: %w", obj.GetNamespace(), obj.GetName(), err)
		}
	}

	// Always wait for CRDs to be present
	if obj.GetKind() == "CustomResourceDefinition" {
		if err := r.pollActions.ForResource(ctx, obj); err != nil {
			return fmt.Errorf("failed to wait for CRD object %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
		}
	}

	logger.Info("Object is fully created, all wait conditions have been fulfilled")

	return nil
}

func (r *resource) CreateFromYAML(
	ctx context.Context,
	yamlFile []byte,
	releaseInstalled bool,
	owner v1.Object,
	name string,
	namespace string,
	nodeSelector map[string]string,
	kernelFullVersion string,
	operatingSystemMajorMinor string,
	ownerLabel string) error {

	scanner := yamlutil.NewYAMLScanner(yamlFile)

	for scanner.Scan() {

		yamlSpec := scanner.Bytes()

		err := r.createObjFromYAML(
			ctx,
			yamlSpec,
			releaseInstalled,
			owner,
			name,
			namespace,
			nodeSelector,
			kernelFullVersion,
			operatingSystemMajorMinor,
			ownerLabel)
		if err != nil {
			return fmt.Errorf("failed to create object from YAML: %w", err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan manifest: %w", err)
	}

	return nil
}

func (r *resource) GetObjectsFromYAML(yamlFile []byte) (*unstructured.UnstructuredList, error) {
	scanner := yamlutil.NewYAMLScanner(yamlFile)
	objList := &unstructured.UnstructuredList{
		Items: []unstructured.Unstructured{},
	}
	for scanner.Scan() {
		yamlSpec := scanner.Bytes()
		obj := unstructured.Unstructured{
			Object: map[string]interface{}{},
		}

		jsonSpec, err := yaml.YAMLToJSON(yamlSpec)
		if err != nil {
			return nil, fmt.Errorf("failed to convert YAML to json: %w", err)
		}

		if err = obj.UnmarshalJSON(jsonSpec); err != nil {
			return nil, fmt.Errorf("failed to unmarshal JSON spec, check your manifest: %w", err)
		}
		objList.Items = append(objList.Items, obj)
	}
	return objList, nil
}

// CRUD Create Update Delete Resource
func (r *resource) crud(ctx context.Context, obj *unstructured.Unstructured, releaseInstalled bool, owner v1.Object, name string, namespace string) error {
	log := ctrl.LoggerFrom(ctx, "objKind", obj.GetKind(), "objName", obj.GetName())

	if r.helper.IsNamespaced(obj.GetKind()) {
		log = log.WithValues("namespace", obj.GetNamespace())
	}

	// SpecialResource is the parent, all other objects are childs and need a reference
	// but only set the ownerreference if created by SRO do not set ownerreference per default
	if obj.GetKind() != "SpecialResource" && obj.GetKind() != "Namespace" {
		if err := controllerutil.SetControllerReference(owner, obj, r.scheme); err != nil {
			return fmt.Errorf("failed to set owner %s/%s to object %s/%s: %w", owner.GetNamespace(), owner.GetName(), obj.GetNamespace(), obj.GetName(), err)
		}

		r.helper.SetMetaData(obj, name, namespace)
	}

	found := obj.DeepCopy()

	key := types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}

	err := r.kubeClient.Get(ctx, key, found)

	if apierrors.IsNotFound(err) {
		oneTimer, err := r.helper.IsOneTimer(obj)
		if err != nil {
			return fmt.Errorf("failed to determine if the object %s/%s is a one-timer: %w", obj.GetNamespace(), obj.GetName(), err)
		}

		// We are not recreating all objects if a release is already installed
		if releaseInstalled && oneTimer {
			return nil
		}

		log.Info("Creating object", "releaseInstalled", releaseInstalled, "oneTimer", oneTimer)

		if err = utils.Annotate(obj); err != nil {
			return fmt.Errorf("failed to annotate object %s/%s with hash: %w", obj.GetNamespace(), obj.GetName(), err)
		}

		if err = r.kubeClient.Create(ctx, obj); err != nil {
			if apierrors.IsForbidden(err) {
				return fmt.Errorf("failed to create object %s/%s, forbidden: %w", obj.GetNamespace(), obj.GetName(), err)
			}

			return fmt.Errorf("failed to create object %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
		}

		log.Info("Object created")

		return nil
	}

	if apierrors.IsForbidden(err) {
		return fmt.Errorf("failed to get object %s/%s, forbidden, check Role, ClusterRole and Bindings for operator: %w", obj.GetNamespace(), obj.GetName(), err)
	}

	if err != nil {
		return fmt.Errorf("failed to get object %s/%s, unexpected error: %w", obj.GetNamespace(), obj.GetName(), err)
	}

	// Not updating Pod because we can only update image and some other
	// specific minor fields.
	if r.helper.IsNotUpdateable(obj.GetKind()) {
		return nil
	}

	equal, err := utils.AnnotationEqual(found, obj)
	if err != nil {
		return fmt.Errorf("failed to check annotation equality for object %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	if equal {
		return nil
	}

	log.Info("Updating object")
	required := obj.DeepCopy()

	if err = utils.Annotate(required); err != nil {
		return fmt.Errorf("failed to annotate object with hash: %w", err)
	}

	// required.ResourceVersion = found.ResourceVersion this is only needed
	// before we update a resource, we do not care when creating, hence
	// !leave this here!
	if err = r.helper.UpdateResourceVersion(required, found); err != nil {
		return fmt.Errorf("failed to update ResourceVersion for object %s/%s: %w", found.GetNamespace(), found.GetName(), err)
	}

	if err = r.kubeClient.Update(ctx, required); err != nil {
		return fmt.Errorf("failed to update objec %s/%s: %w", required.GetNamespace(), required.GetName(), err)
	}

	log.Info("Object has been updated")

	return nil
}

func (r *resource) checkForImagePullBackOff(ctx context.Context, obj *unstructured.Unstructured, namespace string) error {

	if err := r.pollActions.ForDaemonSet(ctx, obj); err == nil {
		return nil
	}

	labels := obj.GetLabels()
	value := labels["app"]

	// DaemonSet is not coming up, lets check if we have to rebuild
	pods := &corev1.PodList{}

	opts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels(map[string]string{"app": value}),
	}

	err := r.kubeClient.List(ctx, pods, opts...)
	if err != nil {
		return fmt.Errorf("failed to get PodList for namespace %s: %w", namespace, err)
	}

	if len(pods.Items) == 0 {
		return fmt.Errorf("no Pods were found in namespace %s", namespace)
	}

	var reason string

	for _, pod := range pods.Items {

		containerStatuses := pod.Status.ContainerStatuses

		if containerStatuses == nil {
			phase := pod.Status.Phase
			if phase == "" {
				return fmt.Errorf("pod %s/%s has an empty phase", pod.GetNamespace(), pod.GetName())
			}
			continue
		}

		for _, containerStatus := range containerStatuses {
			reason = ""

			if containerStatus.State.Waiting != nil {
				reason = containerStatus.State.Waiting.Reason
			}
		}

		if reason == "ImagePullBackOff" || reason == "ErrImagePull" {
			annotations := obj.GetAnnotations()
			if vendor, ok := annotations["specialresource.openshift.io/driver-container-vendor"]; ok {
				UpdateVendor = vendor
				return fmt.Errorf("pod %s/%s is in ImagePullBackOff, need to rebuild %s driver-container", pod.GetNamespace(), pod.GetName(), UpdateVendor)
			}
		}
		UpdateVendor = ""
		return nil
	}

	return fmt.Errorf("unexpected Phase of Pods in DaemonSet %s/%s", obj.GetNamespace(), obj.GetName())
}

func (r *resource) createObjFromYAML(
	ctx context.Context,
	yamlSpec []byte,
	releaseInstalled bool,
	owner v1.Object,
	name string,
	namespace string,
	nodeSelector map[string]string,
	kernelFullVersion string,
	operatingSystemMajorMinor string,
	ownerLabel string) error {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{},
	}

	jsonSpec, err := yaml.YAMLToJSON(yamlSpec)
	if err != nil {
		return fmt.Errorf("could not convert yaml file to json: %w", err)
	}

	if err = obj.UnmarshalJSON(jsonSpec); err != nil {
		return fmt.Errorf("cannot unmarshall json spec, check your manifest: %w", err)
	}

	//  Do not override the namespace if already set
	if r.helper.IsNamespaced(obj.GetKind()) && obj.GetNamespace() == "" {
		obj.SetNamespace(namespace)
	}

	yamlKind := obj.GetKind()
	yamlName := obj.GetName()
	yamlNamespace := obj.GetNamespace()
	metricValue := 0
	defer func() {
		r.metricsClient.SetCompletedKind(name, yamlKind, yamlName, yamlNamespace, metricValue)
	}()

	// We used this for predicate filtering, we're watching a lot of
	// API Objects we want to ignore all objects that do not have this
	// label.
	if err = r.helper.SetLabel(obj, ownerLabel); err != nil {
		return fmt.Errorf("could not set label: %w", err)
	}
	// kernel affinity related attributes only set if there is an
	// annotation specialresource.openshift.io/kernel-affine: true
	if r.kernelAPI.IsObjectAffine(obj) {
		ctrl.LoggerFrom(ctx).Info("Object is kernel affine", "objectName", obj.GetName())
		if err = r.kernelAPI.SetAffineAttributes(obj, kernelFullVersion, operatingSystemMajorMinor); err != nil {
			return fmt.Errorf("failed to set kernel affine attributes for object %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
		}
	}

	// Add nodeSelector terms defined for the specialresource CR to the object
	// we do not want to spread HW enablement stacks on all nodes
	if err = r.helper.SetNodeSelectorTerms(obj, nodeSelector); err != nil {
		return fmt.Errorf("failed to set NodeSelectorTerms for object %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}

	// We are only building a driver-container if we cannot pull the image
	// We are asuming that vendors provide pre compiled DriverContainers
	// If err == nil, build a new container, if err != nil skip it
	if r.skipRebuildDriverContainer(obj) {
		ctrl.LoggerFrom(ctx).Info("Skipping building driver-container", "buildConfigName", obj.GetName())
		return nil
	}

	// Callbacks before crud will update the manifests
	if err = r.beforeCRUD(ctx, obj, owner); err != nil {
		return fmt.Errorf("failed to execute before crud for object %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	// Create Update Delete Patch resources
	err = r.crud(ctx, obj, releaseInstalled, owner, name, namespace)
	if err != nil {
		if strings.Contains(err.Error(), "failed calling webhook") {
			return fmt.Errorf("failed to execute crude for object %s/%s, webhook not ready: %w", obj.GetNamespace(), obj.GetName(), err)
		}

		return fmt.Errorf("failed to execute crud for object %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}

	// Callbacks after CRUD will wait for ressource and check status
	if err = r.afterCRUD(ctx, obj, namespace); err != nil {
		return fmt.Errorf("failed to execute after crud for object %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}

	r.sendNodesMetrics(ctx, obj, name)

	metricValue = 1
	return nil
}

func (r *resource) skipRebuildDriverContainer(obj *unstructured.Unstructured) bool {
	if obj.GetKind() == "BuildConfig" {
		annotations := obj.GetAnnotations()
		if vendor, ok := annotations["specialresource.openshift.io/driver-container-vendor"]; ok {
			return vendor != UpdateVendor
		}
	}
	return false
}

func (r *resource) sendNodesMetrics(ctx context.Context, obj *unstructured.Unstructured, crName string) {
	kind := obj.GetKind()
	if kind != "DaemonSet" && kind != "Deployment" {
		return
	}

	objKey := types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
	getPodsFunc := r.lc.GetPodFromDaemonSet
	if kind == "Deployment" {
		getPodsFunc = r.lc.GetPodFromDeployment
	}

	pl := getPodsFunc(ctx, objKey)
	nodesNames := []string{}
	for _, pod := range pl.Items {
		nodesNames = append(nodesNames, pod.Spec.NodeName)
	}

	if len(nodesNames) != 0 {
		nodesStr := strings.Join(nodesNames, ",")
		r.metricsClient.SetUsedNodes(crName, obj.GetName(), obj.GetNamespace(), kind, nodesStr)
	} else {
		ctrl.LoggerFrom(ctx).Info("No assigned nodes found for UsedNodes metric", "kind", kind, "object", objKey)
	}
}

func (r *resource) beforeCRUD(ctx context.Context, obj *unstructured.Unstructured, sr interface{}) error {
	annotations := obj.GetAnnotations()
	if valid, found := annotations["specialresource.openshift.io/proxy"]; found && valid == "true" {
		if err := r.proxyAPI.Setup(ctx, obj); err != nil {
			return fmt.Errorf("failed to setup Proxy in before crud for object %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
		}
	}
	return nil
}
