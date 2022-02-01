package utils

import (
	"fmt"
	"hash/fnv"
	"strconv"

	"github.com/mitchellh/hashstructure/v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// FNV64a return 64bit hash
func FNV64a(s string) (string, error) {
	h := fnv.New64a()
	if _, err := h.Write([]byte(s)); err != nil {
		return "", fmt.Errorf("Could not write hash: %w", err)
	}
	return fmt.Sprintf("%x", h.Sum64()), nil
}

func Annotate(obj *unstructured.Unstructured) error {

	hash, err := hashstructure.Hash(obj.Object, hashstructure.FormatV2, nil)
	if err != nil {
		return err
	}
	anno := obj.GetAnnotations()
	if anno == nil {
		anno = make(map[string]string)
	}
	anno["specialresource.openshift.io/hash"] = strconv.FormatUint(hash, 10)
	obj.SetAnnotations(anno)
	return nil
}

func AnnotationEqual(new *unstructured.Unstructured, old *unstructured.Unstructured) (bool, error) {

	hash, err := hashstructure.Hash(old.Object, hashstructure.FormatV2, nil)
	if err != nil {
		return false, err
	}
	anno := new.GetAnnotations()

	return anno["specialresource.openshift.io/hash"] == strconv.FormatUint(hash, 10), nil
}
