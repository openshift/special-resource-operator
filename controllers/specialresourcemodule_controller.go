/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	buildv1 "github.com/openshift/api/build/v1"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	k8sruntime "k8s.io/apimachinery/pkg/runtime"

	srov1beta1 "github.com/openshift/special-resource-operator/api/v1beta1"
	"github.com/openshift/special-resource-operator/pkg/assets"
	"github.com/openshift/special-resource-operator/pkg/clients"
	"github.com/openshift/special-resource-operator/pkg/filter"
	"github.com/openshift/special-resource-operator/pkg/helmer"
	"github.com/openshift/special-resource-operator/pkg/metrics"
	"github.com/openshift/special-resource-operator/pkg/registry"
	"github.com/openshift/special-resource-operator/pkg/resource"
	sroruntime "github.com/openshift/special-resource-operator/pkg/runtime"
	"github.com/openshift/special-resource-operator/pkg/utils"
	"github.com/openshift/special-resource-operator/pkg/watcher"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/transport"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	semver = `^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`

	minDelaySRM = 100 * time.Millisecond
	maxDelaySRM = 3 * time.Second

	SRMgvk          = "SpecialResourceModule"
	SRMOwnedLabel   = "specialresourcemodule.openshift.io/owned"
	upgradesInfoURL = "https://api.openshift.com/api/upgrades_info/v1/graph"
)

var (
	versionRegex = regexp.MustCompile(semver)
)

type metadata struct {
	OperatingSystem       string                           `json:"operatingSystem"`
	KernelFullVersion     string                           `json:"kernelFullVersion"`
	RTKernelFullVersion   string                           `json:"rtKernelFullVersion"`
	DriverToolkitImage    string                           `json:"driverToolkitImage"`
	OpenShiftVersion      string                           `json:"openShiftVersion"`
	OSImageURL            string                           `json:"osImageURL"`
	GroupName             sroruntime.ResourceGroupName     `json:"groupName"`
	SpecialResourceModule srov1beta1.SpecialResourceModule `json:"specialResourceModule"`
}

type ocpVersionInfo struct {
	KernelVersion   string
	RTKernelVersion string
	DTKImage        string
	OSVersion       string
	OSImage         string
	OCPVersion      string
}

// SpecialResourceModuleReconciler reconciles a SpecialResourceModule object
type SpecialResourceModuleReconciler struct {
	Scheme *k8sruntime.Scheme

	Metrics     metrics.Metrics
	ResourceAPI resource.ResourceAPI
	Filter      filter.Filter
	Helmer      helmer.Helmer
	Assets      assets.Assets
	KubeClient  clients.ClientsInterface
	Registry    registry.Registry
	Watcher     watcher.Watcher
}

func FindSRM(a []srov1beta1.SpecialResourceModule, x string) (int, bool) {
	for i, n := range a {
		if x == n.GetName() {
			return i, true
		}
	}
	return -1, false
}

func getMetadata(srm srov1beta1.SpecialResourceModule, info ocpVersionInfo) metadata {
	return metadata{
		OperatingSystem:     info.OSVersion,
		KernelFullVersion:   info.KernelVersion,
		RTKernelFullVersion: info.RTKernelVersion,
		DriverToolkitImage:  info.DTKImage,
		OpenShiftVersion:    info.OCPVersion,
		OSImageURL:          info.OSImage,
		GroupName: sroruntime.ResourceGroupName{
			DriverBuild:            "driver-build",
			DriverContainer:        "driver-container",
			RuntimeEnablement:      "runtime-enablement",
			DevicePlugin:           "device-plugin",
			DeviceMonitoring:       "device-monitoring",
			DeviceDashboard:        "device-dashboard",
			DeviceFeatureDiscovery: "device-feature-discovery",
			CSIDriver:              "csi-driver",
		},
		SpecialResourceModule: srm,
	}
}

func getImageFromVersion(entry string) (string, error) {
	type versionNode struct {
		Version string `json:"version"`
		Payload string `json:"payload"`
	}
	type versionGraph struct {
		Nodes []versionNode `json:"nodes"`
	}
	res := versionRegex.FindStringSubmatch(entry)
	full, major, minor := res[0], res[1], res[2]
	var imageURL string
	{
		config := transport.Config{
			UserAgent: rest.DefaultKubernetesUserAgent() + "(release-info)",
		}
		roundTripper := http.DefaultTransport
		rt, err := transport.HTTPWrappersForConfig(&config, roundTripper)
		if err != nil {
			return "", fmt.Errorf("failed to wrap config with default roundTripper: %w", err)
		}
		cl := &http.Client{Transport: rt}
		u, err := url.Parse(upgradesInfoURL)
		if err != nil {
			return "", fmt.Errorf("failed to parse URL %s: %w", upgradesInfoURL, err)
		}
		for _, stream := range []string{"fast", "stable", "candidate"} {
			u.RawQuery = url.Values{"channel": []string{fmt.Sprintf("%s-%s.%s", stream, major, minor)}}.Encode()
			if err := func() error {
				req, err := http.NewRequest("GET", u.String(), nil)
				if err != nil {
					return fmt.Errorf("failed to create HTTP request 'GET %s': %w", u.String(), err)
				}
				req.Header.Set("Accept", "application/json")
				resp, err := cl.Do(req)
				if err != nil {
					return fmt.Errorf("failed to send HTTP request: %w", err)
				}
				defer resp.Body.Close()
				switch resp.StatusCode {
				case http.StatusOK:
				default:
					_, _ = io.Copy(ioutil.Discard, resp.Body)
					return fmt.Errorf("unable to retrieve image, status code %d", resp.StatusCode)
				}
				data, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					return fmt.Errorf("unable to read reponse: %w", err)
				}
				var versions versionGraph
				if err := json.Unmarshal(data, &versions); err != nil {
					return fmt.Errorf("unable to unmarshal json: %w", err)
				}
				for _, version := range versions.Nodes {
					if version.Version == full && len(version.Payload) > 0 {
						imageURL = version.Payload
						break
					}
				}

				return nil
			}(); err != nil {
				return "", fmt.Errorf("failed to get image from version %s: %w", entry, err)
			}
		}
		if len(imageURL) == 0 {
			return imageURL, fmt.Errorf("version %s not found", entry)
		}
	}
	return imageURL, nil
}

func (r *SpecialResourceModuleReconciler) createNamespace(ctx context.Context, resource srov1beta1.SpecialResourceModule) error {

	ns := []byte(`apiVersion: v1
kind: Namespace
metadata:
  annotations:
    specialresource.openshift.io/wait: "true"
    openshift.io/cluster-monitoring: "true"
  name: `)

	if resource.Spec.Namespace == "" {
		resource.Spec.Namespace = resource.Name
	}
	ns = append(ns, []byte(resource.Spec.Namespace)...)

	if err := r.ResourceAPI.CreateFromYAML(ctx, ns, false, &resource, resource.Name, "", nil, "", "", SRMOwnedLabel); err != nil {
		return fmt.Errorf("unable to create namespace '%s': %w", resource.Name, err)
	}
	return nil
}

func (r *SpecialResourceModuleReconciler) deleteNamespace(ctx context.Context, name string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	if err := r.KubeClient.Delete(ctx, ns); err != nil {
		return fmt.Errorf("unable to delete namespace '%s': %w", name, err)
	}
	return nil
}

func (r *SpecialResourceModuleReconciler) getAllResources(kind, apiVersion, namespace, name string) ([]unstructured.Unstructured, error) {
	if name == "" {
		var l unstructured.UnstructuredList
		l.SetKind(kind)
		l.SetAPIVersion(apiVersion)
		if err := r.KubeClient.List(context.Background(), &l); err != nil {
			return nil, fmt.Errorf("failed to list '%s': %w", kind, err)
		}
		return l.Items, nil
	}
	obj := unstructured.Unstructured{}
	obj.SetKind(kind)
	obj.SetAPIVersion(apiVersion)
	obj.SetNamespace(namespace)
	obj.SetName(name)
	key := client.ObjectKeyFromObject(&obj)
	if err := r.KubeClient.Get(context.Background(), key, &obj); err != nil {
		return []unstructured.Unstructured{}, fmt.Errorf("failed to get object kind  %s '%s/%s': %w", kind, name, namespace, err)
	}
	return []unstructured.Unstructured{obj}, nil
}

func (r *SpecialResourceModuleReconciler) filterResources(selectors []srov1beta1.SpecialResourceModuleSelector, objs []unstructured.Unstructured) ([]unstructured.Unstructured, error) {
	if len(selectors) == 0 {
		return objs, nil
	}
	filteredObjects := make([]unstructured.Unstructured, 0)
	for _, selector := range selectors {
		for _, obj := range objs {
			candidates, err := watcher.GetJSONPath(selector.Path, obj)
			if err != nil {
				return []unstructured.Unstructured{}, fmt.Errorf("failed to get json path for '%s': %w", selector.Path, err)
			}
			found := false
			for _, candidate := range candidates {
				if candidate == selector.Value {
					found = true
					break
				}
			}
			if selector.Exclude {
				found = !found
			}
			if found {
				filteredObjects = append(filteredObjects, obj)
			}
		}
	}
	return filteredObjects, nil
}

func (r *SpecialResourceModuleReconciler) getVersionInfoFromImage(ctx context.Context, entry string) (ocpVersionInfo, error) {
	manifestsLastLayer, err := r.Registry.LastLayer(ctx, entry)
	if err != nil {
		return ocpVersionInfo{}, fmt.Errorf("failed to get manifest's last layer for image '%s': %w", entry, err)
	}
	ocpVersion, err := r.Registry.ReleaseMetadataOCPVersion(manifestsLastLayer)
	if err != nil {
		return ocpVersionInfo{}, fmt.Errorf("failed to get OCP version from image '%s': %w", entry, err)
	}
	dtkURL, err := r.Registry.ReleaseManifests(manifestsLastLayer)
	if err != nil {
		return ocpVersionInfo{}, fmt.Errorf("failed to get DTK URL from image '%s': %w", entry, err)
	}
	dtkLastLayer, err := r.Registry.LastLayer(ctx, dtkURL)
	if err != nil {
		return ocpVersionInfo{}, fmt.Errorf("failed to get DTK's last layer for image '%s': %w", dtkURL, err)
	}
	dtkEntry, err := r.Registry.ExtractToolkitRelease(dtkLastLayer)
	if err != nil {
		return ocpVersionInfo{}, fmt.Errorf("failed to extract toolkit release from layer '%s': %w", dtkLastLayer, err)
	}

	runningArch := runtime.GOARCH
	switch runningArch {
	case "amd64":
		runningArch = "x86_64"
	case "arm64":
		runningArch = "aarch64"
	}
	if !strings.Contains(dtkEntry.KernelFullVersion, runningArch) {
		dtkEntry.KernelFullVersion = dtkEntry.KernelFullVersion + "." + runningArch
		dtkEntry.RTKernelFullVersion = dtkEntry.RTKernelFullVersion + "." + runningArch
	}

	return ocpVersionInfo{
		KernelVersion:   dtkEntry.KernelFullVersion,
		RTKernelVersion: dtkEntry.RTKernelFullVersion,
		DTKImage:        dtkURL,
		OSVersion:       dtkEntry.OSVersion,
		OSImage:         entry,
		OCPVersion:      ocpVersion,
	}, nil
}

func (r *SpecialResourceModuleReconciler) getOCPVersions(ctx context.Context, watchList []srov1beta1.SpecialResourceModuleWatch) (map[string]ocpVersionInfo, error) {
	log := ctrl.LoggerFrom(ctx)
	versionMap := make(map[string]ocpVersionInfo)
	for _, resource := range watchList {
		objs, err := r.getAllResources(resource.Kind, resource.ApiVersion, resource.Namespace, resource.Name)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				continue
			}
			return nil, fmt.Errorf("failed to get '%s/%s' in namespace '%s': %w", resource.Kind, resource.Name, resource.Namespace, err)
		}
		objs, err = r.filterResources(resource.Selector, objs)
		if err != nil {
			return nil, fmt.Errorf("failed to filter resources: %w", err)
		}
		for _, obj := range objs {
			result, err := watcher.GetJSONPath(resource.Path, obj)
			if err != nil {
				log.Info(utils.WarnString("could not get json path, continue"), "objectName", obj.GetName(), "path", resource.Path, "error", err)
				continue
			}
			for _, element := range result {
				var image string
				if versionRegex.MatchString(element) {
					tmp, err := getImageFromVersion(element)
					if err != nil {
						return nil, fmt.Errorf("could not get image from '%s': %w", element, err)
					}
					log.Info("Version from regex", "objectName", obj.GetName(), "element", element)
					image = tmp
				} else if strings.Contains(element, "@") || strings.Contains(element, ":") {
					log.Info("Version from image", "objectName", obj.GetName(), "element", element)
					image = element
				} else {
					return nil, fmt.Errorf("format error, '%s' is not a valid image/version", element)
				}
				info, err := r.getVersionInfoFromImage(ctx, image)
				if err != nil {
					return nil, fmt.Errorf("could not get version info from image '%s': %w", image, err)
				}
				versionMap[info.DTKImage] = info
			}
		}
	}
	return versionMap, nil
}

func (r *SpecialResourceModuleReconciler) updateSpecialResourceModuleStatus(resource srov1beta1.SpecialResourceModule) error {
	if err := r.KubeClient.StatusUpdate(context.Background(), &resource); err != nil {
		return fmt.Errorf("unable to update SpecialResourceModule '%s' status: %w", resource.Name, err)
	}
	return nil
}

func (r *SpecialResourceModuleReconciler) reconcileChart(ctx context.Context, srm *srov1beta1.SpecialResourceModule, meta metadata, reconciledInput []string) ([]string, error) {
	reconciledInputMap := make(map[string]bool)
	for _, element := range reconciledInput {
		reconciledInputMap[element] = true
	}
	result := make([]string, 0)
	c, err := r.Helmer.Load(srm.Spec.Chart)
	if err != nil {
		return []string{}, fmt.Errorf("could not load helm chart '%s': %w", srm.Spec.Chart.Name, err)
	}

	nostate := *c
	nostate.Templates = []*chart.File{}
	stateYAMLS := []*chart.File{}
	for _, template := range c.Templates {
		if r.Assets.ValidStateName(template.Name) {
			if _, ok := reconciledInputMap[template.Name]; !ok {
				stateYAMLS = append(stateYAMLS, template)
			} else {
				result = append(result, template.Name)
			}
		} else {
			nostate.Templates = append(nostate.Templates, template)
		}
	}

	sort.Slice(stateYAMLS, func(i, j int) bool {
		return stateYAMLS[i].Name < stateYAMLS[j].Name
	})

	for _, stateYAML := range stateYAMLS {
		step := nostate
		step.Templates = append(nostate.Templates, stateYAML)

		step.Values, err = chartutil.CoalesceValues(&step, srm.Spec.Set.Object)
		if err != nil {
			return result, fmt.Errorf("failed to coalesce values for chart '%s': %w", step.Name(), err)
		}

		rinfo, err := k8sruntime.DefaultUnstructuredConverter.ToUnstructured(&meta)
		if err != nil {
			return result, fmt.Errorf("failed to convert metadata to an unstructured object: %w", err)
		}
		step.Values, err = chartutil.CoalesceValues(&step, rinfo)
		if err != nil {
			return result, fmt.Errorf("failed to coalesce values for chart '%s': %w", step.Name(), err)
		}
		err = r.Helmer.Run(ctx, step, step.Values,
			srm,
			srm.Name,
			srm.Spec.Namespace,
			nil,
			meta.KernelFullVersion,
			meta.OperatingSystem,
			false,
			SRMOwnedLabel)
		if err != nil {
			return result, fmt.Errorf("failed to run helm chart '%s': %w", step.Name(), err)
		}
		result = append(result, stateYAML.Name)
	}
	return nil, nil
}

// SetupWithManager main initalization for manager
func (r *SpecialResourceModuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	c, err := ctrl.NewControllerManagedBy(mgr).
		Named("specialresourcemodule").
		For(&srov1beta1.SpecialResourceModule{}).
		Owns(&buildv1.BuildConfig{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
			RateLimiter:             workqueue.NewItemExponentialFailureRateLimiter(minDelaySRM, maxDelaySRM),
		}).
		WithEventFilter(r.Filter.GetPredicates()).
		Build(r)
	if err != nil {
		return fmt.Errorf("failed to create new controller for specialresourcemodule resources: %w", err)
	}

	r.Watcher = watcher.New(c)
	return nil
}

// Reconcile Reconiliation entry point
func (r *SpecialResourceModuleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Reconciling")

	srm := &srov1beta1.SpecialResourceModuleList{}

	opts := []client.ListOption{}
	if err := r.KubeClient.List(context.Background(), srm, opts...); err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to list SpecialResourceModules: %w", err)
	}

	var request int
	var found bool
	if request, found = FindSRM(srm.Items, req.Name); !found {
		log.Info("Not found")
		return reconcile.Result{}, nil
	}
	resource := srm.Items[request]

	if resource.GetDeletionTimestamp() != nil {
		log.Info("Deleted resource")
		return reconcile.Result{}, r.deleteNamespace(ctx, resource.Spec.Namespace)
	}

	log.Info("Reconciling watches")
	if err := r.Watcher.ReconcileWatches(ctx, resource); err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to update watched resource: %w", err)
	}

	log.Info("Creating namespace")
	_ = r.createNamespace(ctx, resource)

	clusterVersions, err := r.getOCPVersions(ctx, resource.Spec.Watch)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get OCP versions: %w", err)
	}
	log.Info("Active versions retrieved", "numberOfVersions", len(clusterVersions))

	if resource.Status.Versions == nil {
		resource.Status.Versions = make(map[string]srov1beta1.SpecialResourceModuleVersionStatus)
	}

	updateList := make([]string, 0, len(clusterVersions))
	for key := range clusterVersions {
		updateList = append(updateList, key)
	}
	sort.Strings(updateList)

	log.Info("Checking for obsolete versions in status")
	for key := range resource.Status.Versions {
		if _, ok := clusterVersions[key]; !ok {
			log.Info("Removing version from status", "version", key)
			delete(resource.Status.Versions, key)
		}
	}

	log.Info("Reconciling active versions")
	for _, key := range updateList {
		element := clusterVersions[key]
		log.Info("Reconciling version", "version", element.DTKImage)
		metadata := getMetadata(resource, element)
		var inputList []string
		if data, ok := resource.Status.Versions[element.DTKImage]; ok {
			inputList = data.ReconciledTemplates
		}
		reconciledList, err := r.reconcileChart(ctx, &resource, metadata, inputList)
		resource.Status.Versions[element.DTKImage] = srov1beta1.SpecialResourceModuleVersionStatus{
			ReconciledTemplates: reconciledList,
			Complete:            len(reconciledList) == 0,
		}
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to reconcile chart for SpecialResourceModule '%s': %w", resource.Name, err)
		}
		if err := r.updateSpecialResourceModuleStatus(resource); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to update SpecialResourceModule status: %w", err)
		}

	}

	log.Info("Done")
	return reconcile.Result{}, nil
}
