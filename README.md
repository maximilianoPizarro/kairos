# Kairos Operator

<div align="center">
  <img src="docs/images/kairos-logo.svg" alt="Kairos Logo" width="150">
  
  ### καιρός — *The Opportune Moment*
  
  > *"Kairos is the fleeting moment of opportunity — the instant when conditions align perfectly for action. In infrastructure, it's the precise moment to scale before latency spikes, and to release resources before waste accumulates."*

  [![CI](https://github.com/maximilianoPizarro/kairos/actions/workflows/ci.yaml/badge.svg)](https://github.com/maximilianoPizarro/kairos/actions/workflows/ci.yaml)
  [![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
  [![Operator SDK](https://img.shields.io/badge/Operator%20SDK-v1.40.0-red.svg)](https://sdk.operatorframework.io/)
</div>

---

OpenShift operator for intelligent resource management with OpenTelemetry metrics and optional AI-powered autopilot. Designed to coexist with ArgoCD in multi-cluster GitOps environments.

## Architecture

![Kairos Architecture](docs/images/architecture-overview.svg)

## Scaling Flow

![Scaling Flow](docs/images/scaling-flow.svg)

## Features

- **Smart Scaling Policies** - Define metric-based and schedule-based scaling rules as Kubernetes CRDs
- **AI-Powered Agents** - Optional integration with AI models (Granite, Nemotron, Qwen via vLLM/KServe) for autonomous resource optimization
- **OpenTelemetry Native** - Reads metrics directly from OTel Collector via gRPC, with Prometheus fallback
- **ArgoCD Compatible** - Uses Server-Side Apply to manage only the `resources` field without causing sync conflicts
- **Namespace Suffix Filtering** - Watch all namespaces ending with `-dev`, `-test`, `-qa`, `-prod` automatically
- **Multi-cluster Governance Console** - PatternFly 5 web UI for visualizing agents, policies, and events across clusters
- **Safe by Design** - Rate limiting, cooldowns, approval workflows, and rollback on failure

## Namespace Suffix Mode

Instead of listing namespaces manually, define a suffix pattern:

```yaml
apiVersion: kairos.maximilianopizarro.github.io/v1alpha1
kind: KairosAgent
metadata:
  name: agent-production
spec:
  mode: supervised
  watch:
    namespaceSuffix: "-prod"   # Watches ALL *-prod namespaces automatically
    resourceTypes:
    - Deployment
    - StatefulSet
```

Available suffix patterns: `-dev`, `-test`, `-qa`, `-prod` (or any custom suffix).

## Quick Start

### Prerequisites

- OpenShift 4.17+ or Kubernetes 1.25+
- Cluster admin access
- (Optional) OTel Collector deployed
- (Optional) AI model endpoint (vLLM, KServe)

### Install

```bash
# 1. Install CRDs
kubectl apply -f config/crd/bases/

# 2. Create namespace, ServiceAccount, and RBAC
kubectl apply -f config/samples/prereqs.yaml

# 3. Deploy the operator
make deploy IMG=quay.io/maximilianopizarro/kairos-operator:v0.1.0

# 4. Create a scaling policy
kubectl apply -f config/samples/kairos_v1alpha1_smartscalingpolicy.yaml

# 5. (Optional) Deploy AI agent
kubectl apply -f config/samples/kairos_v1alpha1_kairosagent.yaml

# 6. Deploy the governance console
kubectl apply -f config/samples/kairos_v1alpha1_kairosconsole.yaml
```

### Mark a workload for Kairos management

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

## ArgoCD Coexistence

See [docs/argocd-coexistence.md](docs/argocd-coexistence.md) for the complete guide.

**TL;DR:** Leave `resources: {}` empty in your Git manifests, add `kairos.io/managed: "true"` annotation, and configure `ignoreDifferences` in ArgoCD.

## Container Images

| Image | Purpose | Base |
|---|---|---|
| `quay.io/maximilianopizarro/kairos-operator` | Controller manager | UBI9 Micro |
| `quay.io/maximilianopizarro/kairos-console` | Governance dashboard | UBI9 Micro |
| `quay.io/maximilianopizarro/kairos-operator-bundle` | OLM bundle | UBI9 Micro |
| `quay.io/maximilianopizarro/kairos-operator-catalog` | OLM catalog | OPM |

## Development

### Build locally

```bash
# Operator
make docker-build IMG=quay.io/maximilianopizarro/kairos-operator:v0.1.0
make docker-push IMG=quay.io/maximilianopizarro/kairos-operator:v0.1.0

# Console
make docker-build-console
make docker-push-console

# Everything
make release
```

### Run tests

```bash
make test
```

### Generate manifests after type changes

```bash
make generate manifests
```

## CRDs

| Kind | Purpose | Key Fields |
|---|---|---|
| `SmartScalingPolicy` | Metric & schedule scaling rules | target, rules, schedule, ai, paused |
| `KairosAgent` | AI autonomous optimization agent | mode, aiModel, watch.namespaceSuffix, correctionPolicy |
| `KairosConsole` | Multi-cluster governance console | route, auth, clusters |

## Integration with platform-hub-spoke-config

Kairos integrates as a component in the [platform-hub-spoke-config](https://github.com/maximilianoPizarro/platform-hub-spoke-config) Helm chart at sync wave 7, namespace `kairos-system`.

## Documentation

Full documentation: https://maximilianoPizarro.github.io/kairos

## GitHub Actions Variables

Required secrets for CI/CD:

| Secret | Purpose |
|---|---|
| `QUAY_USERNAME` | Quay.io registry username |
| `QUAY_PASSWORD` | Quay.io registry password/token |

## License

Apache License 2.0
