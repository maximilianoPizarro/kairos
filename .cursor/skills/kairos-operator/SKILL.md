# Kairos Operator Skill

Use this skill when working on the Kairos operator codebase. It provides context about architecture, patterns, and conventions.

## Project Overview

Kairos is a Go-native OpenShift operator built with Operator SDK/Kubebuilder that manages intelligent resource scaling with optional AI integration. It coexists with ArgoCD using Server-Side Apply.

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

### CRDs

| CRD | Purpose |
|---|---|
| `SmartScalingPolicy` | Metric-based and schedule-based scaling rules |
| `KairosAgent` | AI agent for autonomous corrections |
| `KairosConsole` | Deploys the governance web console |

## Directory Structure

```
api/v1alpha1/           - CRD type definitions
internal/controller/    - Reconcilers for each CRD
internal/metrics/       - OTel and Prometheus client
internal/ai/            - OpenAI-compatible AI client
internal/scaler/        - Scaling coordinator with SSA
console/                - Web console (Go backend + React frontend)
config/                 - Kustomize manifests (CRDs, RBAC, manager)
bundle/                 - OLM bundle for OperatorHub
```

## Build Commands

```bash
make docker-build IMG=quay.io/maximilianopizarro/kairos-operator:v0.1.0
make docker-push IMG=quay.io/maximilianopizarro/kairos-operator:v0.1.0
make docker-build-console
make docker-push-console
make release            # builds and pushes everything
make generate manifests # regenerate CRDs after type changes
```

## Container Images (Red Hat UBI9)

All images use official Red Hat base images:
- Builder: `registry.access.redhat.com/ubi9/go-toolset:1.22`
- Frontend builder: `registry.access.redhat.com/ubi9/nodejs-20:latest`
- Runtime: `registry.access.redhat.com/ubi9/ubi-micro:latest`

Published to: `quay.io/maximilianopizarro/kairos-*`

## Key Annotations

| Annotation | Purpose |
|---|---|
| `kairos.io/managed` | Must be "true" for operator to touch a resource |
| `kairos.io/policy` | Links resource to a SmartScalingPolicy |
| `kairos.io/last-action` | Records last scaling action |
| `kairos.io/last-action-time` | Cooldown tracking |

## AI Integration

Optional connection to OpenAI-compatible APIs (vLLM, KServe, Ollama, OpenAI). The AI client:
- Sends infrastructure context (metrics, current state)
- Receives scaling recommendations
- Operates in `autopilot` (auto-apply) or `supervised` (require approval) mode

## Testing with platform-hub-spoke-config

The operator installs in the hub cluster at sync wave 7 in namespace `kairos-system`. SmartScalingPolicies target workloads in spoke clusters that report metrics to the hub's OTel Collector.
