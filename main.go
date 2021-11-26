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

	srov1beta1 "github.com/openshift-psap/special-resource-operator/api/v1beta1"
	"github.com/openshift-psap/special-resource-operator/cmd/cli"
	"github.com/openshift-psap/special-resource-operator/cmd/leaderelection"
	"github.com/openshift-psap/special-resource-operator/controllers"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/metrics"
	"github.com/openshift-psap/special-resource-operator/pkg/resource"
	sroscheme "github.com/openshift-psap/special-resource-operator/pkg/scheme"
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

func main() {
	cl, err := cli.ParseCommandLine(os.Args[0], os.Args[1:])
	if err != nil {
		setupLog.Error(err, "could not parse command-line arguments")
		os.Exit(1)
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
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	clients.RestConfig = mgr.GetConfig()

	kubeClientSet, err := clients.GetKubeClientSet()
	if err != nil {
		setupLog.Error(err, "unable to create client set")
		os.Exit(1)
	}
	configClient, err := clients.GetConfigClient()
	if err != nil {
		setupLog.Error(err, "unable to create config client")
		os.Exit(1)
	}
	cachedDiscoveryClient, err := clients.GetCachedDiscoveryClient()
	if err != nil {
		setupLog.Error(err, "unable to create cached discovery client")
		os.Exit(1)
	}

	clients.Interface = &clients.ClientsInterface{
		Client:                   mgr.GetClient(),
		Clientset:                *kubeClientSet,
		ConfigV1Client:           *configClient,
		CachedDiscoveryInterface: cachedDiscoveryClient,
		EventRecorder:            mgr.GetEventRecorderFor("specialresource"),
	}

	resource.RuntimeScheme = mgr.GetScheme()

	if err = (&controllers.SpecialResourceReconciler{
		Log:     ctrl.Log,
		Scheme:  mgr.GetScheme(),
		Metrics: metrics.New(),
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
