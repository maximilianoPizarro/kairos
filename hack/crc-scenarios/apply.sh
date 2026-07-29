#!/usr/bin/env bash
# Apply CRC demo scenarios for Kairos v2.2.0 (no credentials).
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
NS="${KAIROS_NS:-kairos-system}"

echo "Applying scenarios from $DIR"
oc apply -f "$DIR/00-namespace.yaml"
oc apply -f "$DIR/01-demo-app.yaml"
oc apply -f "$DIR/02-demo-fleet.yaml"
oc apply -f "$DIR/03-hub-agent.yaml"
oc apply -f "$DIR/04-demo-scaling-policy.yaml"
oc apply -f "$DIR/05-demo-fleet-policy.yaml"
oc apply -f "$DIR/06-kairos-events.yaml"
oc apply -f "$DIR/07-screenshot-events.yaml"

# Pin console to harden digest if tag cache is stale on CRC
DIGEST="${KAIROS_CONSOLE_DIGEST:-sha256:96a4d448ddd2bfdd396353b4930c7916b6b006b428d054191eca10070cb9e1e9}"
if oc get kairosconsole kairos -n "$NS" >/dev/null 2>&1; then
  oc patch kairosconsole kairos -n "$NS" --type=merge \
    -p "{\"spec\":{\"image\":\"quay.io/maximilianopizarro/kairos-console@${DIGEST}\"}}"
  echo "Pinned KairosConsole image to @$DIGEST"
fi

echo
echo "Next:"
echo "  oc port-forward -n $NS svc/kairos-console 8181:8080"
echo "  $DIR/verify.sh"
echo "  $DIR/capture-screenshots.sh   # optional: refresh docs screenshots"
echo "  Console: https://kairos-console-${NS}.apps-crc.testing"
