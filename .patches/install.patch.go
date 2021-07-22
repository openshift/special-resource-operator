package action

import (
	"time"

	"helm.sh/helm/v3/pkg/release"
)

func (i *Install) FailRelease(rel *release.Release, err error) (*release.Release, error) {
	return i.failRelease(rel, err)
}

func (cfg *Configuration) ExecHook(rl *release.Release, hook release.HookEvent, timeout time.Duration) error {
	return cfg.execHook(rl, hook, timeout)
}

func (i *Install) RecordRelease(r *release.Release) error {
	return i.recordRelease(r)
}
