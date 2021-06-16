package helmer

import (
	"bytes"

	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/go-logr/logr"
	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"github.com/openshift-psap/special-resource-operator/pkg/exit"
	"github.com/openshift-psap/special-resource-operator/pkg/slice"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/releaseutil"
	"helm.sh/helm/v3/pkg/repo"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	log      logr.Logger
	settings *cli.EnvSettings
	// http, oci, and patched for file:////
	getterProviders getter.Providers
	storage         = repo.File{
		APIVersion:   "",
		Generated:    time.Time{},
		Repositories: []*repo.Entry{},
	}
)

func init() {
	log = zap.New(zap.UseDevMode(true)).WithName(color.Print("helm", color.Blue))
	err := OpenShiftInstallOrder()

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

	getterProviders = getter.All(settings)

	exit.OnError(err)
}

type HelmRepo struct {
	// +kubebuilder:validation:Required
	Name string `json:"name"`
	// +kubebuilder:validation:Required
	URL string `json:"url"`
	// +kubebuilder:validation:Optional
	Username string `json:"username"`
	// +kubebuilder:validation:Optional
	Password string `json:"password"`
	// +kubebuilder:validation:Optional
	CertFile string `json:"certFile"`
	// +kubebuilder:validation:Optional
	KeyFile string `json:"keyFile"`
	// +kubebuilder:validation:Optional
	CAFile string `json:"caFile"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=false
	InsecureSkipTLSverify bool `json:"insecure_skip_tls_verify"`
}
type HelmChartObosolete struct {
	Name       string   `json:"name"`
	Version    string   `json:"version"`
	Repository HelmRepo `json:"repository"`
}

type HelmChart struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	// +kubebuilder:validation:Required
	Repository HelmRepo `json:"repository"`
	// +kubebuilder:validation:Optional
	Tags []string `json:"tags"`
}

func (in *HelmChart) DeepCopyInto(out *HelmChart) {
	out.Name = in.Name
	out.Version = in.Version
	out.Repository = in.Repository
	out.Tags = make([]string, len(in.Tags))
	copy(out.Tags, in.Tags)
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

	if storage.Has(entry.Name) {
		return nil
	}

	storage.Update(entry)

	if err = storage.WriteFile(settings.RepositoryConfig, 0644); err != nil {
		return errors.Wrap(err, "could not wirte repository config:"+settings.RepositoryConfig)
	}

	return nil
}

func Load(spec HelmChart) (*chart.Chart, error) {

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

	err := AddorUpdateRepo(entry)
	exit.OnError(err)

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

	return nil
}

func TemplateChart(ch chart.Chart, vals map[string]interface{}) ([]byte, error) {

	actionConfig := action.Configuration{}

	client := action.NewInstall(&actionConfig)

	client.DryRun = true
	client.ReleaseName = ch.Metadata.Name
	client.Replace = true
	client.ClientOnly = true
	client.APIVersions = []string{}
	client.IncludeCRDs = true

	if client.Version == "" {
		client.Version = ">0.0.0-0"
	}

	if ch.Metadata.Type != "" && ch.Metadata.Type != "application" {
		return nil, errors.New("Chart has an unsupported type and is not installable:" + ch.Metadata.Type)
	}

	out := new(bytes.Buffer)

	rel, err := client.Run(&ch, vals)

	if rel != nil {
		var manifests bytes.Buffer
		fmt.Fprintln(&manifests, strings.TrimSpace(rel.Manifest))
		if !client.DisableHooks {
			for _, m := range rel.Hooks {
				fmt.Fprintf(&manifests, "---\n# Source: %s\n%s\n", m.Path, m.Manifest)
			}
		}
		fmt.Fprintf(out, "%s", manifests.String())
	}
	return out.Bytes(), err
}
