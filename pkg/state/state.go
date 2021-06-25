package state

import (
	"path"

	"helm.sh/helm/v3/pkg/chart"
)

var CurrentName string

func GenerateName(file *chart.File, sr string) {

	prefix := "specialresource.openshift.io/state-"
	seq := path.Base(file.Name)[:4]

	CurrentName = prefix + sr + "-" + seq
}
