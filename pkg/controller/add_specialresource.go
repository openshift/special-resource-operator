package controller

import (
	"github.com/openshift-psap/special-resource-operator/pkg/controller/specialresource"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, specialresource.Add)
}
