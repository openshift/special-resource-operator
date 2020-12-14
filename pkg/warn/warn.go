package warn

import (
	"github.com/go-logr/logr"
	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	log logr.Logger
)

func init() {
	log = zap.New(zap.UseDevMode(true)).WithName(color.Print("warning", color.Brown))
}

func OnErrorOrNotFound(found bool, err error) {
	if !found || err != nil {
		log.Info(color.Print("OnErrorOrNotFound: "+err.Error(), color.Brown))
	}
}
