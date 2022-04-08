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

package main

import (
	"errors"
	"os"
	"runtime/debug"
	"strings"

	srov1beta1 "github.com/openshift-psap/special-resource-operator/api/v1beta1"
	"github.com/openshift-psap/special-resource-operator/cmd/cli"
	"github.com/openshift-psap/special-resource-operator/controllers"
	"github.com/openshift-psap/special-resource-operator/internal/controllers/finalizers"
	"github.com/openshift-psap/special-resource-operator/internal/controllers/state"
	"github.com/openshift-psap/special-resource-operator/internal/resourcehelper"
	"github.com/openshift-psap/special-resource-operator/pkg/assets"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/cluster"
	"github.com/openshift-psap/special-resource-operator/pkg/filter"
	"github.com/openshift-psap/special-resource-operator/pkg/helmer"
	"github.com/openshift-psap/special-resource-operator/pkg/kernel"
	"github.com/openshift-psap/special-resource-operator/pkg/lifecycle"
	"github.com/openshift-psap/special-resource-operator/pkg/metrics"
	"github.com/openshift-psap/special-resource-operator/pkg/poll"
	"github.com/openshift-psap/special-resource-operator/pkg/proxy"
	"github.com/openshift-psap/special-resource-operator/pkg/registry"
	"github.com/openshift-psap/special-resource-operator/pkg/resource"
	"github.com/openshift-psap/special-resource-operator/pkg/runtime"
	sroscheme "github.com/openshift-psap/special-resource-operator/pkg/scheme"
	"github.com/openshift-psap/special-resource-operator/pkg/storage"
	"github.com/openshift-psap/special-resource-operator/pkg/upgrade"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = k8sruntime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {

	utilruntime.Must(sroscheme.AddToScheme(scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(srov1beta1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	cl, err := cli.ParseCommandLine(os.Args[0], os.Args[1:])
	if err != nil {
		setupLog.Error(err, "could not parse command-line arguments")
		os.Exit(1)
	}

	helmSettings, err := helmer.DefaultSettings()
	if err != nil {
		setupLog.Error(err, "failed to create Helm settings")
		os.Exit(1)
	}

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	vcsData, err := vcsBuildSettingsToLogArgs()
	if err != nil {
		setupLog.Error(err, "Could not get VCS settings")
	} else {
		setupLog.Info("VCS build settings", vcsData...)
	}

	opts := &ctrl.Options{
		LeaderElection:     cl.EnableLeaderElection,
		LeaderElectionID:   "sro.sigs.k8s.io",
		MetricsBindAddress: cl.MetricsAddr,
		Port:               9443,
		Scheme:             scheme,
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), *opts)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	kubeClient, err := clients.NewClients(mgr.GetClient(), mgr.GetConfig(), mgr.GetEventRecorderFor("specialresource"))
	if err != nil {
		setupLog.Error(err, "unable to create k8s clients")
		os.Exit(1)
	}
	clusterAPI := cluster.NewCluster(kubeClient)

	metricsClient := metrics.New()

	st := storage.NewStorage(kubeClient)
	lc := lifecycle.New(kubeClient, st)
	pollActions := poll.New(kubeClient, lc, st)
	kernelAPI := kernel.NewKernelData()
	proxyAPI := proxy.NewProxyAPI(kubeClient)

	creator := resource.NewCreator(
		kubeClient,
		metricsClient,
		pollActions,
		kernelAPI,
		scheme,
		lc,
		proxyAPI,
		resourcehelper.New())

	clusterInfoAPI := upgrade.NewClusterInfo(registry.NewRegistry(kubeClient), clusterAPI)
	runtimeAPI := runtime.NewRuntimeAPI(kubeClient, clusterAPI, kernelAPI, clusterInfoAPI, proxyAPI)

	if err = (&controllers.SpecialResourceReconciler{
		Cluster:       clusterAPI,
		ClusterInfo:   clusterInfoAPI,
		Creator:       creator,
		PollActions:   pollActions,
		Filter:        filter.NewFilter(lc, st, kernelAPI),
		Finalizer:     finalizers.NewSpecialResourceFinalizer(kubeClient, pollActions),
		StatusUpdater: state.NewStatusUpdater(kubeClient),
		Storage:       st,
		Helmer:        helmer.NewHelmer(creator, helmSettings, kubeClient),
		Assets:        assets.NewAssets(),
		KernelData:    kernelAPI,
		Log:           ctrl.Log,
		Metrics:       metricsClient,
		Scheme:        scheme,
		ProxyAPI:      proxyAPI,
		RuntimeAPI:    runtimeAPI,
		KubeClient:    kubeClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SpecialResource")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func vcsBuildSettingsToLogArgs() ([]any, error) {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return nil, errors.New("could not read build info")
	}

	ret := make([]any, 0)

	for _, s := range bi.Settings {
		if strings.HasPrefix(s.Key, "vcs") {
			ret = append(ret, s.Key, s.Value)
		}
	}

	if len(ret) == 0 {
		return ret, errors.New("build data contains no VCS settings")
	}

	return ret, nil
}
