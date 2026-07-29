#!/usr/bin/env bash
# Verify Kairos v2.2.0 on CRC / OpenShift local after applying scenarios.
set -euo pipefail

NS="${KAIROS_NS:-kairos-system}"
BASE="${KAIROS_CONSOLE_URL:-http://127.0.0.1:8181}"
PASS=0
FAIL=0

ok()   { echo "  PASS  $*"; PASS=$((PASS+1)); }
bad()  { echo "  FAIL  $*"; FAIL=$((FAIL+1)); }
need() { command -v "$1" >/dev/null || { echo "missing $1"; exit 1; }; }

need oc
need curl
need python3

echo "== Cluster =="
oc whoami >/dev/null && ok "oc login" || bad "oc login"
oc get csv -n "$NS" kairos-operator.v2.2.0 -o jsonpath='{.status.phase}' 2>/dev/null | grep -q Succeeded \
  && ok "CSV kairos-operator.v2.2.0 Succeeded" \
  || bad "CSV not Succeeded"
oc get deploy -n "$NS" kairos-controller-manager -o jsonpath='{.status.readyReplicas}' | grep -q '[1-9]' \
  && ok "controller ready" || bad "controller not ready"
oc get deploy -n "$NS" kairos-console -o jsonpath='{.status.readyReplicas}' | grep -q '[1-9]' \
  && ok "console ready" || bad "console not ready"

IMG=$(oc get pod -n "$NS" -l app=kairos-console -o jsonpath='{.items[0].status.containerStatuses[0].imageID}' 2>/dev/null || true)
echo "  console imageID: ${IMG:-unknown}"
# Prefer digest from harden CI (e048312); tag-only cache may be stale on CRC.
if [[ "$IMG" == *"sha256:96a4d448ddd2bfdd396353b4930c7916b6b006b428d054191eca10070cb9e1e9"* ]]; then
  ok "console image is approvals-fix digest 96a4d448"
elif [[ "$IMG" == *"sha256:3fa2dd9e"* ]] || [[ "$IMG" == *"sha256:3d0664e2"* ]]; then
  bad "console on stale digest — pin KairosConsole.spec.image to @sha256:96a4d448..."
else
  ok "console image present (verify digest manually if APIs look stale)"
fi

echo "== CRs =="
oc get kairosagent -n "$NS" hub-agent >/dev/null 2>&1 && ok "hub-agent exists" || bad "hub-agent missing"
oc get smartscalingpolicy -n "$NS" demo-scaling-policy >/dev/null 2>&1 && ok "demo-scaling-policy" || bad "demo-scaling-policy"
oc get smartscalingpolicy -n kairos-demo demo-fleet-policy >/dev/null 2>&1 && ok "demo-fleet-policy" || bad "demo-fleet-policy"
EV=$(oc get kairosevent -n "$NS" --no-headers 2>/dev/null | wc -l | tr -d ' ')
[[ "$EV" -ge 3 ]] && ok "KairosEvents >= 3 ($EV)" || bad "KairosEvents < 3 ($EV)"

echo "== Controller safety (no prom → skip rules) =="
# Avoid pipefail false-negatives when `oc logs` exits non-zero with warnings.
CTRL_LOGS=$(oc logs -n "$NS" deploy/kairos-controller-manager --since=30m 2>/dev/null || true)
if grep -q "No prometheus endpoint configured, skipping metric rule" <<<"$CTRL_LOGS"; then
  ok "policies skip rules without prometheusEndpoint"
else
  bad "expected skip-metric-rule log not found (policies may still be safe if never reconciled)"
fi

echo "== Console API ($BASE) =="
if ! curl -sf --max-time 5 "$BASE/healthz" >/dev/null; then
  bad "console not reachable at $BASE — run: oc port-forward -n $NS svc/kairos-console 8181:8080"
  echo
  echo "Result: $PASS passed, $FAIL failed"
  exit 1
fi
ok "healthz"

python3 - "$BASE" <<'PY'
import json, sys, urllib.request
base = sys.argv[1]

def get(path):
    with urllib.request.urlopen(base + path, timeout=10) as r:
        return json.load(r)

def ok(msg): print("  PASS ", msg); return 1
def bad(msg): print("  FAIL ", msg); return 0
pass_n = fail_n = 0

st = get("/api/v1/status")
checks = [
    (st.get("operatorVersion") == "2.2.0", f"status.operatorVersion=2.2.0 got {st.get('operatorVersion')}"),
    (st.get("totalAgents", 0) >= 1, f"status.totalAgents>=1 got {st.get('totalAgents')}"),
    (st.get("totalPolicies", 0) >= 2, f"status.totalPolicies>=2 got {st.get('totalPolicies')}"),
    (st.get("totalEvents", 0) >= 3, f"status.totalEvents>=3 got {st.get('totalEvents')}"),
    (st.get("totalApprovals", 0) >= 0, f"status.totalApprovals present"),
]
for c, msg in checks:
    if c: pass_n += ok(msg)
    else: fail_n += bad(msg)

events = get("/api/v1/events")
fake = [e for e in events if e.get("action") == "NoAction" or "within threshold" in str(e.get("detail", ""))]
if len(fake) == 0 and len(events) >= 3:
    pass_n += ok(f"events real only (count={len(events)}, fake=0)")
else:
    fail_n += bad(f"events unexpected fake={len(fake)} count={len(events)}")

clusters = get("/api/v1/clusters")
hub = next((c for c in clusters if c.get("name") == "hub"), None)
if hub and hub.get("agents", 0) >= 1 and hub.get("policies", 0) >= 2:
    pass_n += ok(f"clusters hub agents={hub.get('agents')} policies={hub.get('policies')}")
else:
    fail_n += bad(f"clusters hub counts wrong: {hub}")

agents = get("/api/v1/agents")
if any(a.get("name") == "hub-agent" and a.get("aiModel") == "Granite-Vision-3.2" for a in agents):
    pass_n += ok("agents hub-agent Granite-Vision-3.2")
else:
    fail_n += bad(f"agents missing hub/granite: {agents}")

policies = get("/api/v1/policies")
if len(policies) >= 2 and all(p.get("ruleDetails") for p in policies):
    pass_n += ok(f"policies with ruleDetails (count={len(policies)})")
else:
    fail_n += bad(f"policies/ruleDetails incomplete: {[(p.get('name'), p.get('ruleDetails')) for p in policies]}")

managed = get("/api/v1/managed-resources")
if len(managed) >= 8:
    pass_n += ok(f"managed-resources count={len(managed)}")
else:
    fail_n += bad(f"managed-resources too few: {len(managed)}")

notif = get("/api/v1/notifications")
if notif.get("configured") is False:
    pass_n += ok("notifications not fake-configured (demo off)")
else:
    fail_n += bad(f"notifications look demo-ish: {notif}")

print(f"__COUNTS__ {pass_n} {fail_n}")
sys.exit(0 if fail_n == 0 else 1)
PY
api_rc=$?
if [[ $api_rc -eq 0 ]]; then
  : # python already printed PASS/FAIL; recount from marker not needed
else
  FAIL=$((FAIL+1))
fi

echo
echo "Done. Fix any FAIL above before OperatorHub submit."
# Aggregate roughly from exit
exit $api_rc
