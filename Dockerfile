# Build the operator binary
FROM registry.access.redhat.com/ubi9/go-toolset:1.22 AS builder
ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /opt/app-root/src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -a -o manager cmd/main.go

# Runtime image
FROM registry.access.redhat.com/ubi9/ubi-micro:latest

LABEL name="kairos-operator" \
      vendor="maximilianopizarro" \
      version="0.1.0" \
      summary="Kairos Operator - Smart resource scaling for OpenShift" \
      description="OpenShift operator for intelligent resource management with OTel metrics and optional AI-powered autopilot" \
      io.k8s.display-name="Kairos Operator" \
      io.k8s.description="Smart scaling operator with AI support" \
      io.openshift.tags="operator,scaling,ai,otel"

COPY --from=builder /opt/app-root/src/manager /manager

USER 65532:65532
ENTRYPOINT ["/manager"]
