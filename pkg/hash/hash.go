package hash

import (
	"fmt"
	"hash/fnv"

	"github.com/openshift-psap/special-resource-operator/pkg/exit"
	errs "github.com/pkg/errors"
)

// FNV64a return 64bit hash
func FNV64a(s string) string {
	h := fnv.New64a()
	if _, err := h.Write([]byte(s)); err != nil {
		exit.OnError(errs.Wrap(err, "Could not write hash"))
	}
	return fmt.Sprintf("%x", h.Sum64())
}
