#!/usr/bin/env bash
# Apply demo scenarios for Kairos v2.2.0 into kairos-demo (workloads/CRs only).
# Operator/console remain in kairos-system (OLM).
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
DEMO_NS="${KAIROS_DEMO_NS:-kairos-demo}"
OP_NS="${KAIROS_NS:-kairos-system}"

echo "Applying scenarios into ${DEMO_NS} (operator namespace: ${OP_NS})"
oc apply -f "$DIR/00-namespace.yaml"
oc apply -f "$DIR/01-demo-app.yaml"
oc apply -f "$DIR/02-demo-fleet.yaml"
oc apply -f "$DIR/03-hub-agent.yaml"
oc apply -f "$DIR/04-demo-scaling-policy.yaml"
oc apply -f "$DIR/05-demo-fleet-policy.yaml"
oc apply -f "$DIR/06-kairos-events.yaml"
oc apply -f "$DIR/07-screenshot-events.yaml"

# Ensure demo-svc-1 has 128Mi so increase_resources approvals are visible
oc set resources deploy/demo-svc-1 -n "$DEMO_NS" \
  --requests=cpu=50m,memory=128Mi --limits=cpu=50m,memory=128Mi >/dev/null 2>&1 || true

echo
echo "Scenario CRs in ${DEMO_NS}:"
oc get kairosagent,smartscalingpolicy,kairosevent,deploy -n "$DEMO_NS" --no-headers 2>/dev/null | head -40 || true
echo
echo "Next:"
echo "  oc port-forward -n $OP_NS svc/kairos-console 8181:8080"
echo "  KAIROS_DEMO_NS=$DEMO_NS $DIR/verify.sh"
echo "  Console Route: oc get route -n $OP_NS"
