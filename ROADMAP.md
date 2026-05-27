# Kairos Operator — Roadmap

## Current: v1.0.x (Stability & Adoption)

### v1.0.1 — Sprint 1 (Current)
- [x] Dry-run mode documented with CRD field + visible recommendations in status
- [x] RBAC granular documentation (minimum roles per spoke agent)
- [x] Operator self-observability (`/metrics` endpoint + Grafana dashboard reference)
- [x] Generic documentation (not tied to specific demo environments)
- [x] Landing page improvements (real counters, CRD relationship diagram)
- [x] ROADMAP.md published

### v1.0.2 — Sprint 2
- [ ] Override manual (`pinnedUntil` annotation for SRE lockdowns)
- [ ] DaemonSet + CronJob resource type support
- [ ] Grafana dashboard JSON published in `/config/grafana/`
- [ ] SSA conflict documentation (coexistence with HPA, KEDA, VPA)

### v1.0.3 — Sprint 3
- [ ] `KairosEvent` CRD for decision audit history
- [ ] Quick Start with `kind` + setup script (no real cluster needed)
- [ ] ValidatingWebhookConfiguration for pre-apply validation
- [ ] HPA/KEDA coexistence logic (detect and defer)

---

## Next: v1.1.0 (Enterprise Features)

### Planned
- [ ] Multi-tenancy: namespace-scoped operator installation
- [ ] Policy inheritance (global → cluster → namespace)
- [ ] Scheduled scaling windows (CronJob-like resource profiles)
- [ ] Cost estimation integration (cloud provider pricing APIs)
- [ ] Slack/Teams notification webhook on corrections
- [ ] Console: approval workflow UI (click to approve/reject)
- [ ] Console: historical charts (resource usage over time)
- [ ] Console: diff view (before/after correction comparison)

---

## Future: v2.0.0 (AI-Native)

### Vision
- [ ] Fine-tuned model for resource prediction (trained on cluster patterns)
- [ ] Predictive scaling (scale before load spikes based on cron patterns)
- [ ] Cross-workload correlation (detect cascade failures)
- [ ] FinOps integration (right-size for cost, not just performance)
- [ ] OLM catalog publication (OperatorHub.io listing)
- [ ] Helm chart distribution
- [ ] Multi-arch images (amd64 + arm64)

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for how to pick up items from this roadmap.
Priority is indicated by sprint order — earlier sprints are higher priority.
