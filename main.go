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
	"fmt"
	"os"

	srov1beta1 "github.com/openshift-psap/special-resource-operator/api/v1beta1"
	"github.com/openshift-psap/special-resource-operator/cmd/cli"
	"github.com/openshift-psap/special-resource-operator/cmd/leaderelection"
	"github.com/openshift-psap/special-resource-operator/controllers"
	"github.com/openshift-psap/special-resource-operator/internal/controllers/finalizers"
	"github.com/openshift-psap/special-resource-operator/internal/controllers/state"
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
	sroscheme "github.com/openshift-psap/special-resource-operator/pkg/scheme"
	"github.com/openshift-psap/special-resource-operator/pkg/storage"
	"github.com/openshift-psap/special-resource-operator/pkg/upgrade"
	"github.com/openshift-psap/special-resource-operator/pkg/utils"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {

	utilruntime.Must(sroscheme.AddToScheme(scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(srov1beta1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

// run is the main entrypoint of the application.
// This needs to be separate from main because it uses defer, which is ignored by os.Exit.
func run() (err error) {
	cl, err := cli.ParseCommandLine(os.Args[0], os.Args[1:])
	if err != nil {
		return fmt.Errorf("could not parse command-line arguments: %v", err)
	}

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
		return fmt.Errorf("unable to create a new manager: %v", err)
	}

	kubeClient, err := clients.NewClients(mgr.GetClient(), mgr.GetConfig(), mgr.GetEventRecorderFor("specialresource"))
	if err != nil {
		return fmt.Errorf("unable to create k8s clients: %v", err)
	}

	clusterCluster := cluster.NewCluster(kubeClient)
	metricsClient := metrics.New()
	st := storage.NewStorage(kubeClient)
	lc := lifecycle.New(kubeClient, st)
	pollActions := poll.New(kubeClient, lc, st)
	kernelData := kernel.NewKernelData()
	proxyAPI := proxy.NewProxyAPI(kubeClient)

	creator := resource.NewCreator(
		kubeClient,
		metricsClient,
		pollActions,
		kernelData,
		scheme,
		lc,
		proxyAPI)

	com := state.NewClusterOperatorManager(kubeClient, "openshift-special-resource-operator")

	ctx := ctrl.SetupSignalHandler()

	defer func() {
		if comErr := com.Refresh(ctx, utils.EndConditions(err)); comErr != nil {
			err = fmt.Errorf("could not set the final ClusterOperator status: %v, base error: %v", comErr, err)
		}
	}()

	if err = (&controllers.SpecialResourceReconciler{Cluster: clusterCluster,
		ClusterInfo:            upgrade.NewClusterInfo(registry.NewRegistry(kubeClient), clusterCluster),
		ClusterOperatorManager: com,
		Creator:                creator,
		PollActions:            pollActions,
		Filter:                 filter.NewFilter(lc, st, kernelData),
		Finalizer:              finalizers.NewSpecialResourceFinalizer(kubeClient, pollActions),
		StatusUpdater:          state.NewStatusUpdater(kubeClient),
		Storage:                st,
		Helmer:                 helmer.NewHelmer(creator, helmer.DefaultSettings(), kubeClient),
		Assets:                 assets.NewAssets(),
		KernelData:             kernelData,
		Log:                    ctrl.Log,
		Metrics:                metricsClient,
		Scheme:                 scheme,
		ProxyAPI:               proxyAPI,
		KubeClient:             kubeClient,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("could not create the controller: %v", err)
	}
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")

	if err = com.Refresh(ctx, utils.AvailableNotProgressingNotDegraded()); err != nil {
		return fmt.Errorf("could not set the initial ClusterOperator status: %v", err)
	}

	if err = mgr.Start(ctx); err != nil {
		return fmt.Errorf("problem running the manager: %v", err)
	}

	return nil
}

func main() {
	if err := run(); err != nil {
		setupLog.Error(err, "Error running the manager")
		os.Exit(1)
	}
}
