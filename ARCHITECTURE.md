# Architecture

## System Overview

Kairos is a Kubernetes operator for OpenShift that automatically right-sizes workload resources (CPU and memory requests/limits) based on real-time metrics. It uses a hub-spoke architecture for multi-cluster governance.

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           HUB CLUSTER                                    │
│  ┌──────────────────┐  ┌────────────────┐  ┌─────────────────────┐     │
│  │ kairos-operator   │  │ kairos-console │  │ thanos-querier      │     │
│  │ (controller-mgr)  │  │ (Go + React)   │  │ (openshift-monit.)  │     │
│  └────────┬─────────┘  └───────┬────────┘  └──────────┬──────────┘     │
│           │                     │                       │                │
│           ▼                     ▼                       ▼                │
│  ┌──────────────┐     ┌──────────────────┐    ┌──────────────────┐     │
│  │ CRDs:        │     │ /api/v1/agents   │    │ PromQL queries   │     │
│  │ - Policy     │     │ /api/v1/events   │    │ for scaling      │     │
│  │ - Agent      │     │ /api/v1/managed  │    │ decisions        │     │
│  │ - Console    │     │ /api/v1/report   │    │                  │     │
│  └──────────────┘     └────────▲─────────┘    └──────────────────┘     │
│                                │                                         │
└────────────────────────────────┼─────────────────────────────────────────┘
                                 │ POST /api/v1/agent-report
                    ┌────────────┴────────────┐
                    │                         │
┌───────────────────▼─────┐   ┌──────────────▼──────────────┐
│      EAST CLUSTER        │   │       WEST CLUSTER           │
│  ┌──────────────────┐   │   │  ┌──────────────────┐       │
│  │ kairos-operator   │   │   │  │ kairos-operator   │       │
│  │ + KairosAgent     │   │   │  │ + KairosAgent     │       │
│  └────────┬─────────┘   │   │  └────────┬─────────┘       │
│           │              │   │           │                  │
│           ▼              │   │           ▼                  │
│  ┌──────────────────┐   │   │  ┌──────────────────┐       │
│  │ Deployments with  │   │   │  │ Deployments with  │       │
│  │ kairos.io/managed │   │   │  │ kairos.io/managed │       │
│  │ SSA: resources    │   │   │  │ SSA: resources    │       │
│  └──────────────────┘   │   │  └──────────────────┘       │
└──────────────────────────┘   └──────────────────────────────┘
```

## Components

### 1. Operator (Controller Manager)

**Language:** Go 1.22+  
**Framework:** Operator SDK / Kubebuilder  
**Location:** `internal/controller/`

The operator reconciles three Custom Resources:

| Controller | CRD | Responsibility |
|---|---|---|
| `SmartScalingPolicyReconciler` | `SmartScalingPolicy` | Evaluates metric rules, applies resource changes via SSA |
| `KairosAgentReconciler` | `KairosAgent` | Namespace discovery, workload scanning, AI integration, hub reporting |
| `KairosConsoleReconciler` | `KairosConsole` | Deploys/manages console Deployment + Service + Route |

### 2. Governance Console

**Backend:** Go (net/http)  
**Frontend:** React + TypeScript + PatternFly 5  
**Location:** `console/`

The console provides a unified multi-cluster view:

```
console/
├── main.go                 # HTTP server, API handlers, in-cluster K8s queries
├── Dockerfile              # Multi-stage: Go build + Node build + UBI9-micro runtime
└── frontend/
    ├── src/App.tsx          # Navigation shell
    ├── src/pages/
    │   ├── Dashboard.tsx    # Overview with cluster status cards
    │   ├── Agents.tsx       # Agent list with cluster/mode/phase
    │   ├── Events.tsx       # Paginated event log
    │   ├── ManagedResources.tsx  # Multi-cluster resource table with filters
    │   └── Observability.tsx     # Thanos + OTel connection status
    └── package.json
```

### 3. Metrics Layer

**Location:** `internal/metrics/`

```
┌───────────────────────────────────────────────────┐
│              Metrics Client (Priority)             │
├───────────────────────────────────────────────────┤
│ 1. OTel Collector (gRPC:4317)  → OTLP queries    │
│ 2. Thanos Querier (HTTPS:9091) → PromQL          │
│ 3. Prometheus    (HTTP:9090)   → PromQL fallback  │
└───────────────────────────────────────────────────┘
```

The metrics client attempts connections in priority order. Each SmartScalingPolicy can specify `otelEndpoint` and/or `prometheusEndpoint`.

### 4. AI Client

**Location:** `internal/ai/`

Connects to any OpenAI-compatible API endpoint:

```
KairosAgent → AI Client → POST /v1/chat/completions
                              │
                              ▼
                    ┌─────────────────────┐
                    │ vLLM / LiteLLM /    │
                    │ KServe / Ollama     │
                    │                     │
                    │ Models:             │
                    │ - deepseek-r1       │
                    │ - granite-3b        │
                    │ - qwen-14b          │
                    │ - llama-3.1         │
                    └─────────────────────┘
```

The AI receives: current resources, metric history, pod restart events, time context.  
It responds with: recommended resources + confidence level.

### 5. Scaler (SSA Coordinator)

**Location:** `internal/scaler/`

The scaler is the only component that writes to workload resources:

```go
// Field manager ensures ArgoCD coexistence
client.Patch(ctx, obj, client.Apply,
    client.FieldOwner("kairos-operator"),
    client.ForceOwnership,
)
```

Only touches `.spec.template.spec.containers[].resources` — never any other field.

## Data Flow

### Scaling Decision Flow

```
Reconcile Loop (every spec.reporting.interval)
    │
    ▼
┌──────────────┐     ┌──────────────┐     ┌──────────────────┐
│ 1. Discover  │────▶│ 2. Measure   │────▶│ 3. Evaluate      │
│ namespaces   │     │ current      │     │ policy rules     │
│ by suffix    │     │ metrics      │     │                  │
└──────────────┘     └──────────────┘     └────────┬─────────┘
                                                    │
                                          ┌─────────▼──────────┐
                                          │ 4. AI recommend?   │
                                          │ (if mode=autopilot │
                                          │  and aiModel set)  │
                                          └─────────┬──────────┘
                                                    │
                     ┌──────────────┐     ┌─────────▼──────────┐
                     │ 6. Verify    │◀────│ 5. Apply via SSA   │
                     │ rollout ok?  │     │ (resources only)   │
                     └──────┬───────┘     └────────────────────┘
                            │
                  ┌─────────▼──────────┐
                  │ 7. Report to hub   │
                  │ (if hubReporting)  │
                  └────────────────────┘
```

### Multi-Cluster Communication

```
Spoke Agent (KairosAgent)
    │
    │ Every spec.reporting.interval (e.g. 5m)
    │
    ▼
POST /api/v1/agent-report
    │
    │ Payload:
    │ - agent status (name, mode, phase, cluster)
    │ - managed resources list (name, ns, kind, cpu, mem)
    │ - recent events
    │
    ▼
Hub Console (in-memory store)
    │
    ▼
GET /api/v1/managed-resources → Combined: hub local + all spoke reports
GET /api/v1/agents           → Combined: hub local + all spoke agents
GET /api/v1/events           → Combined: hub local + all spoke events
```

## Security Model

### RBAC

The operator ServiceAccount has cluster-wide permissions to:
- `get, list, watch, patch` Deployments and StatefulSets (for SSA)
- `get, list, watch` Namespaces (for suffix discovery)
- `get` Secrets (for AI credentials)
- `create, update, patch` Events

The console ServiceAccount additionally has:
- `cluster-monitoring-view` ClusterRole (Thanos queries)
- `get, list, watch` Deployments/StatefulSets across namespaces (managed resource listing)

### TLS Configuration

All external connections support configurable TLS:

```yaml
tls:
  insecureSkipVerify: false    # Default: verify certificates
  caSecretRef:                 # Custom CA for internal PKI
    name: internal-ca-bundle
    key: ca.crt
```

Applicable to:
- AI model endpoint (`spec.aiModel.tls`)
- Hub reporting endpoint (`spec.hubReporting.tls`)
- Metrics endpoints (`spec.metricsTLS`)

## Technology Stack

| Layer | Technology |
|---|---|
| Operator | Go 1.22+, Operator SDK v1.40.0, controller-runtime |
| Console Backend | Go (net/http), in-cluster K8s client |
| Console Frontend | React 18, TypeScript, PatternFly 5 |
| Container Base | Red Hat UBI9 (go-toolset, nodejs-20, ubi-micro) |
| Metrics | OpenTelemetry OTLP gRPC, Thanos PromQL, Prometheus |
| AI | OpenAI-compatible API (any model) |
| CI/CD | GitHub Actions (test, lint, e2e, build, push, pages) |
| Registries | Quay.io, GitHub Container Registry (GHCR) |
| Documentation | GitHub Pages (PatternFly-styled, dark mode) |

## Deployment Topology

```
Option A: Single Cluster
─────────────────────────
  Operator + Console + Agent (all in kairos-system namespace)
  Metrics from local Thanos/Prometheus

Option B: Multi-Cluster Hub-Spoke
─────────────────────────────────
  Hub:   Operator + Console + Thanos Querier + OTel Collector
  Spoke: Operator + Agent (reports to hub console route)
  
  Each spoke agent manages local workloads and pushes status upstream.
```
