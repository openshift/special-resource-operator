# Build the manager binary
FROM registry.ci.openshift.org/ocp/builder:rhel-8-golang-1.18-openshift-4.11 AS builder

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
