#!/usr/bin/env bash
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
NC='\033[0m'

info() { echo -e "${CYAN}[INFO]${NC} $1"; }
ok() { echo -e "${GREEN}[OK]${NC} $1"; }
fail() { echo -e "${RED}[FAIL]${NC} $1"; exit 1; }

CLUSTER_NAME="kairos-dev"
NAMESPACE="kairos-system"
DEMO_NS="demo-app"
IMG="quay.io/maximilianopizarro/kairos-operator:v1.1.0"

info "Kairos Operator — Quick Start with kind"
echo "========================================="
echo ""

# Check prerequisites
for cmd in kind kubectl docker; do
  command -v $cmd &>/dev/null || fail "$cmd is required but not installed"
done
ok "Prerequisites: kind, kubectl, docker found"

# Create kind cluster
if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
  info "Cluster '${CLUSTER_NAME}' already exists, reusing"
else
  info "Creating kind cluster '${CLUSTER_NAME}'..."
  kind create cluster --name "${CLUSTER_NAME}" --wait 60s
fi
ok "Kind cluster '${CLUSTER_NAME}' is ready"

# Set context
kubectl cluster-info --context "kind-${CLUSTER_NAME}" >/dev/null 2>&1
ok "kubectl context set to kind-${CLUSTER_NAME}"

# Install CRDs
info "Installing Kairos CRDs..."
kubectl apply -f config/crd/bases/ 2>/dev/null || kubectl apply -f config/crd/bases/
ok "CRDs installed"

# Create operator namespace
kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

# Deploy operator
info "Deploying Kairos operator..."
cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kairos-controller-manager
  namespace: ${NAMESPACE}
  labels:
    control-plane: controller-manager
    kairos.io/managed: "true"
spec:
  replicas: 1
  selector:
    matchLabels:
      control-plane: controller-manager
  template:
    metadata:
      labels:
        control-plane: controller-manager
    spec:
      containers:
      - name: manager
        image: ${IMG}
        imagePullPolicy: IfNotPresent
        command: ["/manager"]
        resources:
          limits:
            cpu: 200m
            memory: 256Mi
          requests:
            cpu: 100m
            memory: 128Mi
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 15
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
          initialDelaySeconds: 5
      serviceAccountName: kairos-controller-manager
      terminationGracePeriodSeconds: 10
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kairos-controller-manager
  namespace: ${NAMESPACE}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kairos-manager-rolebinding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
- kind: ServiceAccount
  name: kairos-controller-manager
  namespace: ${NAMESPACE}
EOF
ok "Operator deployed"

# Create demo application
info "Creating demo application..."
kubectl create namespace "${DEMO_NS}" --dry-run=client -o yaml | kubectl apply -f -

cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo-api
  namespace: ${DEMO_NS}
  annotations:
    kairos.io/managed: "true"
  labels:
    app: demo-api
spec:
  replicas: 2
  selector:
    matchLabels:
      app: demo-api
  template:
    metadata:
      labels:
        app: demo-api
    spec:
      containers:
      - name: api
        image: registry.access.redhat.com/ubi9/ubi-minimal:latest
        command: ["sleep", "infinity"]
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 512Mi
EOF
ok "Demo deployment 'demo-api' created in namespace '${DEMO_NS}'"

# Create sample KairosAgent
info "Creating sample KairosAgent..."
cat <<EOF | kubectl apply -f -
apiVersion: kairos.maximilianopizarro.github.io/v1alpha1
kind: KairosAgent
metadata:
  name: dev-agent
  namespace: ${NAMESPACE}
spec:
  mode: supervised
  aiModel:
    apiURL: "http://localhost:11434/v1"
    model: "llama3"
  watch:
    namespaces:
      - ${DEMO_NS}
    resourceTypes:
      - Deployment
  correctionPolicy:
    maxActionsPerHour: 5
    dryRun: true
  reporting:
    interval: "60s"
EOF
ok "KairosAgent 'dev-agent' created (dry-run mode)"

# Wait for operator to start
info "Waiting for operator pod to be ready..."
kubectl wait --for=condition=available deployment/kairos-controller-manager \
  -n "${NAMESPACE}" --timeout=120s 2>/dev/null || info "Operator may take a moment to pull image"

echo ""
echo "========================================="
ok "Kairos Quick Start complete!"
echo ""
info "Useful commands:"
echo "  kubectl get kairosagents -n ${NAMESPACE}"
echo "  kubectl get kairosevents -n ${NAMESPACE}"
echo "  kubectl describe kairosagent dev-agent -n ${NAMESPACE}"
echo ""
info "To clean up:"
echo "  kind delete cluster --name ${CLUSTER_NAME}"
echo ""
