# Build the manager binary
FROM golang:1.18-bullseye AS builder

WORKDIR /workspace

# Following sections are ordered by expected chance of being changed in daily developer's work:
# go.mod is expected to change less frequently than the SRO's code.
# This approach leverages layer caching to speed up rebuilds.

COPY go.mod go.mod
COPY go.sum go.sum
RUN ["go",  "mod", "download"]

COPY Makefile* ./
RUN ["make", "controller-gen"]

COPY cmd/ cmd/
RUN ["make", "helm-plugins/cm-getter/cm-getter"]

COPY hack/ hack/
COPY helm-plugins/ helm-plugins/
COPY scripts/ scripts/

COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/
COPY internal/ internal/
COPY pkg/ pkg/

# Required to include the build information into the binary
COPY .git .git

RUN ["make", "manager"]

FROM debian:bullseye-slim

RUN ["apt", "update"]
RUN ["apt", "install", "-y", "ca-certificates"]

WORKDIR /

ENV HELM_PLUGINS /opt/helm-plugins
RUN useradd  -r -u 499 nonroot
RUN getent group nonroot || groupadd -o -g 499 nonroot

COPY --from=builder /workspace/helm-plugins ${HELM_PLUGINS}
COPY --from=builder /workspace/manager /manager

ENTRYPOINT ["/manager"]

LABEL io.k8s.display-name="Special Resource Operator" \
      io.k8s.description="SRO manages the lifecycle of out-of-tree drivers with enablement stack."
