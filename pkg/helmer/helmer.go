package helmer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/pkg/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/go-logr/logr"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"github.com/openshift-psap/special-resource-operator/pkg/exit"
	helmerv1beta1 "github.com/openshift-psap/special-resource-operator/pkg/helmer/api/v1beta1"
	"github.com/openshift-psap/special-resource-operator/pkg/resource"
	"github.com/openshift-psap/special-resource-operator/pkg/slice"
	"github.com/openshift-psap/special-resource-operator/pkg/warn"
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
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	log      logr.Logger
	settings *cli.EnvSettings
	// http, oci, and patched for file:////
	getterProviders getter.Providers
	repoFile        = repo.File{
		APIVersion:   "",
		Generated:    time.Time{},
		Repositories: []*repo.Entry{},
	}

	ActionConfig *action.Configuration
)

func init() {
	log = zap.New(zap.UseDevMode(true)).WithName(color.Print("helmer", color.Blue))

	err := OpenShiftInstallOrder()
	exit.OnError(err)

	settings = cli.New()

	// cli.EnvSettings{
	//	KubeConfig:       "",
	//	KubeContext:      "",
	//	KubeToken:        "",
	//	KubeAsUser:       "",
	//	KubeAsGroups:     []string{},
	//	KubeAPIServer:    "",
	//	KubeCaFile:       "",
	//	Debug:            false,
	//	RegistryConfig:   "",
	//	RepositoryConfig: "",
	//	RepositoryCache:  "",
	//	PluginsDirectory: "",
	//	MaxHistory:       0,
	// }

	settings.RepositoryConfig = "/cache/helm/repositories/config.yaml"
	settings.RepositoryCache = "/cache/helm/cache"
	settings.Debug = true
	settings.MaxHistory = 10

	getterProviders = getter.All(settings)

}

func AddorUpdateRepo(entry *repo.Entry) error {

	chartRepo, err := repo.NewChartRepository(entry, getterProviders)
	if err != nil {
		return errors.Wrap(err, "new chart repository failed")

	}
	chartRepo.CachePath = settings.RepositoryCache

	if _, err = chartRepo.DownloadIndexFile(); err != nil {
		return errors.Wrap(err, "cannot find index.yaml for: "+entry.URL)
	}

	if repoFile.Has(entry.Name) {
		return nil
	}

	repoFile.Update(entry)

	if err = repoFile.WriteFile(settings.RepositoryConfig, 0644); err != nil {
		return errors.Wrap(err, "could not wirte repository config:"+settings.RepositoryConfig)
	}

	return nil
}

func Load(spec helmerv1beta1.HelmChart) (*chart.Chart, error) {

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

	if err := AddorUpdateRepo(entry); err != nil {
		warn.OnError(err)
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
		Version:               "",
	}
	act.Verify = false

	repoChartName := entry.Name + "/" + spec.Name
	log.Info("Locating", "chart", repoChartName)

	var err error
	var path string

	if path, err = act.LocateChart(repoChartName, settings); err != nil {
		return nil, errors.Wrap(err, "Could not locate chart: "+repoChartName)
	}

	loaded, err := loader.Load(path)

	return loaded, err

}

func OpenShiftInstallOrder() error {

	idx := slice.Find(releaseutil.InstallOrder, "Service")
	releaseutil.InstallOrder = slice.Insert(releaseutil.InstallOrder, idx, "BuildConfig")
	releaseutil.InstallOrder = slice.Insert(releaseutil.InstallOrder, idx, "ImageStream")
	releaseutil.InstallOrder = slice.Insert(releaseutil.InstallOrder, idx, "SecurityContextConstraints")
	releaseutil.InstallOrder = slice.Insert(releaseutil.InstallOrder, idx, "Issuer")
	releaseutil.InstallOrder = slice.Insert(releaseutil.InstallOrder, idx, "Certificates")

	return nil
}

func LogWrap(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	log.Info("Helm", "internal", msg)
}

func InstallCRDs(crds []chart.CRD, owner v1.Object, name string, namespace string) error {

	var manifests bytes.Buffer

	for _, crd := range crds {
		fmt.Fprintf(&manifests, "---\n# Source: %s\n%s\n", crd.Filename, crd.File.Data)
	}
	if err := resource.CreateFromYAML([]byte(manifests.Bytes()),
		false, owner, name, namespace, nil, "", ""); err != nil {
		return err
	}

	return nil
}

func Run(ch chart.Chart, vals map[string]interface{},
	owner v1.Object,
	name string,
	namespace string,
	nodeSelector map[string]string,
	kernelFullVersion string,
	operatingSystemMajorMinor string,
	debug bool) error {

	ActionConfig = new(action.Configuration)

	err := ActionConfig.Init(settings.RESTClientGetter(), namespace, "configmaps", LogWrap)
	exit.OnError(errors.Wrap(err, "Cannot initialize helm action config"))

	resource.HelmClient = ActionConfig.KubeClient

	install := action.NewInstall(ActionConfig)

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
		return errors.New("Chart has an unsupported type and is not installable:" + ch.Metadata.Type)
	}

	// Pre-install anything in the crd/ directory. We do this before Helm
	// contacts the upstream server and builds the capabilities object.
	if crds := ch.CRDObjects(); !install.ClientOnly && !install.SkipCRDs && len(crds) > 0 {

		log.Info("Release CRDs")
		err := InstallCRDs(crds, owner, install.ReleaseName, install.Namespace)
		exit.OnError(errors.Wrap(err, "Cannot install CRDs"))
	}

	rel, err := install.Run(&ch, vals)
	if err != nil {
		warn.OnError(err)
		return err
	}

	if debug {
		json, err := json.MarshalIndent(vals, "", " ")
		exit.OnError(err)

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
	// If Replace is true, we need to supercede the last release.
	if install.Replace {
		if err := install.ReplaceRelease(rel); err != nil {
			return err
		}
	}

	// Store the release in history before continuing (new in Helm 3). We always know
	// that this is a create operation.
	if err := ActionConfig.Releases.Create(rel); err != nil {
		// We could try to recover gracefully here, but since nothing has been installed
		// yet, this is probably safer than trying to continue when we know storage is
		// not working.
		warn.OnError(err)
		//return err
	}

	log.Info("Release pre-install hooks")
	// pre-install hooks
	if !install.DisableHooks {
		if err := ExecHook(rel, release.HookPreInstall, install.Timeout, owner, name, namespace); err != nil {
			_, err := install.FailRelease(rel, fmt.Errorf("failed pre-install: %s", err))
			return err
		}

	}

	log.Info("Release manifests")
	err = resource.CreateFromYAML([]byte(rel.Manifest),
		ReleaseInstalled(name),
		owner,
		name,
		namespace,
		nodeSelector,
		kernelFullVersion,
		operatingSystemMajorMinor)

	if err != nil {
		_, err := install.FailRelease(rel, err)
		warn.OnError(err)
		return err
	}

	log.Info("Release post-install hooks")
	if !install.DisableHooks {
		if err := ExecHook(rel, release.HookPostInstall, install.Timeout, owner, name, namespace); err != nil {
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
		warn.OnError(errors.Wrap(err, "failed to record the release"))
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

func ExecHook(rl *release.Release, hook release.HookEvent, timeout time.Duration, owner v1.Object, name string, namespace string) error {

	obj := unstructured.Unstructured{}
	obj.SetKind("ConfigMap")
	obj.SetAPIVersion("v1")
	obj.SetName(string("sh.helm.hooks." + hook))
	obj.SetNamespace(namespace)

	found := obj.DeepCopy()

	key := types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}

	if err := clients.Interface.Get(context.TODO(), key, found); err != nil {

		if apierrors.IsNotFound(err) {
			log.Info("Hooks", string(hook), "NotReady (IsNotFound)")
		} else {
			return errors.Wrapf(err, "Unexpected error getting hook cm %s", hook)
		}
	} else {
		log.Info("Hooks", string(hook), "Ready (Get)")
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

	for _, h := range hooks {

		if h.DeletePolicies == nil || len(h.DeletePolicies) == 0 {
			h.DeletePolicies = []release.HookDeletePolicy{release.HookBeforeHookCreation}
		}

		if err := ActionConfig.DeleteHookByPolicy(h, release.HookBeforeHookCreation); err != nil {
			return err
		}

		h.LastRun = release.HookExecution{
			StartedAt: helmtime.Now(),
			Phase:     release.HookPhaseRunning,
		}
		ActionConfig.RecordRelease(rl)

		// As long as the implementation of WatchUntilReady does not panic, HookPhaseFailed or HookPhaseSucceeded
		// should always be set by this function. If we fail to do that for any reason, then HookPhaseUnknown is
		// the most appropriate value to surface.
		h.LastRun.Phase = release.HookPhaseUnknown

		if err := resource.CreateFromYAML([]byte(h.Manifest), false, owner, name, namespace, nil, "", ""); err != nil {

			h.LastRun.CompletedAt = helmtime.Now()
			h.LastRun.Phase = release.HookPhaseFailed
			if err := ActionConfig.DeleteHookByPolicy(h, release.HookFailed); err != nil {
				return errors.Wrapf(err, "failed to delete hook by policy %s %s", h.Name, h.Path)
			}
			return errors.Wrapf(err, "hook execution failed %s %s", h.Name, h.Path)
		}

		// Watch hook resources until they have completed
		//err = ActionConfig.KubeClient.WatchUntilReady(resources, timeout)
		// Note the time of success/failure
		h.LastRun.CompletedAt = helmtime.Now()
		h.LastRun.Phase = release.HookPhaseSucceeded
	}
	// If all hooks are successful, check the annotation of each hook to determine whether the hook should be deleted
	// under succeeded condition. If so, then clear the corresponding resource object in each hook
	for _, h := range hooks {
		if err := ActionConfig.DeleteHookByPolicy(h, release.HookSucceeded); err != nil {
			return err
		}
	}

	if err := clients.Interface.Create(context.TODO(), &obj); err != nil {
		log.Info(err.Error())

		if apierrors.IsAlreadyExists(err) {
			log.Info("Hooks", string(hook), "Ready (IsAlreadyExists)")
			return nil
		}

		if apierrors.IsForbidden(err) {
			return errors.Wrap(err, "API error is forbidden")
		}
		return errors.Wrapf(err, "Unexpected error creating hook cm %s", hook)
	}
	log.Info("Hooks", string(hook), "Ready (Created)")
	return nil
}

func ReleaseInstalled(releaseName string) bool {

	h, err := ActionConfig.Releases.History(releaseName)
	if err != nil || len(h) < 1 {
		return false
	}
	releaseutil.Reverse(h, releaseutil.SortByRevision)
	rel := h[0]

	if st := rel.Info.Status; st == release.StatusUninstalled || st == release.StatusFailed {
		return false
	}
	return true
}
