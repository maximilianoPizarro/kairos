#!/usr/bin/env bash
# Interactive approval scenario against a running console (port-forward or Route).
set -euo pipefail

BASE="${KAIROS_CONSOLE_URL:-http://127.0.0.1:8181}"
NS="${KAIROS_NS:-kairos-system}"

echo "== Seed pending approval event =="
oc apply -f - <<EOF
apiVersion: kairos.maximilianopizarro.github.io/v1alpha1
kind: KairosEvent
metadata:
  name: evt-scenario-pending
  namespace: ${NS}
spec:
  agentName: hub-agent
  cluster: hub
  action: scale_up
  resource: demo-app
  namespace: ${NS}
  policyName: demo-scaling-policy
  reason: "CRC scenario: pending approval"
  status: pending-approval
  dryRun: false
  before:
    cpu: "100m"
    memory: "128Mi"
    replicas: "2"
  after:
    cpu: "100m"
    memory: "128Mi"
    replicas: "3"
EOF

sleep 1
AID=$(curl -sf "$BASE/api/v1/approvals" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(next((a["id"] for a in d if a.get("resource")=="demo-app"), d[0]["id"] if d else ""))')
echo "approval id=$AID"
test -n "$AID"

echo "== Reject =="
curl -sf -X POST "$BASE/api/v1/approvals/$AID/reject" | python3 -m json.tool
STATUS=$(oc get kairosevent evt-scenario-pending -n "$NS" -o jsonpath='{.spec.status}')
echo "CR status=$STATUS"
[[ "$STATUS" == "rejected" ]]

echo "== Re-seed and approve =="
oc delete kairosevent evt-scenario-pending -n "$NS" --wait=false
oc apply -f - <<EOF
apiVersion: kairos.maximilianopizarro.github.io/v1alpha1
kind: KairosEvent
metadata:
  name: evt-scenario-pending
  namespace: ${NS}
spec:
  agentName: hub-agent
  cluster: hub
  action: scale_up
  resource: demo-app
  namespace: ${NS}
  policyName: demo-scaling-policy
  reason: "CRC scenario: approve path"
  status: pending-approval
  dryRun: false
  before:
    cpu: "100m"
    memory: "128Mi"
    replicas: "2"
  after:
    cpu: "100m"
    memory: "128Mi"
    replicas: "3"
EOF
sleep 1
AID=$(curl -sf "$BASE/api/v1/approvals" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d[0]["id"] if d else "")')
curl -sf -X POST "$BASE/api/v1/approvals/$AID/approve" | python3 -m json.tool
STATUS=$(oc get kairosevent evt-scenario-pending -n "$NS" -o jsonpath='{.spec.status}')
echo "CR status=$STATUS"
[[ "$STATUS" == "applied" ]]
echo "PASS approval scenarios"
