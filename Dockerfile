# Build the manager binary
FROM golang:1.17-bullseye AS builder

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

COPY hack/ hack/
COPY helm-plugins/ helm-plugins/
COPY Makefile.specialresource.mk Makefile.specialresource.mk
COPY Makefile.helm.mk Makefile.helm.mk
COPY Makefile.helper.mk Makefile.helper.mk
COPY Makefile Makefile
COPY scripts/ scripts/

# Copy the go source
COPY vendor/ vendor/
COPY .patches/ .patches/
COPY main.go main.go
COPY api/ api/
COPY cmd/ cmd/
COPY controllers/ controllers/
COPY pkg/ pkg/

RUN ["make", "manager", "helm-plugins/cm-getter/cm-getter"]

FROM debian:bullseye-slim

RUN ["apt", "update"]
RUN ["apt", "install", "-y", "ca-certificates"]

WORKDIR /

ENV HELM_PLUGINS /opt/helm-plugins

COPY --from=builder /workspace/manager .
COPY --from=builder /workspace/helm-plugins ${HELM_PLUGINS}

COPY build/charts/ /charts/

RUN useradd  -r -u 499 nonroot
RUN getent group nonroot || groupadd -o -g 499 nonroot

ENTRYPOINT ["/manager"]

LABEL io.k8s.display-name="OpenShift Special Resource Operator" \
      io.k8s.description="This is a component of OpenShift and manages the lifecycle of out-of-tree drivers with enablement stack."
