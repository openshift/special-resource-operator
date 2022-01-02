package resource

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"github.com/openshift-psap/special-resource-operator/pkg/filter"
	"github.com/openshift-psap/special-resource-operator/pkg/hash"
	"github.com/openshift-psap/special-resource-operator/pkg/kernel"
	"github.com/openshift-psap/special-resource-operator/pkg/lifecycle"
	"github.com/openshift-psap/special-resource-operator/pkg/metrics"
	"github.com/openshift-psap/special-resource-operator/pkg/poll"
	"github.com/openshift-psap/special-resource-operator/pkg/proxy"
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

type resourceCallbacks map[string]func(obj *unstructured.Unstructured, sr interface{}) error

var (
	customCallback = make(resourceCallbacks)
	UpdateVendor   string
)

//go:generate mockgen -source=resource.go -package=resource -destination=mock_resource_api.go

type Creator interface {
	CreateFromYAML([]byte, bool, v1.Object, string, string, map[string]string, string, string) error
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
}

func NewCreator(
	kubeClient clients.ClientsInterface,
	metricsClient metrics.Metrics,
	pollActions poll.PollActions,
	kernelData kernel.KernelData,
	scheme *runtime.Scheme,
	lc lifecycle.Lifecycle,
	proxyAPI proxy.ProxyAPI,
) Creator {
	return &creator{
		kubeClient:    kubeClient,
		lc:            lc,
		log:           zap.New(zap.UseDevMode(true)).WithName(color.Print("resource", color.Blue)),
		metricsClient: metricsClient,
		pollActions:   pollActions,
		kernelData:    kernelData,
		scheme:        scheme,
		proxyAPI:      proxyAPI,
	}
}

func (c *creator) AfterCRUD(obj *unstructured.Unstructured, namespace string) error {

	annotations := obj.GetAnnotations()
	clients.Namespace = namespace

	if state, found := annotations["specialresource.openshift.io/state"]; found && state == "driver-container" {
		c.log.Info("specialresource.openshift.io/state")
		if err := c.checkForImagePullBackOff(obj, namespace); err != nil {
			return fmt.Errorf("cannot check for ImagePullBackOff: %w", err)
		}
	}

	if wait, found := annotations["specialresource.openshift.io/wait"]; found && wait == "true" {
		c.log.Info("specialresource.openshift.io/wait")
		if err := c.pollActions.ForResource(obj); err != nil {
			return fmt.Errorf("could not wait for resource: %w", err)
		}
	}

	if pattern, found := annotations["specialresource.openshift.io/wait-for-logs"]; found && len(pattern) > 0 {
		c.log.Info("specialresource.openshift.io/wait-for-logs")
		if err := c.pollActions.ForDaemonSetLogs(obj, pattern); err != nil {
			return fmt.Errorf("could not wait for DaemonSet logs: %w", err)
		}
	}

	if _, found := annotations["helm.sh/hook"]; found {
		// In the case of hooks we're always waiting for all ressources
		if err := c.pollActions.ForResource(obj); err != nil {
			return fmt.Errorf("could not wait for resource: %w", err)
		}
	}

	// Always wait for CRDs to be present
	if obj.GetKind() == "CustomResourceDefinition" {
		if err := c.pollActions.ForResource(obj); err != nil {
			return fmt.Errorf("could not wait for CRD: %w", err)
		}
	}

	return nil
}

func (c *creator) CreateFromYAML(yamlFile []byte,
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

		err := c.createObjFromYAML(yamlSpec,
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
func (c *creator) CRUD(obj *unstructured.Unstructured, releaseInstalled bool, owner v1.Object, name string, namespace string) error {

	var logg logr.Logger
	if IsNamespaced(obj.GetKind()) {
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

		SetMetaData(obj, name, namespace)
	}

	found := obj.DeepCopy()

	key := types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}

	err := c.kubeClient.Get(context.TODO(), key, found)

	if apierrors.IsNotFound(err) {
		oneTimer, err := IsOneTimer(obj)
		if err != nil {
			return fmt.Errorf("could not determine if the object is a one-timer: %w", err)
		}

		// We are not recreating all objects if a release is already installed
		if releaseInstalled && oneTimer {
			logg.Info("Skipping creation")
			return nil
		}

		logg.Info("Not found, creating")

		logg.Info("Release", "Installed", releaseInstalled)
		logg.Info("Is", "OneTimer", oneTimer)

		if err = hash.Annotate(obj); err != nil {
			return fmt.Errorf("can not annotate with hash: %w", err)
		}

		// If we create the resource set the owner reference
		if err = controllerutil.SetControllerReference(owner, obj, c.scheme); err != nil {
			return fmt.Errorf("could not set the owner reference: %w", err)
		}

		SetMetaData(obj, name, namespace)

		if err = c.kubeClient.Create(context.TODO(), obj); err != nil {
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
	if IsNotUpdateable(obj.GetKind()) {
		logg.Info("Not Updateable", "Resource", obj.GetKind())
		return nil
	}

	equal, err := hash.AnnotationEqual(found, obj)
	if err != nil {
		return err
	}
	if equal {
		logg.Info("Found, not updating, hash the same: " + found.GetKind() + "/" + found.GetName())
		return nil
	}

	logg.Info("Found, updating")
	required := obj.DeepCopy()

	if err = hash.Annotate(required); err != nil {
		return fmt.Errorf("can not annotate with hash: %w", err)
	}

	// required.ResourceVersion = found.ResourceVersion this is only needed
	// before we update a resource, we do not care when creating, hence
	// !leave this here!
	if err = UpdateResourceVersion(required, found); err != nil {
		return fmt.Errorf("couldn't Update ResourceVersion: %w", err)
	}

	if err = c.kubeClient.Update(context.TODO(), required); err != nil {
		return fmt.Errorf("couldn't Update Resource: %w", err)
	}

	return nil
}

func (c *creator) checkForImagePullBackOff(obj *unstructured.Unstructured, namespace string) error {

	if err := c.pollActions.ForDaemonSet(obj); err == nil {
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

	c.log.Info("checkForImagePullBackOff get PodList from: " + namespace)

	opts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels(find),
	}

	err := c.kubeClient.List(context.TODO(), pods, opts...)
	if err != nil {
		c.log.Error(err, "Could not get PodList")
		return err
	}

	if len(pods.Items) == 0 {
		return fmt.Errorf("no Pods found, reconciling")
	}

	var reason string

	for _, pod := range pods.Items {
		c.log.Info("checkForImagePullBackOff", "PodName", pod.GetName())

		var found bool
		var containerStatuses []interface{}

		if containerStatuses, found, err = unstructured.NestedSlice(pod.Object, "status", "containerStatuses"); !found || err != nil {
			var phase string

			phase, found, err = unstructured.NestedString(pod.Object, "status", "phase")
			if err != nil || !found {
				return fmt.Errorf("error or not found: %w", err)
			}

			c.log.Info("Pod is in phase: " + phase)
			continue
		}

		for _, containerStatus := range containerStatuses {
			switch cs := containerStatus.(type) {
			case map[string]interface{}:
				reason, _, _ = unstructured.NestedString(cs, "state", "waiting", "reason")
				c.log.Info("Reason", "reason", reason)
			default:
				c.log.Info("checkForImagePullBackOff", "DEFAULT NOT THE CORRECT TYPE", cs)
			}
			break
		}

		if reason == "ImagePullBackOff" || reason == "ErrImagePull" {
			annotations := obj.GetAnnotations()
			if vendor, ok := annotations["specialresource.openshift.io/driver-container-vendor"]; ok {
				UpdateVendor = vendor
				return fmt.Errorf("ImagePullBackOff need to rebuild %s driver-container", UpdateVendor)
			}
		}

		c.log.Info("Unsetting updateVendor, Pods not in ImagePullBackOff or ErrImagePull")
		UpdateVendor = ""
		return nil
	}

	return fmt.Errorf("unexpected Phase of Pods in DameonSet: %s", obj.GetName())
}

func (c *creator) createObjFromYAML(yamlSpec []byte,
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
	if IsNamespaced(obj.GetKind()) && obj.GetNamespace() == "" {
		c.log.Info("Namespace empty settting", "namespace", namespace)
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
	if err = SetLabel(obj); err != nil {
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
	if err = SetNodeSelectorTerms(obj, nodeSelector); err != nil {
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
	err = c.CRUD(obj, releaseInstalled, owner, name, namespace)
	if err != nil {
		if strings.Contains(err.Error(), "failed calling webhook") {
			return fmt.Errorf("webhook not ready, requeue: %w", err)
		}

		return fmt.Errorf("CRUD exited non-zero on Object: %+v: %w", obj, err)
	}

	// Callbacks after CRUD will wait for ressource and check status
	if err = c.AfterCRUD(obj, namespace); err != nil {
		return fmt.Errorf("after CRUD hooks failed: %w", err)
	}

	c.sendNodesMetrics(obj, name)

	metricValue = 1
	return nil
}

func (c *creator) rebuildDriverContainer(obj *unstructured.Unstructured) error {

	logger := c.log.WithValues("Kind", obj.GetKind(), "Namespace", obj.GetNamespace(), "Name", obj.GetName())
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

func (c *creator) sendNodesMetrics(obj *unstructured.Unstructured, crName string) {
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

	pl := getPodsFunc(objKey)
	nodesNames := []string{}
	for _, pod := range pl.Items {
		nodeName, found, err := unstructured.NestedString(pod.Object, "spec", "nodeName")
		if err == nil && found {
			nodesNames = append(nodesNames, nodeName)
		}
	}

	if len(nodesNames) != 0 {
		nodesStr := strings.Join(nodesNames, ",")
		c.metricsClient.SetUsedNodes(crName, obj.GetName(), obj.GetNamespace(), kind, nodesStr)
	} else {
		c.log.Info("No assigned nodes for found for UsedNodes metric", "kind", kind, "name", obj.GetName(), "crName", crName)
	}
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
		if err != nil || !fnd {
			return fmt.Errorf("error or resourceVersion not found: %w", err)
		}

		if err = unstructured.SetNestedField(req.Object, version, "metadata", "resourceVersion"); err != nil {
			return fmt.Errorf("couldn't update ResourceVersion: %w", err)
		}

	}
	if kind == "Service" {
		clusterIP, fnd, err := unstructured.NestedString(found.Object, "spec", "clusterIP")
		if err != nil || !fnd {
			return fmt.Errorf("error or clusterIP not found: %w", err)
		}

		if err = unstructured.SetNestedField(req.Object, clusterIP, "spec", "clusterIP"); err != nil {
			return fmt.Errorf("couldn't update clusterIP: %w", err)
		}
	}

	return nil
}

func SetNodeSelectorTerms(obj *unstructured.Unstructured, terms map[string]string) error {

	if strings.Compare(obj.GetKind(), "DaemonSet") == 0 ||
		strings.Compare(obj.GetKind(), "Deployment") == 0 ||
		strings.Compare(obj.GetKind(), "Statefulset") == 0 { // TODO(qbarrand) should this be StatefulSet?
		if err := nodeSelectorTerms(terms, obj, "spec", "template", "spec", "nodeSelector"); err != nil {
			return fmt.Errorf("cannot setup %s nodeSelector: %w", obj.GetKind(), err)
		}
	}
	if strings.Compare(obj.GetKind(), "Pod") == 0 {
		if err := nodeSelectorTerms(terms, obj, "spec", "nodeSelector"); err != nil {
			return fmt.Errorf("cannot setup Pod nodeSelector: %w", err)
		}
	}
	if strings.Compare(obj.GetKind(), "BuildConfig") == 0 {
		if err := nodeSelectorTerms(terms, obj, "spec", "nodeSelector"); err != nil {
			return fmt.Errorf("cannot setup BuildConfig nodeSelector: %w", err)
		}
	}

	return nil
}

func nodeSelectorTerms(terms map[string]string, obj *unstructured.Unstructured, fields ...string) error {

	nodeSelector, found, err := unstructured.NestedMap(obj.Object, fields...)
	if err != nil {
		return err
	}

	if !found {
		nodeSelector = make(map[string]interface{})
	}

	for k, v := range terms {
		nodeSelector[k] = v
	}

	if err = unstructured.SetNestedMap(obj.Object, nodeSelector, fields...); err != nil {
		return fmt.Errorf("cannot update nodeSelector for %s : %w", obj.GetName(), err)
	}

	return nil
}

func IsOneTimer(obj *unstructured.Unstructured) (bool, error) {

	// We are not recreating Pods that have restartPolicy: Never
	if obj.GetKind() == "Pod" {
		restartPolicy, found, err := unstructured.NestedString(obj.Object, "spec", "restartPolicy")
		if err != nil || !found {
			return false, fmt.Errorf("error or restartPolicy not found: %w", err)
		}

		if restartPolicy == "Never" {
			return true, nil
		}
	}

	return false, nil
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

func (c *creator) BeforeCRUD(obj *unstructured.Unstructured, sr interface{}) error {

	var found bool
	todo := ""
	annotations := obj.GetAnnotations()

	if valid, found := annotations["specialresource.openshift.io/proxy"]; found && valid == "true" {
		if err := c.proxyAPI.Setup(obj); err != nil {
			return fmt.Errorf("could not setup Proxy: %w", err)
		}
	}

	if todo, found = annotations["specialresource.openshift.io/callback"]; !found {
		return nil
	}

	if prefix, ok := customCallback[todo]; ok {
		if err := prefix(obj, sr); err != nil {
			return fmt.Errorf("could not run prefix callback: %w", err)
		}
	}
	return nil
}

func SetLabel(obj *unstructured.Unstructured) error {

	var labels map[string]string

	if labels = obj.GetLabels(); labels == nil {
		labels = make(map[string]string)
	}

	labels[filter.OwnedLabel] = "true"
	obj.SetLabels(labels)

	return setSubResourceLabel(obj)
}

func setSubResourceLabel(obj *unstructured.Unstructured) error {

	if obj.GetKind() == "DaemonSet" || obj.GetKind() == "Deployment" ||
		obj.GetKind() == "StatefulSet" {

		labels, found, err := unstructured.NestedMap(obj.Object, "spec", "template", "metadata", "labels")
		if err != nil {
			return err
		}
		if !found {
			return errors.New("Labels not found")
		}

		labels[filter.OwnedLabel] = "true"
		if err := unstructured.SetNestedMap(obj.Object, labels, "spec", "template", "metadata", "labels"); err != nil {
			return err
		}
	}

	// TODO: how to set label ownership for Builds and related Pods
	/*
		if obj.GetKind() == "BuildConfig" {
			output, found, err := unstructured.NestedMap(obj.Object, "spec", "output")
			if err != nil {
				return err
			}
			if !found {
				return errors.New("output not found")
			}

			label := make(map[string]interface{})
			label["name"] = filter.OwnedLabel
			label["value"] = "true"
			imageLabels := append(make([]interface{}, 0), label)

			if _, found := output["imageLabels"]; !found {
				err := unstructured.SetNestedSlice(obj.Object, imageLabels, "spec", "output", "imageLabels")
				if err != nil {
					return err
				}
			}
		}
	*/
	return nil
}
