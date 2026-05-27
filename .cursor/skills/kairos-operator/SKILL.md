# Kairos Operator Skill

Use this skill when working on the Kairos operator codebase. It provides context about architecture, patterns, conventions, and multi-cluster capabilities.

## Project Overview

Kairos is a Go-native OpenShift operator built with Operator SDK/Kubebuilder that **intelligently right-sizes workload resources** (CPU/memory requests and limits) using real-time metrics from OpenTelemetry, Thanos, or Prometheus — with optional AI-powered recommendations. It coexists with ArgoCD using Server-Side Apply (SSA).

## Key Architectural Decisions

### Server-Side Apply (SSA) for ArgoCD Coexistence

The operator ONLY modifies `.spec.template.spec.containers[].resources` using Server-Side Apply with field owner `kairos-operator`. This prevents conflicts with ArgoCD which manages all other fields.

```go
client.Patch(ctx, obj, client.Apply, client.FieldOwner("kairos-operator"), client.ForceOwnership)
```

Users must:
1. Leave `resources: {}` empty in their Git-managed manifests
2. Add annotation `kairos.io/managed: "true"` to opt-in
3. Configure `ignoreDifferences` in ArgoCD Applications

### Anti-Crash Pattern (from jhipster-online-operator)

All controllers use:
- `MaxConcurrentReconciles: 2` (or 1 for agents)
- `ExponentialFailureRateLimiter`: 5s base, 5min max
- `RequeueAfter` on errors (NEVER immediate requeue)
- No indiscriminate `Owns()` or `Watches()`

### Multi-Cluster Hub-Spoke Architecture

- **Hub cluster**: runs operator + governance console + Thanos Querier + OTel Collector
- **Spoke clusters**: run operator + KairosAgent that reports to hub
- **Communication**: Spoke agents push status via `POST /api/v1/agent-report` to hub console
- **Security**: Configurable TLS (full verify, custom CA, or insecureSkipVerify for air-gapped)

### Managed Resources Multi-Cluster Visibility

The console aggregates resources from:
1. **Hub**: queries local Kubernetes API for `kairos.io/managed: "true"` annotated workloads
2. **Spoke clusters**: agents include `managedResources` array in their periodic reports
3. **Console**: combines both into a unified view with cluster filtering

### CRDs

| CRD | Purpose |
|---|---|
| `SmartScalingPolicy` | Metric-based and schedule-based scaling rules with default resource baselines |
| `KairosAgent` | AI agent for autonomous corrections with namespace suffix discovery |
| `KairosConsole` | Deploys the multi-cluster governance web console |

### KairosAgent Configuration Modes

| Mode | AI | Namespace Selection | TLS | Best For |
|---|---|---|---|---|
| A | No | Explicit list | Standard | Simple metric-based, few namespaces |
| B | Yes (autopilot) | Suffix pattern | Configurable | Large envs with many *-prod namespaces |
| C | Yes (supervised) | Combined | mTLS/token | Critical workloads + dynamic discovery |
| D | No | Explicit list | InsecureSkipVerify | Disconnected / air-gapped / industrial |

## Directory Structure

```
api/v1alpha1/              - CRD type definitions (SmartScalingPolicy, KairosAgent, KairosConsole)
internal/controller/       - Reconcilers for each CRD
internal/metrics/          - OTel (gRPC/OTLP) and Prometheus/Thanos clients
internal/ai/               - OpenAI-compatible AI client (any model via vLLM/LiteLLM/Ollama)
internal/scaler/           - Scaling coordinator with SSA field ownership
console/                   - Web console (Go backend + React/PatternFly 5 frontend)
console/frontend/          - React app (TypeScript, PatternFly 5 components)
config/                    - Kustomize manifests (CRDs, RBAC, manager, samples)
config/samples/            - Ready-to-use CR examples
bundle/                    - OLM bundle for OperatorHub
docs/                      - Architecture docs, SVG diagrams
docs/pages/                - GitHub Pages documentation site
.github/workflows/         - CI (test, lint, e2e), CD (build+push), Pages
```

## Build Commands

```bash
# Operator
make generate manifests        # Regenerate CRDs + RBAC after type changes
make test                      # Unit tests
make lint                      # golangci-lint
make docker-build IMG=quay.io/maximilianopizarro/kairos-operator:v1.0.0
make docker-push IMG=quay.io/maximilianopizarro/kairos-operator:v1.0.0
make deploy IMG=quay.io/maximilianopizarro/kairos-operator:v1.0.0

# Console (multi-stage: Go backend + React frontend)
podman build -f console/Dockerfile -t quay.io/maximilianopizarro/kairos-console:latest .
podman push quay.io/maximilianopizarro/kairos-console:latest

# Local development
make install                   # Install CRDs into cluster
make run                       # Run operator locally (outside cluster)
```

## Container Images (Red Hat UBI9)

All images use official Red Hat base images:
- Builder: `registry.access.redhat.com/ubi9/go-toolset:latest`
- Frontend builder: `registry.access.redhat.com/ubi9/nodejs-20:latest`
- Runtime: `registry.access.redhat.com/ubi9/ubi-micro:latest`

Published to:
- `quay.io/maximilianopizarro/kairos-operator`
- `quay.io/maximilianopizarro/kairos-console`
- `ghcr.io/maximilianopizarro/kairos-operator`
- `ghcr.io/maximilianopizarro/kairos-console`

## Key Annotations

| Annotation | Purpose |
|---|---|
| `kairos.io/managed` | Must be "true" for operator to touch a resource |
| `kairos.io/policy` | Links resource to a SmartScalingPolicy |
| `kairos.io/agent` | Which KairosAgent manages this resource |
| `kairos.io/last-action` | Records last scaling action |
| `kairos.io/last-action-time` | Cooldown tracking |

## AI Integration

Optional connection to OpenAI-compatible APIs (vLLM, KServe, Ollama, LiteLLM). The AI client:
- Sends infrastructure context (metrics history, current state, pod restart history)
- Receives scaling recommendations with confidence levels
- Operates in `autopilot` (auto-apply) or `supervised` (log for review) mode
- Supports TLS configuration including `insecureSkipVerify` for self-signed certs

## Metrics Priority

1. **OpenTelemetry** (gRPC/OTLP port 4317) — lowest latency, push-based
2. **Thanos Querier** (PromQL/HTTPS port 9091) — federated multi-cluster (OpenShift built-in)
3. **Prometheus** (PromQL/HTTP port 9090) — single-cluster fallback

## Console API Endpoints

| Endpoint | Method | Purpose |
|---|---|---|
| `/api/v1/agents` | GET | List all agents (hub + spoke reported) |
| `/api/v1/policies` | GET | List SmartScalingPolicies |
| `/api/v1/events` | GET | Scaling events across all clusters |
| `/api/v1/managed-resources` | GET | Multi-cluster managed resource inventory |
| `/api/v1/agent-report` | POST | Spoke agents push status + resources here |
| `/api/v1/observability` | GET | Thanos + OTel connection status |
| `/api/v1/metrics/query` | GET | Proxy PromQL queries to Thanos |

## Testing with Multi-cluster

1. Deploy operator on hub: `make deploy`
2. Deploy console: `oc apply -f config/samples/kairos_v1alpha1_kairosconsole.yaml`
3. Deploy agents on spoke clusters with `hubReporting.endpoint` pointing to hub console route
4. Agents automatically report status and managed resources to hub
5. Console shows unified multi-cluster view

## Namespace Suffix Pattern

Instead of listing namespaces explicitly, use `spec.watch.namespaceSuffix`:
```yaml
watch:
  namespaceSuffix: "-prod"    # Discovers ALL *-prod namespaces automatically
  resourceTypes: [Deployment, StatefulSet]
```

## Safety Mechanisms

- `correctionPolicy.maxActionsPerHour`: rate-limit corrections
- `correctionPolicy.cooldownPeriod`: min time between actions on same workload
- `correctionPolicy.rollbackOnFailure`: revert if pods crash after resize
- `correctionPolicy.dryRun`: evaluate but don't apply (testing mode)
