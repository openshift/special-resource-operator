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
	"os"
	goruntime "runtime"

	srov1beta1 "github.com/openshift-psap/special-resource-operator/api/v1beta1"
	"github.com/openshift-psap/special-resource-operator/cmd/cli"
	"github.com/openshift-psap/special-resource-operator/cmd/leaderelection"
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

	setupLog.Info("Environment and flags",
		"enable-leader-election", cl.EnableLeaderElection,
		"metrics-addr", cl.MetricsAddr,
		"OPERATOR_NAMESPACE", os.Getenv("OPERATOR_NAMESPACE"),
		"RELEASE_VERSION", os.Getenv("RELEASE_VERSION"),
		"GOARCH", goruntime.GOARCH,
		"GOMAXPROCS", os.Getenv("GOMAXPROCS"),
	)

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	opts := &ctrl.Options{
		MetricsBindAddress: cl.MetricsAddr,
		Port:               9443,
		Scheme:             scheme,
	}

	if cl.EnableLeaderElection {
		opts.LeaderElection = cl.EnableLeaderElection
		opts = leaderelection.ApplyOpenShiftOptions(opts)
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
	runInfo := runtimeAPI.InitRunInfo()

	if err = (&controllers.SpecialResourceReconciler{Cluster: clusterAPI,
		ClusterInfo:            clusterInfoAPI,
		ClusterOperatorManager: state.NewClusterOperatorManager(kubeClient, "special-resource-operator"),
		Creator:                creator,
		PollActions:            pollActions,
		Filter:                 filter.NewFilter(lc, st, kernelAPI),
		Finalizer:              finalizers.NewSpecialResourceFinalizer(kubeClient, pollActions),
		StatusUpdater:          state.NewStatusUpdater(kubeClient),
		Storage:                st,
		Helmer:                 helmer.NewHelmer(creator, helmer.DefaultSettings(), kubeClient),
		Assets:                 assets.NewAssets(),
		KernelData:             kernelAPI,
		Log:                    ctrl.Log,
		Metrics:                metricsClient,
		Scheme:                 scheme,
		ProxyAPI:               proxyAPI,
		RuntimeAPI:             runtimeAPI,
		KubeClient:             kubeClient,
		RunInfo:                runInfo,
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
