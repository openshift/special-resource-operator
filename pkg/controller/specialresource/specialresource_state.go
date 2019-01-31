package specialresource

import srov1alpha1 "github.com/zvonkok/special-resource-operator/pkg/apis/sro/v1alpha1"

type state interface {
	init(*ReconcileSpecialResource, *srov1alpha1.SpecialResource)
	step()
	validate()
	last()
}

type SRO struct {
	resources []Resources
	controls  []controlFunc
	rec       *ReconcileSpecialResource
	ins       *srov1alpha1.SpecialResource
	idx       int
}

func addState(n *SRO, path string) error {

	res, ctrl := addResourcesControls(path)

	n.controls = append(n.controls, ctrl)
	n.resources = append(n.resources, res)

	return nil
}

func (n *SRO) init(r *ReconcileSpecialResource,
	i *srov1alpha1.SpecialResource) error {
	n.rec = r
	n.ins = i
	n.idx = 0

	addState(n, "/opt/sro/state-driver")
	addState(n, "/opt/sro/state-driver-validation")
	addState(n, "/opt/sro/state-device-plugin")

	return nil
}

func (n *SRO) step() error {

	for _, fs := range n.controls[n.idx] {

		err := fs(*n)
		if err != nil {
			return err
		}
	}

	n.idx = n.idx + 1

	return nil
}

func (n SRO) validate() {
	// TODO add custom validation functions
}

func (n SRO) last() bool {
	if n.idx == len(n.controls) {
		return true
	}
	return false
}
