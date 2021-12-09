#!/bin/bash

cp .patches/options.patch.go vendor/github.com/google/go-containerregistry/pkg/crane/.
cp .patches/getter.patch.go vendor/helm.sh/helm/v3/pkg/getter/.
cp .patches/action.patch.go vendor/helm.sh/helm/v3/pkg/action/.
cp .patches/install.patch.go vendor/helm.sh/helm/v3/pkg/action/.
patch -p1 -r /dev/null -N -i .patches/helm.patch
err=$?
# patch is returning errorCode=1 in case the patch is already applied.
# we will treat every other errorCode as an error
if (( $err > 1 )); then
    exit $err
fi
