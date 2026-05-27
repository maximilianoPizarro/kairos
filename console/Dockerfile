# Build frontend
FROM registry.access.redhat.com/ubi9/nodejs-20:latest AS frontend-builder
WORKDIR /opt/app-root/src
COPY console/frontend/package*.json ./
RUN npm ci
COPY console/frontend/ .
RUN npm run build

# Build backend
FROM registry.access.redhat.com/ubi9/go-toolset:latest AS backend-builder
WORKDIR /opt/app-root/src
COPY console/go.mod console/go.sum ./
RUN go mod download
COPY console/ .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o console-server main.go

# Runtime
FROM registry.access.redhat.com/ubi9/ubi-micro:latest

LABEL name="kairos-console" \
      vendor="maximilianopizarro" \
      version="1.1.0" \
      summary="Kairos Console - Multi-cluster governance dashboard" \
      description="Web console for visualizing Kairos agents, policies and scaling events across OpenShift clusters" \
      io.k8s.display-name="Kairos Console" \
      io.k8s.description="Multi-cluster governance dashboard" \
      io.openshift.tags="console,dashboard,governance,multicluster"

COPY --from=backend-builder /opt/app-root/src/console-server /console-server
COPY --from=frontend-builder /opt/app-root/src/dist /static

USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/console-server"]
