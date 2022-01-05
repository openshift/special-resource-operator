package utils

import (
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var log = zap.New(zap.UseDevMode(true)).WithName(Print("warning", Brown))

// OnErrorOrNotFound warn on error or not found
func WarnOnErrorOrNotFound(found bool, err error) {
	if !found || err != nil {
		log.Info(Print("OnErrorOrNotFound: "+err.Error(), Brown))
	}
}

// OnError warn on error
func WarnOnError(err error) {
	if err != nil {
		log.Info(Print("OnError: "+err.Error(), Brown))
	}
}
