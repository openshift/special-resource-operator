package cli

import (
	"flag"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/library-go/pkg/config/leaderelection"
)

var openShiftDefaultLeaderElection = leaderelection.LeaderElectionDefaulting(
	configv1.LeaderElection{},
	"",
	"")

type CommandLine struct {
	EnableLeaderElection        bool
	LeaderElectionLeaseDuration time.Duration
	MetricsAddr                 string
}

func ParseCommandLine(programName string, args []string) (*CommandLine, error) {
	cl := CommandLine{}

	fs := flag.NewFlagSet(programName, flag.ContinueOnError)

	fs.StringVar(&cl.MetricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	fs.BoolVar(&cl.EnableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	fs.DurationVar(
		&cl.LeaderElectionLeaseDuration,
		"leader-election-lease-duration",
		openShiftDefaultLeaderElection.LeaseDuration.Duration,
		"The leader election lease duration.")

	return &cl, fs.Parse(args)
}
