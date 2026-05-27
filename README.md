# Kairos Operator

<div align="center">
  <img src="docs/images/kairos-logo.svg" alt="Kairos Logo" width="150">
  
  ### καιρός — *The Opportune Moment*
  
  > *"Kairos is the fleeting moment of opportunity — the instant when conditions align perfectly for action. In infrastructure, it's the precise moment to scale before latency spikes, and to release resources before waste accumulates."*

  [![CI](https://github.com/maximilianoPizarro/kairos/actions/workflows/ci.yaml/badge.svg)](https://github.com/maximilianoPizarro/kairos/actions/workflows/ci.yaml)
  [![E2E Tests](https://github.com/maximilianoPizarro/kairos/actions/workflows/test-e2e.yml/badge.svg)](https://github.com/maximilianoPizarro/kairos/actions/workflows/test-e2e.yml)
  [![GitHub Pages](https://github.com/maximilianoPizarro/kairos/actions/workflows/pages.yaml/badge.svg)](https://github.com/maximilianoPizarro/kairos/actions/workflows/pages.yaml)
  [![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
  [![OpenShift](https://img.shields.io/badge/OpenShift-4.14+-red.svg)](https://www.redhat.com/en/technologies/cloud-computing/openshift)
  [![Operator SDK](https://img.shields.io/badge/Operator%20SDK-v1.40.0-blueviolet.svg)](https://sdk.operatorframework.io/)
  [![Go](https://img.shields.io/badge/Go-1.23-00ADD8.svg)](https://go.dev/)
  [![Documentation](https://img.shields.io/badge/Docs-GitHub%20Pages-brightgreen.svg)](https://maximilianoPizarro.github.io/kairos)
</div>

---

OpenShift operator for **intelligent resource management** with OpenTelemetry metrics and optional AI-powered autopilot. Designed to coexist with ArgoCD in multi-cluster GitOps environments using Server-Side Apply (SSA).

## Architecture

![Kairos Architecture](docs/images/architecture-overview.svg)

## Scaling Flow

![Scaling Flow](docs/images/scaling-flow.svg)

## Features

- **Smart Scaling Policies** — Define metric-based and schedule-based scaling rules as Kubernetes CRDs. Rules evaluate OTel/Prometheus/Thanos metrics and trigger horizontal (replicas) or vertical (CPU/memory) scaling.
- **AI-Powered Agents** — Optional integration with AI models (DeepSeek, Granite, Qwen via vLLM/KServe/LiteLLM) for autonomous resource optimization in autopilot or supervised mode.
- **OpenTelemetry Native** — Reads metrics from OTel Collector via gRPC (OTLP), with Prometheus/Thanos fallback. Compatible with Red Hat build of OpenTelemetry.
- **ArgoCD Compatible** — Uses Server-Side Apply with field ownership to manage only the `resources` field. No sync conflicts with GitOps.
- **Namespace Suffix Filtering** — Watch all namespaces matching a suffix pattern (`-dev`, `-test`, `-qa`, `-prod`) automatically.
- **Multi-cluster Governance Console** — PatternFly 5 dashboard for real-time visualization of agents, policies, and events across clusters.
- **Safe by Design** — Rate limiting, exponential backoff, cooldown periods, approval workflows, and automatic rollback on failure.

## Metrics Integration

Kairos supports three metrics sources (in priority order):

| Priority | Source | Protocol | Use Case |
|---|---|---|---|
| 1 | OpenTelemetry Collector | gRPC/OTLP (port 4317) | Primary — lowest latency, push-based |
| 2 | Thanos Querier | PromQL over HTTPS (port 9091) | Federated multi-cluster metrics |
| 3 | Prometheus | PromQL over HTTP (port 9090) | Single-cluster fallback |

### Configuring with OpenTelemetry

Deploy an `OpenTelemetryCollector` CR that scrapes Thanos and exposes OTLP gRPC:

```yaml
apiVersion: opentelemetry.io/v1beta1
kind: OpenTelemetryCollector
metadata:
  name: kairos-otel
  namespace: kairos-system
spec:
  mode: deployment
  config:
    receivers:
      otlp:
        protocols:
          grpc:
            endpoint: 0.0.0.0:4317
      prometheus:
        config:
          scrape_configs:
          - job_name: thanos-federated
            scrape_interval: 30s
            scheme: https
            tls_config:
              insecure_skip_verify: true
            bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token
            static_configs:
            - targets: ['thanos-querier.openshift-monitoring.svc:9091']
    exporters:
      debug: {}
    service:
      pipelines:
        metrics:
          receivers: [otlp, prometheus]
          exporters: [debug]
```

Then reference it in your SmartScalingPolicy:

```yaml
spec:
  otelEndpoint: "kairos-otel-collector.kairos-system.svc:4317"
  prometheusEndpoint: "https://thanos-querier.openshift-monitoring.svc:9091"
```

### Using Thanos Querier (OpenShift built-in)

OpenShift clusters include Thanos Querier by default. No additional installation required:

```yaml
apiVersion: kairos.maximilianopizarro.github.io/v1alpha1
kind: SmartScalingPolicy
metadata:
  name: my-policy
spec:
  target:
    apiVersion: apps/v1
    kind: Deployment
    name: my-app
    namespace: my-namespace
  prometheusEndpoint: "https://thanos-querier.openshift-monitoring.svc:9091"
  rules:
  - name: cpu-high
    when:
      metric: "container_cpu_usage_seconds_total"
      operator: GreaterThan
      threshold: "0.8"
    action:
      type: AddReplicas
```

## AI Model Configuration

Create a secret with your API key:

```bash
oc create secret generic kairos-ai-credentials \
  -n kairos-system \
  --from-literal=api-key=<your-api-key>
```

Configure the KairosAgent:

```yaml
apiVersion: kairos.maximilianopizarro.github.io/v1alpha1
kind: KairosAgent
metadata:
  name: my-agent
spec:
  mode: autopilot          # or "supervised" for human approval
  aiModel:
    apiURL: "https://litellm-prod.apps.maas.redhatworkshops.io/v1/chat/completions"
    model: "deepseek-r1-distill-qwen-14b"
    secretRef:
      name: kairos-ai-credentials
      key: api-key
  watch:
    namespaceSuffix: "-prod"
    resourceTypes:
    - Deployment
    - StatefulSet
  correctionPolicy:
    maxActionsPerHour: 10
    rollbackOnFailure: true
  reporting:
    interval: "5m"
```

Supported AI endpoints: vLLM, KServe, LiteLLM, Ollama, OpenAI-compatible APIs.

## Namespace Suffix Mode

Instead of listing namespaces manually, define a suffix pattern:

```yaml
spec:
  watch:
    namespaceSuffix: "-prod"   # Watches ALL *-prod namespaces automatically
    resourceTypes:
    - Deployment
    - StatefulSet
```

Available suffix patterns: `-dev`, `-test`, `-qa`, `-prod` (or any custom suffix).

## Quick Start

### Prerequisites

- OpenShift 4.14+ or Kubernetes 1.25+
- Cluster admin access
- Red Hat build of OpenTelemetry (recommended, available via OperatorHub)
- Thanos Querier (included by default in OpenShift monitoring stack)
- (Optional) AI model endpoint for autopilot mode

### Install

```bash
# Clone the repository
git clone https://github.com/maximilianoPizarro/kairos.git
cd kairos

# Install CRDs into the cluster
make install

# Deploy the operator (uses kustomize)
make deploy IMG=quay.io/maximilianopizarro/kairos-operator:v0.1.0

# Verify the operator is running
oc get pods -n kairos-system
# NAME                                        READY   STATUS    RESTARTS   AGE
# kairos-operator-controller-manager-xxx      1/1     Running   0          30s
```

### Mark a workload for Kairos management

Add the annotation `kairos.io/managed: "true"` to any Deployment or StatefulSet. Leave `resources: {}` empty so Kairos manages it via Server-Side Apply:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-service
  annotations:
    kairos.io/managed: "true"       # Opt-in to Kairos management
    kairos.io/policy: "my-policy"   # Link to a SmartScalingPolicy
spec:
  template:
    spec:
      containers:
      - name: app
        image: my-image:latest
        resources: {}   # Leave EMPTY - Kairos manages this via SSA
```

### Create a SmartScalingPolicy

```bash
# Apply the sample scaling policy
oc apply -f config/samples/kairos_v1alpha1_smartscalingpolicy.yaml

# Check status
oc get smartscalingpolicies -n kairos-system
# NAME             TARGET          ACTIVE RULES   LAST EVALUATION
# my-policy        my-app          2              2026-05-27T19:23:29Z
```

### Deploy the AI Agent (optional)

```bash
# Create the AI credentials secret
oc create secret generic kairos-ai-credentials \
  -n kairos-system \
  --from-literal=api-key=<your-api-key>

# Deploy the agent
oc apply -f config/samples/kairos_v1alpha1_kairosagent.yaml

# Check agent status
oc get kairosagents -n kairos-system
# NAME         MODE        AI MODEL                        PHASE    WATCHED
# hub-agent    supervised  deepseek-r1-distill-qwen-14b    Active   8
```

### Deploy the Governance Console

```bash
# Deploy the console
oc apply -f config/samples/kairos_v1alpha1_kairosconsole.yaml

# Get the route URL
oc get route kairos-console -n kairos-system -o jsonpath='{.spec.host}'
# kairos-console.apps.your-cluster.example.com
```

## ArgoCD Coexistence

See [docs/argocd-coexistence.md](docs/argocd-coexistence.md) for the complete guide.

**TL;DR:** Leave `resources: {}` empty in your Git manifests, add `kairos.io/managed: "true"` annotation, and configure `ignoreDifferences` in your ArgoCD Application:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: my-app
spec:
  ignoreDifferences:
  - group: apps
    kind: Deployment
    jqPathExpressions:
    - .spec.template.spec.containers[].resources
```

## Container Images

All images are built on Red Hat Universal Base Images (UBI9) for security and compliance.

| Image | Purpose | Base | Registry |
|---|---|---|---|
| `kairos-operator` | Controller manager | UBI9 Micro | [Quay.io](https://quay.io/repository/maximilianopizarro/kairos-operator) / [GHCR](https://github.com/maximilianoPizarro/kairos/pkgs/container/kairos-operator) |
| `kairos-console` | Governance dashboard | UBI9 Micro | [Quay.io](https://quay.io/repository/maximilianopizarro/kairos-console) / [GHCR](https://github.com/maximilianoPizarro/kairos/pkgs/container/kairos-console) |
| `kairos-operator-bundle` | OLM bundle | UBI9 Micro | [Quay.io](https://quay.io/repository/maximilianopizarro/kairos-operator-bundle) |
| `kairos-operator-catalog` | OLM catalog | OPM | [Quay.io](https://quay.io/repository/maximilianopizarro/kairos-operator-catalog) |

Pull images:
```bash
# From Quay.io
podman pull quay.io/maximilianopizarro/kairos-operator:v0.1.0
podman pull quay.io/maximilianopizarro/kairos-console:v0.1.3

# From GitHub Container Registry
podman pull ghcr.io/maximilianopizarro/kairos-operator:v0.1.0
podman pull ghcr.io/maximilianopizarro/kairos-console:v0.1.3
```

## CRDs

| Kind | Purpose | Key Fields |
|---|---|---|
| `SmartScalingPolicy` | Metric & schedule scaling rules | target, otelEndpoint, prometheusEndpoint, rules, schedule, ai, paused |
| `KairosAgent` | AI autonomous optimization agent | mode, aiModel, watch.namespaceSuffix, correctionPolicy |
| `KairosConsole` | Multi-cluster governance console | route, auth, clusters |

## Development

### Build locally

```bash
# Build the operator image
make docker-build IMG=quay.io/maximilianopizarro/kairos-operator:dev

# Build the console image (from project root)
podman build -f console/Dockerfile -t quay.io/maximilianopizarro/kairos-console:dev .

# Push images
make docker-push IMG=quay.io/maximilianopizarro/kairos-operator:dev
podman push quay.io/maximilianopizarro/kairos-console:dev
```

### Run tests

```bash
# Unit tests with envtest
make test

# Run specific controller test
go test ./internal/controller/... -v -run TestSmartScalingPolicy

# E2E tests (requires Kind cluster)
make test-e2e
```

### Generate manifests after type changes

```bash
# After modifying api/v1alpha1/*_types.go, regenerate:
make generate    # Updates DeepCopy methods
make manifests   # Updates CRD YAML in config/crd/bases/

# Verify everything is in sync
git diff --exit-code
```

### Install CRDs for local development

```bash
# Install CRDs into the current cluster context
make install

# Run the operator locally (without container)
make run

# Uninstall CRDs
make uninstall
```

### Lint

```bash
# Run golangci-lint
golangci-lint run

# Format code
gofmt -w .
```

## Documentation

Full documentation available at: **https://maximilianoPizarro.github.io/kairos**

The documentation covers:
- Multi-cluster configuration (hub-spoke topology)
- Observability setup (OpenTelemetry + Thanos)
- AI model integration
- ArgoCD coexistence patterns
- CRD reference
- Installation guide

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-feature`)
3. Commit your changes (`git commit -m 'Add my feature'`)
4. Push to the branch (`git push origin feature/my-feature`)
5. Open a Pull Request

Ensure all tests pass (`make test`) and lints are clean before submitting.

## License

Apache License 2.0
