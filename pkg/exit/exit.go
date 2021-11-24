package exit

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var log = zap.New(zap.UseDevMode(true)).WithName(color.Print("exit", color.Red))

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
		// notice that we're using 1, so it will actually log where
		// the error happened, 0 = this function, we don't want that.
		pc, fn, line, _ := runtime.Caller(1)
		function := filepath.Base(runtime.FuncForPC(pc).Name())
		file := filepath.Base(fn)

		msg := fmt.Sprintf("OnError: %s[%s:%d] %v", function, file, line, err)
		log.Info(color.Print(msg, color.Red))
		os.Exit(1)
	}
}
