package cli

import (
	"flag"
)

type CommandLine struct {
	EnableLeaderElection bool
	MetricsAddr          string
}

func ParseCommandLine(programName string, args []string) (*CommandLine, error) {
	cl := CommandLine{}

	fs := flag.NewFlagSet(programName, flag.ContinueOnError)

	fs.StringVar(&cl.MetricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	fs.BoolVar(&cl.EnableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

	return &cl, fs.Parse(args)
}
