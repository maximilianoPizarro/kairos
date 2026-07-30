#!/usr/bin/env bash
# Interactive approval scenario against a running console (port-forward or Route).
# Workloads + KairosEvents live in kairos-demo; console stays in kairos-system.
set -euo pipefail

BASE="${KAIROS_CONSOLE_URL:-http://127.0.0.1:8181}"
DEMO_NS="${KAIROS_DEMO_NS:-kairos-demo}"

echo "== Seed pending approval event (ns=${DEMO_NS}) =="
oc apply -f - <<EOF
apiVersion: kairos.maximilianopizarro.github.io/v1alpha1
kind: KairosEvent
metadata:
  name: evt-scenario-pending
  namespace: ${DEMO_NS}
spec:
  agentName: hub-agent
  cluster: hub
  action: scale_up
  resource: demo-app
  namespace: ${DEMO_NS}
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
AID=$(oc get kairosevent evt-scenario-pending -n "$DEMO_NS" -o jsonpath='{.metadata.uid}')
echo "approval id=$AID"
test -n "$AID"

echo "== Reject =="
curl -sf -X POST "$BASE/api/v1/approvals/$AID/reject" | python3 -m json.tool
STATUS=$(oc get kairosevent evt-scenario-pending -n "$DEMO_NS" -o jsonpath='{.spec.status}')
echo "CR status=$STATUS"
[[ "$STATUS" == "rejected" ]]

echo "== Re-seed and approve (operator applies → applied + scale) =="
oc scale deploy/demo-app -n "$DEMO_NS" --replicas=2 >/dev/null
oc delete kairosevent evt-scenario-pending -n "$DEMO_NS" --wait=false --ignore-not-found
oc apply -f - <<EOF
apiVersion: kairos.maximilianopizarro.github.io/v1alpha1
kind: KairosEvent
metadata:
  name: evt-scenario-pending
  namespace: ${DEMO_NS}
spec:
  agentName: hub-agent
  cluster: hub
  action: scale_up
  resource: demo-app
  namespace: ${DEMO_NS}
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
AID=$(oc get kairosevent evt-scenario-pending -n "$DEMO_NS" -o jsonpath='{.metadata.uid}')
curl -sf -X POST "$BASE/api/v1/approvals/$AID/approve" | python3 -m json.tool

STATUS=""
for i in $(seq 1 30); do
  STATUS=$(oc get kairosevent evt-scenario-pending -n "$DEMO_NS" -o jsonpath='{.spec.status}')
  echo "  wait apply status=$STATUS ($i)"
  [[ "$STATUS" == "applied" || "$STATUS" == "failed" ]] && break
  sleep 2
done
echo "CR status=$STATUS"
[[ "$STATUS" == "applied" ]]

REPLICAS=$(oc get deploy demo-app -n "$DEMO_NS" -o jsonpath='{.spec.replicas}')
echo "demo-app replicas=$REPLICAS"
[[ "$REPLICAS" == "3" ]]
echo "PASS approval scenarios"
