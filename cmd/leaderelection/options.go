package leaderelection

import (
	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/library-go/pkg/config/leaderelection"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func ApplyOpenShiftOptions(opts *ctrl.Options) *ctrl.Options {
	openshiftDefaults := leaderelection.LeaderElectionDefaulting(
		configv1.LeaderElection{},
		"",
		"")

	if opts == nil {
		opts = &manager.Options{}
	}

	opts.LeaderElectionID = "b6ae617b.openshift.io"
	opts.LeaseDuration = &openshiftDefaults.LeaseDuration.Duration
	opts.RetryPeriod = &openshiftDefaults.RetryPeriod.Duration
	opts.RenewDeadline = &openshiftDefaults.RenewDeadline.Duration

	return opts
}
