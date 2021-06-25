package hash

import (
	"fmt"
	"hash/fnv"
	"strconv"

	"github.com/mitchellh/hashstructure/v2"
	"github.com/openshift-psap/special-resource-operator/pkg/exit"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// FNV64a return 64bit hash
func FNV64a(s string) string {
	h := fnv.New64a()
	if _, err := h.Write([]byte(s)); err != nil {
		exit.OnError(errors.Wrap(err, "Could not write hash"))
	}
	return fmt.Sprintf("%x", h.Sum64())
}

func Annotate(obj *unstructured.Unstructured) {

	hash, err := hashstructure.Hash(obj.Object, hashstructure.FormatV2, nil)
	exit.OnError(err)
	anno := obj.GetAnnotations()
	if anno == nil {
		anno = make(map[string]string)
	}
	anno["specialresource.openshift.io/hash"] = strconv.FormatUint(hash, 10)
	obj.SetAnnotations(anno)

}

func AnnotationEqual(new *unstructured.Unstructured, old *unstructured.Unstructured) bool {

	hash, err := hashstructure.Hash(old.Object, hashstructure.FormatV2, nil)
	exit.OnError(err)
	anno := new.GetAnnotations()

	return anno["specialresource.openshift.io/hash"] == strconv.FormatUint(hash, 10)
}
