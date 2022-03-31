package resource

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/yaml"

	"github.com/openshift/special-resource-operator/internal/resourcehelper"
	"github.com/openshift/special-resource-operator/pkg/clients"
	"github.com/openshift/special-resource-operator/pkg/filter"
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
	CreateFromYAML(context.Context, []byte, bool, v1.Object, string, string, map[string]string, string, string) error
	GetObjectsFromYAML([]byte) (*unstructured.UnstructuredList, error)
}

type resource struct {
	kubeClient    clients.ClientsInterface
	lc            lifecycle.Lifecycle
	log           logr.Logger
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
		log:           zap.New(zap.UseDevMode(true)).WithName(utils.Print("resource", utils.Blue)),
		metricsClient: metricsClient,
		pollActions:   pollActions,
		kernelAPI:     kernelAPI,
		scheme:        scheme,
		proxyAPI:      proxyAPI,
		helper:        resHelper,
	}
}

func (r *resource) AfterCRUD(ctx context.Context, obj *unstructured.Unstructured, namespace string) error {

	logger := r.log.WithValues("Kind", obj.GetObjectKind(), "namespace", obj.GetNamespace(), "name", obj.GetName())

	annotations := obj.GetAnnotations()
	clients.Namespace = namespace

	if state, found := annotations["specialresource.openshift.io/state"]; found && state == "driver-container" {
		logger.Info("specialresource.openshift.io/state")
		if err := r.checkForImagePullBackOff(ctx, obj, namespace); err != nil {
			return fmt.Errorf("cannot check for ImagePullBackOff: %w", err)
		}
	}

	if wait, found := annotations["specialresource.openshift.io/wait"]; found && wait == "true" {
		logger.Info("specialresource.openshift.io/wait")
		if err := r.pollActions.ForResource(ctx, obj); err != nil {
			return fmt.Errorf("could not wait for resource: %w", err)
		}
	}

	if pattern, found := annotations["specialresource.openshift.io/wait-for-logs"]; found && len(pattern) > 0 {
		logger.Info("specialresource.openshift.io/wait-for-logs")
		if err := r.pollActions.ForDaemonSetLogs(ctx, obj, pattern); err != nil {
			return fmt.Errorf("could not wait for DaemonSet logs: %w", err)
		}
	}

	if _, found := annotations["helm.sh/hook"]; found {
		// In the case of hooks we're always waiting for all ressources
		if err := r.pollActions.ForResource(ctx, obj); err != nil {
			return fmt.Errorf("could not wait for resource: %w", err)
		}
	}

	// Always wait for CRDs to be present
	if obj.GetKind() == "CustomResourceDefinition" {
		if err := r.pollActions.ForResource(ctx, obj); err != nil {
			return fmt.Errorf("could not wait for CRD: %w", err)
		}
	}

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
	operatingSystemMajorMinor string) error {

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
			operatingSystemMajorMinor)
		if err != nil {
			return err
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
			return nil, fmt.Errorf("could not convert YAML file to json: %s: error %w", string(yamlSpec), err)
		}

		if err = obj.UnmarshalJSON(jsonSpec); err != nil {
			return nil, fmt.Errorf("cannot unmarshall JSON spec, check your manifest: %s: %w", jsonSpec, err)
		}
		objList.Items = append(objList.Items, obj)
	}
	return objList, nil
}

// CRUD Create Update Delete Resource
func (r *resource) CRUD(ctx context.Context, obj *unstructured.Unstructured, releaseInstalled bool, owner v1.Object, name string, namespace string) error {

	var logg logr.Logger
	if r.helper.IsNamespaced(obj.GetKind()) {
		logg = r.log.WithValues("Kind", obj.GetKind()+": "+obj.GetNamespace()+"/"+obj.GetName())
	} else {
		logg = r.log.WithValues("Kind", obj.GetKind()+": "+obj.GetName())
	}

	// SpecialResource is the parent, all other objects are childs and need a reference
	// but only set the ownerreference if created by SRO do not set ownerreference per default
	if obj.GetKind() != "SpecialResource" && obj.GetKind() != "Namespace" {
		if err := controllerutil.SetControllerReference(owner, obj, r.scheme); err != nil {
			return err
		}

		r.helper.SetMetaData(obj, name, namespace)
	}

	found := obj.DeepCopy()

	key := types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}

	err := r.kubeClient.Get(ctx, key, found)

	if apierrors.IsNotFound(err) {
		oneTimer, err := r.helper.IsOneTimer(obj)
		if err != nil {
			return fmt.Errorf("could not determine if the object is a one-timer: %w", err)
		}

		// We are not recreating all objects if a release is already installed
		if releaseInstalled && oneTimer {
			return nil
		}

		logg.Info("Creating resource", "releaseInstalled", releaseInstalled, "oneTimer", oneTimer)

		if err = utils.Annotate(obj); err != nil {
			return fmt.Errorf("can not annotate with hash: %w", err)
		}

		// If we create the resource set the owner reference
		if err = controllerutil.SetControllerReference(owner, obj, r.scheme); err != nil {
			return fmt.Errorf("could not set the owner reference: %w", err)
		}

		r.helper.SetMetaData(obj, name, namespace)

		if err = r.kubeClient.Create(ctx, obj); err != nil {
			if apierrors.IsForbidden(err) {
				return fmt.Errorf("API error: forbidden: %w", err)
			}

			return fmt.Errorf("unknown error: %w", err)
		}

		return nil
	}

	if apierrors.IsForbidden(err) {
		return fmt.Errorf("forbidden: check Role, ClusterRole and Bindings for operator: %w", err)
	}

	if err != nil {
		return fmt.Errorf("unexpected error: %w", err)
	}

	// Not updating Pod because we can only update image and some other
	// specific minor fields.
	if r.helper.IsNotUpdateable(obj.GetKind()) {
		return nil
	}

	equal, err := utils.AnnotationEqual(found, obj)
	if err != nil {
		return err
	}
	if equal {
		return nil
	}

	logg.Info("Updating resource")
	required := obj.DeepCopy()

	if err = utils.Annotate(required); err != nil {
		return fmt.Errorf("can not annotate with hash: %w", err)
	}

	// required.ResourceVersion = found.ResourceVersion this is only needed
	// before we update a resource, we do not care when creating, hence
	// !leave this here!
	if err = r.helper.UpdateResourceVersion(required, found); err != nil {
		return fmt.Errorf("couldn't Update ResourceVersion: %w", err)
	}

	if err = r.kubeClient.Update(ctx, required); err != nil {
		return fmt.Errorf("couldn't Update Resource: %w", err)
	}

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
		r.log.Error(err, "Could not get PodList")
		return err
	}

	if len(pods.Items) == 0 {
		return fmt.Errorf("no Pods found, reconciling")
	}

	var reason string

	for _, pod := range pods.Items {

		containerStatuses := pod.Status.ContainerStatuses

		if containerStatuses == nil {
			phase := pod.Status.Phase
			if phase == "" {
				return errors.New("pod has an empty phase")
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
				return fmt.Errorf("ImagePullBackOff need to rebuild %s driver-container", UpdateVendor)
			}
		}
		UpdateVendor = ""
		return nil
	}

	return fmt.Errorf("unexpected Phase of Pods in DameonSet: %s", obj.GetName())
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
	operatingSystemMajorMinor string) error {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{},
	}

	jsonSpec, err := yaml.YAMLToJSON(yamlSpec)
	if err != nil {
		return fmt.Errorf("Could not convert yaml file to json: %s: error %w", string(yamlSpec), err)
	}

	if err = obj.UnmarshalJSON(jsonSpec); err != nil {
		return fmt.Errorf("cannot unmarshall json spec, check your manifest: %s: %w", jsonSpec, err)
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
	if err = r.helper.SetLabel(obj, filter.OwnedLabel); err != nil {
		return fmt.Errorf("could not set label: %w", err)
	}
	// kernel affinity related attributes only set if there is an
	// annotation specialresource.openshift.io/kernel-affine: true
	if r.kernelAPI.IsObjectAffine(obj) {
		if err = r.kernelAPI.SetAffineAttributes(obj, kernelFullVersion, operatingSystemMajorMinor); err != nil {
			return fmt.Errorf("cannot set kernel affine attributes: %w", err)
		}
	}

	// Add nodeSelector terms defined for the specialresource CR to the object
	// we do not want to spread HW enablement stacks on all nodes
	if err = r.helper.SetNodeSelectorTerms(obj, nodeSelector); err != nil {
		return fmt.Errorf("setting NodeSelectorTerms failed: %w", err)
	}

	// We are only building a driver-container if we cannot pull the image
	// We are asuming that vendors provide pre compiled DriverContainers
	// If err == nil, build a new container, if err != nil skip it
	if err = r.rebuildDriverContainer(obj); err != nil {
		r.log.Info("Skipping building driver-container", "Name", obj.GetName())
		return nil
	}

	// Callbacks before CRUD will update the manifests
	if err = r.BeforeCRUD(obj, owner); err != nil {
		return fmt.Errorf("before CRUD hooks failed: %w", err)
	}
	// Create Update Delete Patch resources
	err = r.CRUD(ctx, obj, releaseInstalled, owner, name, namespace)
	if err != nil {
		if strings.Contains(err.Error(), "failed calling webhook") {
			return fmt.Errorf("webhook not ready, requeue: %w", err)
		}

		return fmt.Errorf("CRUD exited non-zero on Object: %+v: %w", obj, err)
	}

	// Callbacks after CRUD will wait for ressource and check status
	if err = r.AfterCRUD(ctx, obj, namespace); err != nil {
		return fmt.Errorf("after CRUD hooks failed: %w", err)
	}

	r.sendNodesMetrics(ctx, obj, name)

	metricValue = 1
	return nil
}

func (r *resource) rebuildDriverContainer(obj *unstructured.Unstructured) error {
	// BuildConfig are currently not triggered by an update need to delete first
	if obj.GetKind() == "BuildConfig" {
		annotations := obj.GetAnnotations()
		if vendor, ok := annotations["specialresource.openshift.io/driver-container-vendor"]; ok {
			if vendor == UpdateVendor {
				return nil
			}
			return errors.New("vendor != updateVendor")
		}
		return nil
	}
	return nil
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
		r.log.Info("No assigned nodes for found for UsedNodes metric", "kind", kind, "name", obj.GetName(), "crName", crName)
	}
}

func (r *resource) BeforeCRUD(obj *unstructured.Unstructured, sr interface{}) error {
	annotations := obj.GetAnnotations()
	if valid, found := annotations["specialresource.openshift.io/proxy"]; found && valid == "true" {
		if err := r.proxyAPI.Setup(obj); err != nil {
			return fmt.Errorf("could not setup Proxy: %w", err)
		}
	}
	return nil
}
