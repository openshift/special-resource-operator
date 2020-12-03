# Build the manager binary
FROM registry.svc.ci.openshift.org/ocp/builder:rhel-8-golang-1.15-openshift-4.7 AS builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
# RUN go mod download

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/

COPY yamlutil/ yamlutil/
COPY vendor/ vendor/


# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -mod=vendor -a -o manager main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM registry.svc.ci.openshift.org/ocp/4.7:base
WORKDIR /
COPY --from=builder /workspace/manager .

COPY config/recipes/ /opt/sro/recipes/
COPY manifests /manifests

USER nonroot:nonroot
ENTRYPOINT ["/manager"]

LABEL io.k8s.display-name="OpenShift special-resource-operator" \
        io.k8s.description="This is a component of OpenShift and manages the lifecycle of out-of-tree drivers with enablement stack." \
        io.openshift.release.operator=true
