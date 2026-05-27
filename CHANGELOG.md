# Changelog

All notable changes to the Kairos Operator project are documented here.

## [v1.0.0] - 2026-05-27

### Initial Release

First public release of the Kairos Operator for OpenShift intelligent resource management.

### Features

#### Core Operator
- **SmartScalingPolicy CRD**: Define metric-based scaling rules with default resource baselines, PromQL conditions, and automatic actions (SetResources, AddReplicas)
- **KairosAgent CRD**: AI-powered autonomous optimization agent with namespace suffix discovery, configurable correction policies, and hub reporting
- **KairosConsole CRD**: Deploy a multi-cluster governance dashboard with automatic Route and Service creation
- **Server-Side Apply (SSA)**: Manages only `.spec.template.spec.containers[].resources` with field owner `kairos-operator` — zero ArgoCD conflicts
- **Anti-crash controller pattern**: Exponential backoff rate limiter (5s-5min), limited concurrent reconciles, safe requeue strategies

#### Multi-Cluster Governance
- **Hub-spoke architecture**: Operator runs on all clusters, console runs on hub, agents report upstream
- **Multi-cluster managed resources view**: Console aggregates resources from hub (direct K8s API query) and spoke clusters (agent push-based reporting)
- **Cluster filtering**: Frontend provides cluster-based filtering with color-coded badges (hub/east/west)
- **Agent reporting protocol**: Spoke agents POST status + managed resource inventory to hub console at configurable intervals

#### Observability Integration
- **OpenTelemetry (OTLP gRPC)**: Primary metrics source with lowest latency
- **Thanos Querier**: Built-in OpenShift federated metrics (PromQL over HTTPS)
- **Prometheus fallback**: Single-cluster fallback for environments without Thanos/OTel
- **Console Observability tab**: Real-time connection status, active targets, pipeline health

#### AI Integration (Optional)
- **OpenAI-compatible API**: Works with vLLM, LiteLLM, KServe, Ollama — any model
- **Supported models**: DeepSeek R1, Granite 3B, Qwen 14B, LLaMA 3.1
- **Autopilot mode**: AI recommendations applied automatically within rate limits
- **Supervised mode**: AI recommendations logged for human review before application
- **TLS configuration**: Full verify, custom CA, or insecureSkipVerify for disconnected environments

#### Governance Console
- **PatternFly 5 dashboard**: Dark-mode, responsive multi-cluster governance UI
- **Dashboard**: Cluster status overview, operator health, recent events
- **Agents view**: All agents across clusters with mode/phase/watched resources
- **Events view**: Paginated event log with action-type filtering
- **Managed Resources view**: Multi-cluster resource table with cluster filter, pagination, CPU/memory display
- **Observability view**: Thanos + OTel connection status, active targets, pipeline health
- **Favicon**: Custom Kairos icon

#### Namespace Discovery
- **Suffix pattern**: `namespaceSuffix: "-prod"` discovers ALL matching namespaces dynamically
- **Explicit list**: `namespaces: [ns1, ns2]` for targeted management
- **Combined mode**: Both suffix + explicit list simultaneously
- **Label selector**: Additional `labelSelector` filtering within namespaces

#### Safety Mechanisms
- **Rate limiting**: `maxActionsPerHour` caps corrections per agent
- **Cooldown period**: Minimum time between actions on same workload
- **Rollback on failure**: Automatic revert if pods crash after resource change
- **Dry-run mode**: Evaluate policies without applying changes

#### Infrastructure
- **Red Hat UBI9 containers**: All images built on official Red Hat Universal Base Images
- **Dual registry**: Published to both Quay.io and GitHub Container Registry (GHCR)
- **GitHub Actions CI/CD**: Automated testing (unit + e2e), linting (golangci-lint), building, pushing
- **GitHub Pages documentation**: PatternFly-styled documentation site with product focus
- **OLM Bundle**: ClusterServiceVersion for OpenShift OperatorHub visibility

### Container Images

| Image | Registry |
|---|---|
| `kairos-operator:v1.0.0` | quay.io/maximilianopizarro, ghcr.io/maximilianopizarro |
| `kairos-console:v0.1.7` | quay.io/maximilianopizarro |

### Known Limitations

- Console in-memory store resets on pod restart (spoke agent data is lost until next report)
- AI model responses are not cached between reconcile cycles
- No persistent storage for event history (events are ephemeral)
- Multi-cluster resource view depends on spoke agents actively reporting (no pull-based query of remote clusters yet)

### Documentation

- GitHub Pages: https://maximilianoPizarro.github.io/kairos
- Architecture: [ARCHITECTURE.md](ARCHITECTURE.md)
- Contributing: [CONTRIBUTING.md](CONTRIBUTING.md)
