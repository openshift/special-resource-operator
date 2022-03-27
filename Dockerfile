# Build the manager binary
FROM registry.ci.openshift.org/ocp/builder:rhel-8-golang-1.17-openshift-4.11 as builder

# This Dockerfile is sometimes built in environments (ART) where there's no
# Internet connectivity. By messing up Internet connectivity across the board
# for everyone, we make sure that developers won't make a mistake of making
# this Dockerfile build dependent on Internet connectivity. This current
# implementation of the connectivity blocking is not airtight, it only affects
# HTTP traffic and only affects programs which actually respect these proxy
# environment variables, but it was chosen because it's simple and alternatives
# are hard to come up with.
#
# The goal is only to block Internet connectivity for the build process itself
# - not the actual resulting image. This is why at the end of this Dockerfile
# we re-set these environment variables to an empty string.
#
# If you add more stages (multi-stage) to this Dockerfile, make sure they also
# have these environment variables set at their beginning. Unsetting the
# environement variables should only be done for the latest stage, as its the
# only one that gets ran by users.
ENV http_proxy http://127.0.0.1:80
ENV https_proxy http://127.0.0.1:80
ENV HTTP_PROXY http://127.0.0.1:80
ENV HTTPS_PROXY http://127.0.0.1:80

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

COPY hack/ hack/
COPY helm-plugins/ helm-plugins/
COPY Makefile.specialresource.mk Makefile.specialresource.mk
COPY Makefile.helper.mk Makefile.helper.mk
COPY Makefile Makefile
COPY scripts/ scripts/

# Copy the go source
COPY vendor/ vendor/
COPY main.go main.go
COPY api/ api/
COPY cmd/ cmd/
COPY controllers/ controllers/
COPY internal/ internal/
COPY pkg/ pkg/

RUN ["make", "manager", "helm-plugins/cm-getter/cm-getter"]

FROM registry.ci.openshift.org/ocp/4.11:base

# Block connectivity also in this stage of the multi-stage Dockerfile
# build. See the comment at the beginning of the first stage for more 
# information on why we're doing this.
ENV http_proxy http://127.0.0.1:80
ENV https_proxy http://127.0.0.1:80
ENV HTTP_PROXY http://127.0.0.1:80
ENV HTTPS_PROXY http://127.0.0.1:80

COPY helm-plugins/ helm-plugins/

WORKDIR /

ENV HELM_PLUGINS /opt/helm-plugins

COPY --from=builder /workspace/manager /manager
COPY --from=builder /workspace/helm-plugins ${HELM_PLUGINS}

RUN useradd  -r -u 499 nonroot
RUN getent group nonroot || groupadd -o -g 499 nonroot

ENTRYPOINT ["/manager"]

LABEL io.k8s.display-name="OpenShift Special Resource Operator" \
      io.k8s.description="This is a component of OpenShift and manages the lifecycle of out-of-tree drivers with enablement stack."

# ======== Warning! Re-enabling connectivity below this line =========
# =========== Warning! Do not add anything below this line ===========

# Restore regular Internet connectivity for actual users of the image 
# See beginning of the file for more information
ENV http_proxy=
ENV https_proxy=
ENV HTTP_PROXY=
ENV HTTPS_PROXY=

# =========== Warning! Do not add anything below this line ===========
