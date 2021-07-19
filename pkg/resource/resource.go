package resource

import (
	"strings"

	"github.com/openshift-psap/special-resource-operator/pkg/exit"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func IsNamespaced(kind string) bool {
	if kind == "Namespace" ||
		kind == "ClusterRole" ||
		kind == "ClusterRoleBinding" ||
		kind == "SecurityContextConstraint" ||
		kind == "SpecialResource" {
		return false
	}
	return true
}

func IsNotUpdateable(kind string) bool {
	// ServiceAccounts cannot be updated, maybe delete and create?
	if kind == "ServiceAccount" || kind == "Pod" {
		return true
	}
	return false
}

// Some resources need an updated resourceversion, during updates
func NeedsResourceVersionUpdate(kind string) bool {
	if kind == "SecurityContextConstraints" ||
		kind == "Service" ||
		kind == "ServiceMonitor" ||
		kind == "Route" ||
		kind == "Build" ||
		kind == "BuildRun" ||
		kind == "BuildConfig" ||
		kind == "ImageStream" ||
		kind == "PrometheusRule" ||
		kind == "CSIDriver" ||
		kind == "Issuer" ||
		kind == "CustomResourceDefinition" ||
		kind == "Certificate" ||
		kind == "SpecialResource" ||
		kind == "OperatorGroup" ||
		kind == "CertManager" ||
		kind == "MutatingWebhookConfiguration" ||
		kind == "ValidatingWebhookConfiguration" ||
		kind == "Deployment" {
		return true
	}
	return false

}

func UpdateResourceVersion(req *unstructured.Unstructured, found *unstructured.Unstructured) error {

	kind := found.GetKind()

	if NeedsResourceVersionUpdate(kind) {
		version, fnd, err := unstructured.NestedString(found.Object, "metadata", "resourceVersion")
		exit.OnErrorOrNotFound(fnd, err)

		if err := unstructured.SetNestedField(req.Object, version, "metadata", "resourceVersion"); err != nil {
			return errors.Wrap(err, "Couldn't update ResourceVersion")
		}

	}
	if kind == "Service" {
		clusterIP, fnd, err := unstructured.NestedString(found.Object, "spec", "clusterIP")
		exit.OnErrorOrNotFound(fnd, err)

		if err := unstructured.SetNestedField(req.Object, clusterIP, "spec", "clusterIP"); err != nil {
			return errors.Wrap(err, "Couldn't update clusterIP")
		}
		return nil
	}
	return nil
}

func SetNodeSelectorTerms(obj *unstructured.Unstructured, terms map[string]string) error {

	if strings.Compare(obj.GetKind(), "DaemonSet") == 0 {
		if err := nodeSelectorTerms(terms, obj, "spec", "template", "spec", "nodeSelector"); err != nil {
			return errors.Wrap(err, "Cannot setup DaemonSet kernel version affinity")
		}
	}
	if strings.Compare(obj.GetKind(), "Pod") == 0 {
		if err := nodeSelectorTerms(terms, obj, "spec", "nodeSelector"); err != nil {
			return errors.Wrap(err, "Cannot setup Pod kernel version affinity")
		}
	}
	if strings.Compare(obj.GetKind(), "BuildConfig") == 0 {
		if err := nodeSelectorTerms(terms, obj, "spec", "nodeSelector"); err != nil {
			return errors.Wrap(err, "Cannot setup BuildConfig kernel version affinity")
		}
	}

	return nil
}

func nodeSelectorTerms(terms map[string]string, obj *unstructured.Unstructured, fields ...string) error {

	nodeSelector, found, err := unstructured.NestedMap(obj.Object, fields...)
	exit.OnError(err)

	if !found {
		nodeSelector = make(map[string]interface{})
	}

	for k, v := range terms {
		nodeSelector[k] = v
	}

	if err := unstructured.SetNestedMap(obj.Object, nodeSelector, fields...); err != nil {
		return errors.Wrap(err, "Cannot update nodeSelector for: "+obj.GetName())
	}

	return nil
}
