# Build the manager binary
FROM golang:1.13 as builder
ARG VERSION=6.4.0
WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY pkg/ pkg/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -ldflags "-X helm.ChartVersion=$VERSION" -a -o manager main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM alpine/helm:3.1.1
ARG VERSION=6.4.0
WORKDIR /
COPY --from=builder /workspace/manager .
RUN mkdir /data && \
    cd /data && \
    wget https://external-secrets.github.io/kubernetes-external-secrets/kubernetes-external-secrets-$VERSION.tgz

ENTRYPOINT ["/manager"]
