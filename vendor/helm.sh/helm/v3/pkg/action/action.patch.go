package action

import (
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
	"k8s.io/cli-runtime/pkg/resource"
)

func (i *Install) InstallCRDs(crds []chart.CRD) error {
	return i.installCRDs(crds)
}

func (i *Install) ReplaceRelease(rel *release.Release) error {
	return i.replaceRelease(rel)
}

func SetMetadataVisitor(releaseName, releaseNamespace string, force bool) resource.VisitorFunc {
	return setMetadataVisitor(releaseName, releaseNamespace, force)
}

func (cfg *Configuration) DeleteHookByPolicy(h *release.Hook, policy release.HookDeletePolicy) error {
	return cfg.deleteHookByPolicy(h, policy)
}

// recordRelease with an update operation in case reuse has been set.
func (c *Configuration) RecordRelease(r *release.Release) {
	c.recordRelease(r)
}
