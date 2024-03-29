package helmer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/openshift/special-resource-operator/pkg/clients"
	helmerv1beta1 "github.com/openshift/special-resource-operator/pkg/helmer/api/v1beta1"
	"github.com/openshift/special-resource-operator/pkg/resource"
	"github.com/openshift/special-resource-operator/pkg/utils"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/releaseutil"
	"helm.sh/helm/v3/pkg/repo"
	helmtime "helm.sh/helm/v3/pkg/time"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"
	ctrlruntime "sigs.k8s.io/controller-runtime"
)

func defaultSettings() (*cli.EnvSettings, error) {
	s := cli.New()

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("failed to obtain a cache directory: %w", err)
	}

	s.RepositoryConfig = filepath.Join(cacheDir, "special-resource-operator/helm/repositories/config.yaml")
	s.RepositoryCache = filepath.Join(cacheDir, "special-resource-operator/helm/cache")
	s.RegistryConfig = filepath.Join(cacheDir, "special-resource-operator/helm/registry.json")
	s.MaxHistory = 10

	return s, nil
}

func OpenShiftInstallOrder() {
	// Mutates helm package exported variables
	idx := utils.StringSliceFind(releaseutil.InstallOrder, "Service")
	releaseutil.InstallOrder = utils.StringSliceInsert(releaseutil.InstallOrder, idx, "BuildConfig")
	releaseutil.InstallOrder = utils.StringSliceInsert(releaseutil.InstallOrder, idx, "ImageStream")
	releaseutil.InstallOrder = utils.StringSliceInsert(releaseutil.InstallOrder, idx, "SecurityContextConstraints")
	releaseutil.InstallOrder = utils.StringSliceInsert(releaseutil.InstallOrder, idx, "Issuer")
	releaseutil.InstallOrder = utils.StringSliceInsert(releaseutil.InstallOrder, idx, "Certificates")
}

//go:generate mockgen -source=helmer.go -package=helmer -destination=mock_helmer_api.go

type Helmer interface {
	Load(helmerv1beta1.HelmChart) (*chart.Chart, error)
	Run(context.Context, chart.Chart, map[string]interface{}, v1.Object, string, string, map[string]string, string, string, bool, string) error
	GetHelmOutput(context.Context, chart.Chart, map[string]interface{}, string, string) (string, error)
}

type helmer struct {
	actionConfig    *action.Configuration
	resourceAPI     resource.ResourceAPI
	getterProviders getter.Providers
	kubeClient      clients.ClientsInterface
	repoFile        *repo.File
	settings        *cli.EnvSettings
	kubeVersion     chartutil.KubeVersion
	apiVersions     chartutil.VersionSet
}

func NewHelmer(resourceAPI resource.ResourceAPI, kubeClient clients.ClientsInterface) (*helmer, error) {
	settings, err := defaultSettings()
	if err != nil {
		return nil, fmt.Errorf("unable to create settings: %w", err)
	}
	dc, err := settings.RESTClientGetter().ToDiscoveryClient()
	if err != nil {
		return nil, fmt.Errorf("unable to get discovery client: %w", err)
	}
	dc.Invalidate()
	version, err := dc.ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve server version: %w", err)
	}
	apiVersions, err := action.GetVersionSet(dc)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve API versions: %w", err)
	}
	var kubeVersion chartutil.KubeVersion
	if version != nil {
		kubeVersion.Version = version.GitVersion
		kubeVersion.Major = version.Major
		kubeVersion.Minor = version.Minor
	}
	return newHelmerWithVersions(resourceAPI, settings, version, apiVersions, kubeClient)
}

func newHelmerWithVersions(resourceAPI resource.ResourceAPI, settings *cli.EnvSettings, version *version.Info, apiVersions chartutil.VersionSet, kubeClient clients.ClientsInterface) (*helmer, error) {
	var kubeVersion chartutil.KubeVersion
	if version != nil {
		kubeVersion.Version = version.GitVersion
		kubeVersion.Major = version.Major
		kubeVersion.Minor = version.Minor
	}
	return &helmer{
		resourceAPI:     resourceAPI,
		getterProviders: getter.All(settings),
		kubeClient:      kubeClient,
		repoFile: &repo.File{
			APIVersion:   "",
			Generated:    time.Time{},
			Repositories: []*repo.Entry{},
		},
		settings:    settings,
		kubeVersion: kubeVersion,
		apiVersions: apiVersions,
	}, nil
}

func init() {
	OpenShiftInstallOrder()
}

func (h *helmer) AddorUpdateRepo(entry *repo.Entry) error {

	chartRepo, err := repo.NewChartRepository(entry, h.getterProviders)
	if err != nil {
		return fmt.Errorf("new chart repository constructor failed for %s: %w", entry.URL, err)

	}
	chartRepo.CachePath = h.settings.RepositoryCache

	if _, err = chartRepo.DownloadIndexFile(); err != nil {
		return fmt.Errorf("cannot find index.yaml for %s: %w", entry.URL, err)
	}

	if h.repoFile.Has(entry.Name) {
		return nil
	}

	h.repoFile.Update(entry)

	if err = h.repoFile.WriteFile(h.settings.RepositoryConfig, 0644); err != nil {
		return fmt.Errorf("could not write repository config %s: %w", h.settings.RepositoryConfig, err)
	}

	return nil
}

func (h *helmer) Load(spec helmerv1beta1.HelmChart) (*chart.Chart, error) {

	entry := &repo.Entry{
		Name:                  spec.Repository.Name,
		URL:                   spec.Repository.URL,
		Username:              spec.Repository.Username,
		Password:              spec.Repository.Password,
		CertFile:              spec.Repository.CertFile,
		KeyFile:               spec.Repository.KeyFile,
		CAFile:                spec.Repository.CAFile,
		InsecureSkipTLSverify: spec.Repository.InsecureSkipTLSverify,
	}

	if err := h.AddorUpdateRepo(entry); err != nil {
		return nil, fmt.Errorf("failed to add or update repo for entry %s: %w", entry.Name, err)
	}

	act := action.ChartPathOptions{
		CaFile:                "",
		CertFile:              "",
		KeyFile:               "",
		InsecureSkipTLSverify: entry.InsecureSkipTLSverify,
		Keyring:               "",
		Password:              "",
		RepoURL:               "",
		Username:              "",
		Verify:                false,
		Version:               spec.Version,
	}
	act.Verify = false

	repoChartName := entry.Name + "/" + spec.Name

	var err error
	var path string

	if path, err = act.LocateChart(repoChartName, h.settings); err != nil {
		return nil, fmt.Errorf("failed to locate chart %s: %w", repoChartName, err)
	}

	loaded, err := loader.Load(path)

	return loaded, err

}

func (h *helmer) failRelease(rel *release.Release, err error) error {
	rel.SetStatus(release.StatusFailed, fmt.Sprintf("Release %q failed: %s", rel.Name, err.Error()))
	if e := h.actionConfig.Releases.Update(rel); e != nil {
		return fmt.Errorf("failed to update release status: %w", e)
	}
	return err
}

func (h *helmer) deleteHookByPolicy(hook *release.Hook, policy release.HookDeletePolicy) error {
	if hook.Kind == "CustomResourceDefinition" {
		return nil
	}
	found := false
	for _, v := range hook.DeletePolicies {
		if policy == v {
			found = true
			break
		}
	}
	if !found {
		return nil
	}
	resources, err := h.actionConfig.KubeClient.Build(bytes.NewBufferString(hook.Manifest), false)
	if err != nil {
		return fmt.Errorf("failed to build kubernetes object for deleteHook %s: %w", hook.Path, err)
	}
	_, errs := h.actionConfig.KubeClient.Delete(resources)
	if len(errs) > 0 {
		es := make([]string, 0, len(errs))
		for _, e := range errs {
			es = append(es, e.Error())
		}
		return fmt.Errorf("failed to delete hook resource %s: %s", hook.Path, strings.Join(es, "; "))
	}
	return nil
}

func (h *helmer) InstallCRDs(ctx context.Context, crds []chart.CRD, owner v1.Object, name, namespace, ownerLabel string) error {

	var manifests bytes.Buffer

	for _, crd := range crds {
		fmt.Fprintf(&manifests, "---\n# Source: %s\n%s\n", crd.Filename, crd.File.Data)
	}
	if err := h.resourceAPI.CreateFromYAML(ctx, manifests.Bytes(),
		false, owner, name, namespace, nil, "", "", ownerLabel); err != nil {
		return fmt.Errorf("failed to create from yaml %s/%s: %w", namespace, name, err)
	}

	return nil
}

func (h *helmer) dryRunHelmChart(ctx context.Context,
	ch chart.Chart,
	vals map[string]interface{},
	name string,
	namespace string) (*release.Release, *action.Install, error) {

	log := ctrlruntime.LoggerFrom(ctx)
	h.actionConfig = new(action.Configuration)

	err := h.actionConfig.Init(h.settings.RESTClientGetter(), namespace, "configmaps", func(format string, v ...interface{}) {
		msg := fmt.Sprintf(format, v...)
		log.Info("Helm", "internal", msg)
	})
	if err != nil {
		return nil, nil, fmt.Errorf("Cannot initialize helm action config for chart %s: %w", ch.Name(), err)
	}

	install := action.NewInstall(h.actionConfig)

	install.DryRun = true
	install.ReleaseName = name
	install.Replace = false
	install.ClientOnly = true
	install.KubeVersion = &h.kubeVersion
	install.APIVersions = h.apiVersions
	install.IncludeCRDs = false
	install.Namespace = namespace
	install.DisableHooks = false
	install.IsUpgrade = false
	install.Timeout = time.Second * 300

	if install.Version == "" {
		install.Version = ">0.0.0-0"
	}

	if ch.Metadata.Type != "" && ch.Metadata.Type != "application" {
		return nil, nil, fmt.Errorf("chart %s has an unsupported type %s and can not be installed", ch.Name(), ch.Metadata.Type)
	}

	rel, err := install.Run(&ch, vals)
	return rel, install, err
}

func (h *helmer) GetHelmOutput(ctx context.Context,
	ch chart.Chart,
	vals map[string]interface{},
	name string,
	namespace string) (string, error) {
	rel, _, err := h.dryRunHelmChart(ctx, ch, vals, name, namespace)
	if err != nil {
		return "", fmt.Errorf("failed to process chart %s: %w", ch.Name(), err)
	}
	return rel.Manifest, nil
}

func (h *helmer) Run(
	ctx context.Context,
	ch chart.Chart,
	vals map[string]interface{},
	owner v1.Object,
	name string,
	namespace string,
	nodeSelector map[string]string,
	kernelFullVersion string,
	operatingSystemMajorMinor string,
	debug bool,
	ownerLabel string) error {

	log := ctrlruntime.LoggerFrom(ctx)

	rel, install, err := h.dryRunHelmChart(ctx, ch, vals, name, namespace)
	if err != nil {
		return fmt.Errorf("failed to process helm chart %s: %w", ch.Name(), err)
	}

	if debug {
		json, err := json.MarshalIndent(vals, "", " ")
		if err != nil {
			return fmt.Errorf("failed to marshal values map during debug for chart %s: %w", ch.Name(), err)
		}
		log.Info("Debug active. Showing manifests", "json", json, "manifest", rel.Manifest)
		for _, hook := range rel.Hooks {
			log.Info("Debug active. Showing hooks", "hookName", hook.Name, "manifest", hook.Manifest)
		}
	}

	// Store the release in history before continuing (new in Helm 3). We always know
	// that this is a create operation.
	if err = h.actionConfig.Releases.Create(rel); err != nil {
		// We could try to recover gracefully here, but since nothing has been installed
		// yet, this is probably safer than trying to continue when we know storage is
		// not working.
		log.Info(utils.WarnString("Failed to create a Helm release"), "error", err)
	}

	// Pre-install anything in the crd/ directory.
	if crds := ch.CRDObjects(); len(crds) > 0 {

		err := h.InstallCRDs(ctx, crds, owner, install.ReleaseName, install.Namespace, ownerLabel)
		if err != nil {
			return fmt.Errorf("failed to install CRDs for chart %s: %w", ch.Name(), err)
		}
	}

	// pre-install hooks
	if !install.DisableHooks {
		if err := h.ExecHook(ctx, rel, release.HookPreInstall, owner, name, namespace, ownerLabel); err != nil {
			return h.failRelease(rel, fmt.Errorf("failed pre-install hook for chart %s: %w", ch.Name(), err))
		}

	}

	err = h.resourceAPI.CreateFromYAML(
		ctx,
		[]byte(rel.Manifest),
		h.ReleaseInstalled(name),
		owner,
		name,
		namespace,
		nodeSelector,
		kernelFullVersion,
		operatingSystemMajorMinor,
		ownerLabel)

	if err != nil {
		return h.failRelease(rel, fmt.Errorf("failed to create resources from yaml for chart %s: %w", ch.Name(), err))
	}

	if !install.DisableHooks {
		if err := h.ExecHook(ctx, rel, release.HookPostInstall, owner, name, namespace, ownerLabel); err != nil {
			return h.failRelease(rel, fmt.Errorf("failed post-install hook for chart %s: %w", ch.Name(), err))
		}
	}

	if len(install.Description) > 0 {
		rel.SetStatus(release.StatusDeployed, install.Description)
	} else {
		rel.SetStatus(release.StatusDeployed, "Install complete")
	}

	if err := h.actionConfig.Releases.Update(rel); err != nil {
		return fmt.Errorf("failed tpo update action config relase for chart %s: %w", ch.Name(), err)
	}

	return nil
}

// hookByWeight is a sorter for hooks
type hookByWeight []*release.Hook

func (x hookByWeight) Len() int      { return len(x) }
func (x hookByWeight) Swap(i, j int) { x[i], x[j] = x[j], x[i] }
func (x hookByWeight) Less(i, j int) bool {
	if x[i].Weight == x[j].Weight {
		return x[i].Name < x[j].Name
	}
	return x[i].Weight < x[j].Weight
}

func (h *helmer) ExecHook(ctx context.Context, rl *release.Release, hook release.HookEvent, owner v1.Object, name, namespace, ownerLabel string) error {

	obj := unstructured.Unstructured{}
	obj.SetKind("ConfigMap")
	obj.SetAPIVersion("v1")
	obj.SetName(string("sh.helm.hooks." + hook))
	obj.SetNamespace(namespace)
	obj.SetOwnerReferences([]metav1.OwnerReference{
		{
			APIVersion: "sro.openshift.io/v1beta1",
			Kind:       "SpecialResource",
			Name:       owner.GetName(),
			UID:        owner.GetUID(),
		},
	})

	found := obj.DeepCopy()

	key := types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}

	if err := h.kubeClient.Get(ctx, key, found); err != nil {

		if apierrors.IsNotFound(err) {
			ctrlruntime.LoggerFrom(ctx).Info("Hook not found", "hookName", string(hook))
		} else {
			return fmt.Errorf("unexpected error getting hook cm %s: %w", hook, err)
		}
	} else {
		return nil
	}

	hooks := []*release.Hook{}

	for _, h := range rl.Hooks {
		for _, e := range h.Events {
			if e == hook {
				hooks = append(hooks, h)
			}
		}
	}

	sort.Stable(hookByWeight(hooks))

	for _, hk := range hooks {

		if hk.DeletePolicies == nil || len(hk.DeletePolicies) == 0 {
			hk.DeletePolicies = []release.HookDeletePolicy{release.HookBeforeHookCreation}
		}

		if err := h.deleteHookByPolicy(hk, release.HookBeforeHookCreation); err != nil {
			return fmt.Errorf("failed to delete hook %s %s by policy before-hook-creation: %w", hk.Name, hk.Path, err)
		}

		hk.LastRun = release.HookExecution{
			StartedAt: helmtime.Now(),
			Phase:     release.HookPhaseRunning,
		}
		if err := h.actionConfig.Releases.Update(rl); err != nil {
			return fmt.Errorf("unable to update release status for hook %s %s: %w", hk.Name, hk.Path, err)
		}

		// As long as the implementation of WatchUntilReady does not panic, HookPhaseFailed or HookPhaseSucceeded
		// should always be set by this function. If we fail to do that for any reason, then HookPhaseUnknown is
		// the most appropriate value to surface.
		hk.LastRun.Phase = release.HookPhaseUnknown

		if err := h.resourceAPI.CreateFromYAML(ctx, []byte(hk.Manifest), false, owner, name, namespace, nil, "", "", ownerLabel); err != nil {

			hk.LastRun.CompletedAt = helmtime.Now()
			hk.LastRun.Phase = release.HookPhaseFailed
			if err := h.deleteHookByPolicy(hk, release.HookFailed); err != nil {
				return fmt.Errorf("failed to delete hook by policy hook-failed %s %s: %w", hk.Name, hk.Path, err)
			}
			return fmt.Errorf("hook execution failed %s %s: %w", hk.Name, hk.Path, err)
		}

		// Watch hook resources until they have completed
		//err = ActionConfig.KubeClient.WatchUntilReady(resources, timeout)
		// Note the time of success/failure
		hk.LastRun.CompletedAt = helmtime.Now()
		hk.LastRun.Phase = release.HookPhaseSucceeded
	}
	// If all hooks are successful, check the annotation of each hook to determine whether the hook should be deleted
	// under succeeded condition. If so, then clear the corresponding resource object in each hook
	for _, hk := range hooks {
		if err := h.deleteHookByPolicy(hk, release.HookSucceeded); err != nil {
			return fmt.Errorf("failed to delete hokk %s %s by policy before-hook-creation: %w", hk.Name, hk.Path, err)
		}
	}

	if len(hooks) > 0 {
		if err := h.kubeClient.Create(ctx, &obj); err != nil {
			if apierrors.IsAlreadyExists(err) {
				return nil
			}
			if apierrors.IsForbidden(err) {
				return fmt.Errorf("failed to create configmap %s/%s for hook %s, forbidden: %w", obj.GetNamespace(), obj.GetName(), hook, err)
			}
			return fmt.Errorf("failed to create configmap %s/%s for hook %s: %w", obj.GetNamespace(), obj.GetName(), hook, err)
		}
	}
	return nil
}

func (h *helmer) ReleaseInstalled(releaseName string) bool {

	hist, err := h.actionConfig.Releases.History(releaseName)
	if err != nil || len(hist) < 1 {
		return false
	}
	releaseutil.Reverse(hist, releaseutil.SortByRevision)
	rel := hist[0]

	if st := rel.Info.Status; st == release.StatusUninstalled || st == release.StatusFailed {
		return false
	}
	return true
}
