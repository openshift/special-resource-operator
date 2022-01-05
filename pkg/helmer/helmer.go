package helmer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/go-logr/logr"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	helmerv1beta1 "github.com/openshift-psap/special-resource-operator/pkg/helmer/api/v1beta1"
	"github.com/openshift-psap/special-resource-operator/pkg/resource"
	"github.com/openshift-psap/special-resource-operator/pkg/utils"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/releaseutil"
	"helm.sh/helm/v3/pkg/repo"
	helmtime "helm.sh/helm/v3/pkg/time"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func DefaultSettings() *cli.EnvSettings {
	s := cli.New()

	s.RepositoryConfig = "/cache/helm/repositories/config.yaml"
	s.RepositoryCache = "/cache/helm/cache"
	s.Debug = true
	s.MaxHistory = 10

	return s
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

type Helmer interface {
	Load(helmerv1beta1.HelmChart) (*chart.Chart, error)
	Run(context.Context, chart.Chart, map[string]interface{}, v1.Object, string, string, map[string]string, string, string, bool) error
}

type helmer struct {
	actionConfig    *action.Configuration
	creator         resource.Creator
	getterProviders getter.Providers
	log             logr.Logger
	repoFile        *repo.File
	settings        *cli.EnvSettings
}

func NewHelmer(creator resource.Creator, settings *cli.EnvSettings) *helmer {
	return &helmer{
		creator:         creator,
		getterProviders: getter.All(settings),
		log:             zap.New(zap.UseDevMode(true)).WithName(utils.Print("helmer", utils.Blue)),
		repoFile: &repo.File{
			APIVersion:   "",
			Generated:    time.Time{},
			Repositories: []*repo.Entry{},
		},
		settings: settings,
	}
}

func init() {
	OpenShiftInstallOrder()
}

func (h *helmer) AddorUpdateRepo(entry *repo.Entry) error {

	chartRepo, err := repo.NewChartRepository(entry, h.getterProviders)
	if err != nil {
		return fmt.Errorf("new chart repository failed: %w", err)

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
		utils.WarnOnError(err)
		return nil, err
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
	h.log.Info("Locating", "chart", repoChartName)

	var err error
	var path string

	if path, err = act.LocateChart(repoChartName, h.settings); err != nil {
		return nil, fmt.Errorf("Could not locate chart %s: %w", repoChartName, err)
	}

	loaded, err := loader.Load(path)

	return loaded, err

}

func (h *helmer) logWrap(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	h.log.Info("Helm", "internal", msg)
}

func (h *helmer) InstallCRDs(ctx context.Context, crds []chart.CRD, owner v1.Object, name string, namespace string) error {

	var manifests bytes.Buffer

	for _, crd := range crds {
		fmt.Fprintf(&manifests, "---\n# Source: %s\n%s\n", crd.Filename, crd.File.Data)
	}
	if err := h.creator.CreateFromYAML(ctx, manifests.Bytes(),
		false, owner, name, namespace, nil, "", ""); err != nil {
		return err
	}

	return nil
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
	debug bool) error {

	h.actionConfig = new(action.Configuration)

	err := h.actionConfig.Init(h.settings.RESTClientGetter(), namespace, "configmaps", h.logWrap)
	if err != nil {
		return fmt.Errorf("Cannot initialize helm action config: %w", err)
	}

	install := action.NewInstall(h.actionConfig)

	install.DryRun = true
	install.ReleaseName = ch.Metadata.Name
	install.Replace = false
	install.ClientOnly = false
	install.APIVersions = []string{}
	install.IncludeCRDs = false
	install.Namespace = namespace
	install.DisableHooks = false
	install.IsUpgrade = false
	install.Timeout = time.Second * 300

	if install.Version == "" {
		install.Version = ">0.0.0-0"
	}

	if ch.Metadata.Type != "" && ch.Metadata.Type != "application" {
		return fmt.Errorf("Chart has an unsupported type %s and can not be installed", ch.Metadata.Type)
	}

	// Pre-install anything in the crd/ directory. We do this before Helm
	// contacts the upstream server and builds the capabilities object.
	if crds := ch.CRDObjects(); !install.ClientOnly && !install.SkipCRDs && len(crds) > 0 {

		h.log.Info("Release CRDs")
		err := h.InstallCRDs(ctx, crds, owner, install.ReleaseName, install.Namespace)
		if err != nil {
			return fmt.Errorf("Cannot install CRDs: %w", err)
		}
	}

	rel, err := install.Run(&ch, vals)
	if err != nil {
		utils.WarnOnError(err)
		return err
	}

	if debug {
		json, err := json.MarshalIndent(vals, "", " ")
		if err != nil {
			return err
		}

		fmt.Printf("--------------------------------------------------------------------------------\n")
		fmt.Printf("\"%s\"\n", json)
		fmt.Printf("--------------------------------------------------------------------------------\n")
		fmt.Printf("\"%s\"\n", rel.Manifest)
		fmt.Printf("--------------------------------------------------------------------------------\n")
		for _, hook := range rel.Hooks {
			fmt.Printf("%s\n", hook.Manifest)
		}
		fmt.Printf("--------------------------------------------------------------------------------\n")
	}

	// Store the release in history before continuing (new in Helm 3). We always know
	// that this is a create operation.
	if err = h.actionConfig.Releases.Create(rel); err != nil {
		// We could try to recover gracefully here, but since nothing has been installed
		// yet, this is probably safer than trying to continue when we know storage is
		// not working.
		utils.WarnOnError(err)
		//return err
	}

	h.log.Info("Release pre-install hooks")
	// pre-install hooks
	if !install.DisableHooks {
		if err := h.ExecHook(ctx, rel, release.HookPreInstall, owner, name, namespace); err != nil {
			_, err := install.FailRelease(rel, fmt.Errorf("failed pre-install: %s", err))
			return err
		}

	}

	h.log.Info("Release manifests")
	err = h.creator.CreateFromYAML(
		ctx,
		[]byte(rel.Manifest),
		h.ReleaseInstalled(name),
		owner,
		name,
		namespace,
		nodeSelector,
		kernelFullVersion,
		operatingSystemMajorMinor)

	if err != nil {
		_, err := install.FailRelease(rel, err)
		utils.WarnOnError(err)
		return err
	}

	h.log.Info("Release post-install hooks")
	if !install.DisableHooks {
		if err := h.ExecHook(ctx, rel, release.HookPostInstall, owner, name, namespace); err != nil {
			_, err := install.FailRelease(rel, fmt.Errorf("failed post-install: %s", err))
			return err
		}
	}

	if len(install.Description) > 0 {
		rel.SetStatus(release.StatusDeployed, install.Description)
	} else {
		rel.SetStatus(release.StatusDeployed, "Install complete")
	}

	if err := install.RecordRelease(rel); err != nil {
		utils.WarnOnError(fmt.Errorf("failed to record the release: %w", err))
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

func (h *helmer) ExecHook(ctx context.Context, rl *release.Release, hook release.HookEvent, owner v1.Object, name string, namespace string) error {

	obj := unstructured.Unstructured{}
	obj.SetKind("ConfigMap")
	obj.SetAPIVersion("v1")
	obj.SetName(string("sh.helm.hooks." + hook))
	obj.SetNamespace(namespace)

	found := obj.DeepCopy()

	key := types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}

	if err := clients.Interface.Get(ctx, key, found); err != nil {

		if apierrors.IsNotFound(err) {
			h.log.Info("Hooks", string(hook), "NotReady (IsNotFound)")
		} else {
			return fmt.Errorf("Unexpected error getting hook cm %s: %w", hook, err)
		}
	} else {
		h.log.Info("Hooks", string(hook), "Ready (Get)")
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

	// hooke are pre-ordered by kind, so keep order stable
	sort.Stable(hookByWeight(hooks))

	for _, hk := range hooks {

		if hk.DeletePolicies == nil || len(hk.DeletePolicies) == 0 {
			hk.DeletePolicies = []release.HookDeletePolicy{release.HookBeforeHookCreation}
		}

		if err := h.actionConfig.DeleteHookByPolicy(hk, release.HookBeforeHookCreation); err != nil {
			return err
		}

		hk.LastRun = release.HookExecution{
			StartedAt: helmtime.Now(),
			Phase:     release.HookPhaseRunning,
		}
		h.actionConfig.RecordRelease(rl)

		// As long as the implementation of WatchUntilReady does not panic, HookPhaseFailed or HookPhaseSucceeded
		// should always be set by this function. If we fail to do that for any reason, then HookPhaseUnknown is
		// the most appropriate value to surface.
		hk.LastRun.Phase = release.HookPhaseUnknown

		if err := h.creator.CreateFromYAML(ctx, []byte(hk.Manifest), false, owner, name, namespace, nil, "", ""); err != nil {

			hk.LastRun.CompletedAt = helmtime.Now()
			hk.LastRun.Phase = release.HookPhaseFailed
			if err := h.actionConfig.DeleteHookByPolicy(hk, release.HookFailed); err != nil {
				return fmt.Errorf("failed to delete hook by policy %s %s: %w", hk.Name, hk.Path, err)
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
		if err := h.actionConfig.DeleteHookByPolicy(hk, release.HookSucceeded); err != nil {
			return err
		}
	}

	if err := clients.Interface.Create(ctx, &obj); err != nil {
		h.log.Info(err.Error())

		if apierrors.IsAlreadyExists(err) {
			h.log.Info("Hooks", string(hook), "Ready (IsAlreadyExists)")
			return nil
		}

		if apierrors.IsForbidden(err) {
			return fmt.Errorf("API error is forbidden: %w", err)
		}
		return fmt.Errorf("Unexpected error creating hook cm %s: %w", hook, err)
	}
	h.log.Info("Hooks", string(hook), "Ready (Created)")
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
