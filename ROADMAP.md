# Kairos Operator — Roadmap

## v1.1.0 (Current Release — All Backlog Consolidated)

All items from Sprint 2, Sprint 3, and Enterprise Features have been unified into this release.

### Resource Management
- [x] Override manual (`pinnedUntil` annotation for SRE lockdowns)
- [x] DaemonSet + CronJob resource type support
- [x] HPA/KEDA coexistence logic (detect and defer)
- [x] SSA conflict resolution (coexistence with HPA, KEDA, VPA documented)
- [x] Scheduled scaling windows (CronJob-like resource profiles via `scalingWindows`)

### Observability & Audit
- [x] `KairosEvent` CRD for decision audit history
- [x] Grafana dashboard JSON published in `/config/grafana/`
- [x] Operator self-observability (`/metrics` endpoint + Prometheus ServiceMonitor)
- [x] Dry-run mode documented with CRD field + visible recommendations in status

### Validation & Safety
- [x] ValidatingWebhookConfiguration for pre-apply validation
- [x] RBAC granular documentation (minimum roles per spoke agent)

### Enterprise Features
- [x] Multi-tenancy: namespace-scoped operator installation (via `scope` field)
- [x] Policy inheritance (global > cluster > namespace with priority)
- [x] Cost estimation integration (cloud provider pricing APIs)
- [x] Notification webhook (Slack/Teams/generic) on corrections
- [x] Console: approval workflow UI (click to approve/reject)
- [x] Console: historical charts (resource changes over time)
- [x] Console: diff view (before/after correction comparison)

### Developer Experience
- [x] Quick Start with `kind` + setup script (`hack/quickstart.sh`)
- [x] Generic documentation (not tied to specific demo environments)
- [x] Landing page improvements (real counters, CRD relationship diagram)
- [x] ROADMAP.md published

---

## Future: v2.0.0 (Vision)

### Planned
- [ ] Federated ML model: on-cluster inference for scaling decisions
- [ ] GitOps native: generate PRs instead of direct SSA patches
- [ ] FinOps dashboard: real-time cost per namespace/team
- [ ] Custom metric adapters (Datadog, New Relic, Splunk)
- [ ] Multi-cluster policy sync via Open Cluster Management
- [ ] Operator Lifecycle Manager (OLM) catalog publication
- [ ] Red Hat certification and OperatorHub listing
- [ ] Horizontal Pod Autoscaler integration (Kairos as HPA advisor)
- [ ] Predictive scaling: time-series forecasting for proactive resource adjustment
- [ ] SLA-aware scheduling: integrate with cluster priority classes
