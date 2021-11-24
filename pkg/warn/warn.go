package warn

import (
	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var log = zap.New(zap.UseDevMode(true)).WithName(color.Print("warning", color.Brown))

// OnErrorOrNotFound warn on error or not found
func OnErrorOrNotFound(found bool, err error) {
	if !found || err != nil {
		log.Info(color.Print("OnErrorOrNotFound: "+err.Error(), color.Brown))
	}
}

// OnError warn on error
func OnError(err error) {
	if err != nil {
		log.Info(color.Print("OnError: "+err.Error(), color.Brown))
	}
}
