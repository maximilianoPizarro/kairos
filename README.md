# Kairos Operator

OpenShift operator for intelligent resource management with OpenTelemetry metrics and optional AI-powered autopilot. Designed to coexist with ArgoCD in multi-cluster GitOps environments.

## Features

- **Smart Scaling Policies** - Define metric-based and schedule-based scaling rules as Kubernetes CRDs
- **AI-Powered Agents** - Optional integration with AI models (vLLM, KServe, Ollama, OpenAI) for autonomous resource optimization
- **OpenTelemetry Native** - Reads metrics directly from OTel Collector via gRPC, with Prometheus fallback
- **ArgoCD Compatible** - Uses Server-Side Apply to manage only the `resources` field without causing sync conflicts
- **Multi-cluster Governance Console** - PatternFly 5 web UI for visualizing agents, policies, and events across clusters
- **Safe by Design** - Rate limiting, cooldowns, approval workflows, and rollback on failure

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        Hub Cluster                           │
│                                                             │
│  ┌─────────────┐    ┌──────────────┐    ┌───────────────┐  │
│  │ Kairos      │◄───│ OTel         │◄───│ Workloads     │  │
│  │ Operator    │    │ Collector    │    │ (east/west)   │  │
│  └──────┬──────┘    └──────────────┘    └───────────────┘  │
│         │                                                   │
│         ▼                                                   │
│  ┌─────────────┐    ┌──────────────┐                       │
│  │ AI Model    │    │ Kairos       │                       │
│  │ (optional)  │    │ Console      │                       │
│  └─────────────┘    └──────────────┘                       │
└─────────────────────────────────────────────────────────────┘

ArgoCD manages: image, replicas (in Git), all non-resource fields
Kairos manages: .spec.template.spec.containers[].resources (via SSA)
```

## Quick Start

### Prerequisites

- OpenShift 4.17+ or Kubernetes 1.25+
- Cluster admin access
- (Optional) OTel Collector deployed
- (Optional) AI model endpoint (vLLM, KServe)

### Install the Operator

```bash
# Via OLM (when published)
# Or directly:
make deploy IMG=quay.io/maximilianopizarro/kairos-operator:v0.1.0
```

### Create a Scaling Policy

```yaml
apiVersion: kairos.maximilianopizarro.github.io/v1alpha1
kind: SmartScalingPolicy
metadata:
  name: my-service-policy
spec:
  target:
    apiVersion: apps/v1
    kind: Deployment
    name: my-service
    namespace: my-namespace
  rules:
  - name: high-latency
    when:
      metric: "http.server.request.duration.p99"
      operator: GreaterThan
      threshold: "200"
      for: "2m"
    action:
      type: IncreaseResources
      increaseMemoryPercent: 25
      maxMemory: "4Gi"
      cooldown: "5m"
```

### Enable AI Agent (Optional)

```yaml
apiVersion: kairos.maximilianopizarro.github.io/v1alpha1
kind: KairosAgent
metadata:
  name: my-agent
spec:
  mode: supervised  # or "autopilot"
  aiModel:
    apiURL: "https://my-model-endpoint/v1/chat/completions"
    model: "granite-31-8b"
    apiKeySecret:
      name: ai-credentials
      key: token
  watch:
    namespaces: ["my-namespace"]
    resourceTypes: ["Deployment"]
  correctionPolicy:
    maxActionsPerHour: 5
    rollbackOnFailure: true
```

### Deploy the Console

```yaml
apiVersion: kairos.maximilianopizarro.github.io/v1alpha1
kind: KairosConsole
metadata:
  name: kairos-console
spec:
  route:
    enabled: true
    host: "kairos.apps.my-cluster.example.com"
  auth:
    type: openshift-oauth
```

## ArgoCD Coexistence

See [docs/argocd-coexistence.md](docs/argocd-coexistence.md) for the complete guide.

**TL;DR:** Leave `resources: {}` empty in your Git manifests and add `kairos.io/managed: "true"` annotation.

## Container Images

| Image | Purpose |
|---|---|
| `quay.io/maximilianopizarro/kairos-operator` | Operator controller manager |
| `quay.io/maximilianopizarro/kairos-console` | Web governance console |
| `quay.io/maximilianopizarro/kairos-operator-bundle` | OLM bundle |
| `quay.io/maximilianopizarro/kairos-operator-catalog` | OLM catalog |

## Development

### Build locally

```bash
# Operator
make docker-build IMG=quay.io/maximilianopizarro/kairos-operator:v0.1.0
make docker-push IMG=quay.io/maximilianopizarro/kairos-operator:v0.1.0

# Console
make docker-build-console CONSOLE_IMG=quay.io/maximilianopizarro/kairos-console:v0.1.0
make docker-push-console CONSOLE_IMG=quay.io/maximilianopizarro/kairos-console:v0.1.0

# All at once
make release
```

### Run tests

```bash
make test
```

### Generate manifests

```bash
make generate manifests
```

## CRDs

| CRD | Purpose |
|---|---|
| `SmartScalingPolicy` | Defines metric-based and schedule-based scaling rules |
| `KairosAgent` | AI-powered autonomous agent for resource optimization |
| `KairosConsole` | Deploys the multi-cluster governance web console |

## Integration with platform-hub-spoke-config

Kairos integrates as a component in the [platform-hub-spoke-config](https://github.com/maximilianoPizarro/platform-hub-spoke-config) Helm chart at sync wave 7.

## License

Apache License 2.0
