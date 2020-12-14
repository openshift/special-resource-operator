package exit

import (
	"os"

	"github.com/go-logr/logr"
	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	log logr.Logger
)

func init() {
	log = zap.New(zap.UseDevMode(true)).WithName(color.Print("exit", color.Red))
}

// OnErrorOrNotFound Exit if something is not found or error occured
func OnErrorOrNotFound(found bool, err error) {
	if !found || err != nil {
		log.Info(color.Print("OnErrorOrNotFound: "+err.Error(), color.Red))
		os.Exit(1)
	}
}

// OnError exit on error
func OnError(err error) {
	if err != nil {
		log.Info(color.Print("OnError: "+err.Error(), color.Red))
		os.Exit(1)
	}
}
