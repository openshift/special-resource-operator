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

	"github.com/openshift-psap/special-resource-operator/internal/resourcehelper"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/filter"
	"github.com/openshift-psap/special-resource-operator/pkg/kernel"
	"github.com/openshift-psap/special-resource-operator/pkg/lifecycle"
	"github.com/openshift-psap/special-resource-operator/pkg/metrics"
	"github.com/openshift-psap/special-resource-operator/pkg/poll"
	"github.com/openshift-psap/special-resource-operator/pkg/proxy"
	"github.com/openshift-psap/special-resource-operator/pkg/utils"
	"github.com/openshift-psap/special-resource-operator/pkg/yamlutil"
)

var (
	UpdateVendor string
)

//go:generate mockgen -source=resource.go -package=resource -destination=mock_resource_api.go

type Creator interface {
	CreateFromYAML(context.Context, []byte, bool, v1.Object, string, string, map[string]string, string, string) error
}

type creator struct {
	kubeClient    clients.ClientsInterface
	lc            lifecycle.Lifecycle
	log           logr.Logger
	metricsClient metrics.Metrics
	pollActions   poll.PollActions
	kernelData    kernel.KernelData
	proxyAPI      proxy.ProxyAPI
	scheme        *runtime.Scheme
	helper        resourcehelper.Helper
}

func NewCreator(
	kubeClient clients.ClientsInterface,
	metricsClient metrics.Metrics,
	pollActions poll.PollActions,
	kernelData kernel.KernelData,
	scheme *runtime.Scheme,
	lc lifecycle.Lifecycle,
	proxyAPI proxy.ProxyAPI,
	resHelper resourcehelper.Helper,
) Creator {
	return &creator{
		kubeClient:    kubeClient,
		lc:            lc,
		log:           zap.New(zap.UseDevMode(true)).WithName(utils.Print("resource", utils.Blue)),
		metricsClient: metricsClient,
		pollActions:   pollActions,
		kernelData:    kernelData,
		scheme:        scheme,
		proxyAPI:      proxyAPI,
		helper:        resHelper,
	}
}

func (c *creator) AfterCRUD(ctx context.Context, obj *unstructured.Unstructured, namespace string) error {

	logger := c.log.WithValues("Kind", obj.GetObjectKind(), "namespace", obj.GetNamespace(), "name", obj.GetName())

	annotations := obj.GetAnnotations()
	clients.Namespace = namespace

	if state, found := annotations["specialresource.openshift.io/state"]; found && state == "driver-container" {
		logger.Info("specialresource.openshift.io/state")
		if err := c.checkForImagePullBackOff(ctx, obj, namespace); err != nil {
			return fmt.Errorf("cannot check for ImagePullBackOff: %w", err)
		}
	}

	if wait, found := annotations["specialresource.openshift.io/wait"]; found && wait == "true" {
		logger.Info("specialresource.openshift.io/wait")
		if err := c.pollActions.ForResource(ctx, obj); err != nil {
			return fmt.Errorf("could not wait for resource: %w", err)
		}
	}

	if pattern, found := annotations["specialresource.openshift.io/wait-for-logs"]; found && len(pattern) > 0 {
		logger.Info("specialresource.openshift.io/wait-for-logs")
		if err := c.pollActions.ForDaemonSetLogs(ctx, obj, pattern); err != nil {
			return fmt.Errorf("could not wait for DaemonSet logs: %w", err)
		}
	}

	if _, found := annotations["helm.sh/hook"]; found {
		// In the case of hooks we're always waiting for all ressources
		if err := c.pollActions.ForResource(ctx, obj); err != nil {
			return fmt.Errorf("could not wait for resource: %w", err)
		}
	}

	// Always wait for CRDs to be present
	if obj.GetKind() == "CustomResourceDefinition" {
		if err := c.pollActions.ForResource(ctx, obj); err != nil {
			return fmt.Errorf("could not wait for CRD: %w", err)
		}
	}

	return nil
}

func (c *creator) CreateFromYAML(
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

		err := c.createObjFromYAML(
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

// CRUD Create Update Delete Resource
func (c *creator) CRUD(ctx context.Context, obj *unstructured.Unstructured, releaseInstalled bool, owner v1.Object, name string, namespace string) error {

	var logg logr.Logger
	if c.helper.IsNamespaced(obj.GetKind()) {
		logg = c.log.WithValues("Kind", obj.GetKind()+": "+obj.GetNamespace()+"/"+obj.GetName())
	} else {
		logg = c.log.WithValues("Kind", obj.GetKind()+": "+obj.GetName())
	}

	// SpecialResource is the parent, all other objects are childs and need a reference
	// but only set the ownerreference if created by SRO do not set ownerreference per default
	if obj.GetKind() != "SpecialResource" && obj.GetKind() != "Namespace" {
		if err := controllerutil.SetControllerReference(owner, obj, c.scheme); err != nil {
			return err
		}

		c.helper.SetMetaData(obj, name, namespace)
	}

	found := obj.DeepCopy()

	key := types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}

	err := c.kubeClient.Get(ctx, key, found)

	if apierrors.IsNotFound(err) {
		oneTimer, err := c.helper.IsOneTimer(obj)
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
		if err = controllerutil.SetControllerReference(owner, obj, c.scheme); err != nil {
			return fmt.Errorf("could not set the owner reference: %w", err)
		}

		c.helper.SetMetaData(obj, name, namespace)

		if err = c.kubeClient.Create(ctx, obj); err != nil {
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
	if c.helper.IsNotUpdateable(obj.GetKind()) {
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
	if err = c.helper.UpdateResourceVersion(required, found); err != nil {
		return fmt.Errorf("couldn't Update ResourceVersion: %w", err)
	}

	if err = c.kubeClient.Update(ctx, required); err != nil {
		return fmt.Errorf("couldn't Update Resource: %w", err)
	}

	return nil
}

func (c *creator) checkForImagePullBackOff(ctx context.Context, obj *unstructured.Unstructured, namespace string) error {

	if err := c.pollActions.ForDaemonSet(ctx, obj); err == nil {
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

	err := c.kubeClient.List(ctx, pods, opts...)
	if err != nil {
		c.log.Error(err, "Could not get PodList")
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

func (c *creator) createObjFromYAML(
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
	if c.helper.IsNamespaced(obj.GetKind()) && obj.GetNamespace() == "" {
		obj.SetNamespace(namespace)
	}

	yamlKind := obj.GetKind()
	yamlName := obj.GetName()
	yamlNamespace := obj.GetNamespace()
	metricValue := 0
	defer func() {
		c.metricsClient.SetCompletedKind(name, yamlKind, yamlName, yamlNamespace, metricValue)
	}()

	// We used this for predicate filtering, we're watching a lot of
	// API Objects we want to ignore all objects that do not have this
	// label.
	if err = c.helper.SetLabel(obj, filter.OwnedLabel); err != nil {
		return fmt.Errorf("could not set label: %w", err)
	}
	// kernel affinity related attributes only set if there is an
	// annotation specialresource.openshift.io/kernel-affine: true
	if c.kernelData.IsObjectAffine(obj) {
		if err = c.kernelData.SetAffineAttributes(obj, kernelFullVersion, operatingSystemMajorMinor); err != nil {
			return fmt.Errorf("cannot set kernel affine attributes: %w", err)
		}
	}

	// Add nodeSelector terms defined for the specialresource CR to the object
	// we do not want to spread HW enablement stacks on all nodes
	if err = c.helper.SetNodeSelectorTerms(obj, nodeSelector); err != nil {
		return fmt.Errorf("setting NodeSelectorTerms failed: %w", err)
	}

	// We are only building a driver-container if we cannot pull the image
	// We are asuming that vendors provide pre compiled DriverContainers
	// If err == nil, build a new container, if err != nil skip it
	if err = c.rebuildDriverContainer(obj); err != nil {
		c.log.Info("Skipping building driver-container", "Name", obj.GetName())
		return nil
	}

	// Callbacks before CRUD will update the manifests
	if err = c.BeforeCRUD(obj, owner); err != nil {
		return fmt.Errorf("before CRUD hooks failed: %w", err)
	}
	// Create Update Delete Patch resources
	err = c.CRUD(ctx, obj, releaseInstalled, owner, name, namespace)
	if err != nil {
		if strings.Contains(err.Error(), "failed calling webhook") {
			return fmt.Errorf("webhook not ready, requeue: %w", err)
		}

		return fmt.Errorf("CRUD exited non-zero on Object: %+v: %w", obj, err)
	}

	// Callbacks after CRUD will wait for ressource and check status
	if err = c.AfterCRUD(ctx, obj, namespace); err != nil {
		return fmt.Errorf("after CRUD hooks failed: %w", err)
	}

	c.sendNodesMetrics(ctx, obj, name)

	metricValue = 1
	return nil
}

func (c *creator) rebuildDriverContainer(obj *unstructured.Unstructured) error {
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

func (c *creator) sendNodesMetrics(ctx context.Context, obj *unstructured.Unstructured, crName string) {
	kind := obj.GetKind()
	if kind != "DaemonSet" && kind != "Deployment" {
		return
	}

	objKey := types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
	getPodsFunc := c.lc.GetPodFromDaemonSet
	if kind == "Deployment" {
		getPodsFunc = c.lc.GetPodFromDeployment
	}

	pl := getPodsFunc(ctx, objKey)
	nodesNames := []string{}
	for _, pod := range pl.Items {
		nodesNames = append(nodesNames, pod.Spec.NodeName)
	}

	if len(nodesNames) != 0 {
		nodesStr := strings.Join(nodesNames, ",")
		c.metricsClient.SetUsedNodes(crName, obj.GetName(), obj.GetNamespace(), kind, nodesStr)
	} else {
		c.log.Info("No assigned nodes for found for UsedNodes metric", "kind", kind, "name", obj.GetName(), "crName", crName)
	}
}

func (c *creator) BeforeCRUD(obj *unstructured.Unstructured, sr interface{}) error {
	annotations := obj.GetAnnotations()
	if valid, found := annotations["specialresource.openshift.io/proxy"]; found && valid == "true" {
		if err := c.proxyAPI.Setup(obj); err != nil {
			return fmt.Errorf("could not setup Proxy: %w", err)
		}
	}
	return nil
}
