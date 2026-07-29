# Kairos Operator — Roadmap

## Current: v2.2.0 (Released)

### Scaling safety
- [x] Replace stub metric evaluator with real Prometheus/Thanos queries
- [x] Policy-wide cooldown (no alternating-rule bypass)
- [x] Skip metric rules when endpoint unavailable (safe default)
- [x] HTTPS + in-cluster SA bearer for OpenShift monitoring endpoints
- [x] Honor `when.for` duration before firing metric rules

### Console
- [x] Expandable SmartScalingPolicy rule details
- [x] Agents/clusters from real CRs (no hardcoded deepseek / 3 clusters)
- [x] Dashboard PatternFly charts (`/api/v1/stats`)
- [x] Demo data gated behind `KAIROS_CONSOLE_DEMO_DATA=true`
- [x] GitHub Pages Console v2.2.0 screenshot gallery (real-data screens only)

### Bundle / OperatorHub
- [x] `replaces: kairos-operator.v2.1.1`
- [x] Trim CSV description to shipped capabilities
- [x] OpenShift versions annotation `v4.12-v4.22`
- [x] No `route.openshift.io` in `nativeAPIs`

---

## Next

- [ ] FBC artifacts for community-operators-prod (`2.2.0/` + semver)
- [ ] Wire Approvals / History / Diff View screenshots once KairosEvents exist
- [ ] Observability connected screenshot (Thanos + `cluster-monitoring-view`)
- [ ] Deeper unit/e2e coverage for console handlers

---

## Previous: v2.1.x

- [x] Real Diff View / Approvals / History from KairosEvent CRs
- [x] SSA merge-patch fix (`spec.selector` corruption)
- [x] Console RBAC (`kairos-console-reader`, monitoring CRB non-fatal)

## Previous: v2.0.x

- [x] Console policies API from cluster
- [x] OperatorHub bundle structure
- [x] Multi-cluster hub-spoke console
- [x] Vulnerability remediation (grpc/otel/oauth2)
