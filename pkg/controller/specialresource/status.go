package specialresource

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func updateStatus(obj *unstructured.Unstructured, r *ReconcileSpecialResource, label map[string]string) {

	var current []string

	for k := range label {
		current = append(current, k)
	}

	r.specialresource.Status.State = current[0]

	err := r.client.Status().Update(context.TODO(), &r.specialresource)
	if err != nil {
		log.Error(err, "Failed to update SpecialResource status")
		return
	}
}
