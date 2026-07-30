# Kairos Console v2.2.0

Live screenshots are published on GitHub Pages:

**https://maximilianoPizarro.github.io/kairos/** → tab **Console v2.2.0**

## What is shown

Captured on OpenShift local (CRC) with the `hack/crc-scenarios` demo (real CRs, no mock console data):

| Screen | File |
|---|---|
| Dashboard (charts + status) | `docs/images/screenshots/01-dashboard.png` |
| AI Agents (`Granite-Vision-3.2`) | `docs/images/screenshots/02-agents.png` |
| Scaling Policies | `docs/images/screenshots/03-policies.png` |
| Rules expanded | `docs/images/screenshots/03b-policies-rules-expanded.png` |
| Events | `docs/images/screenshots/04-events.png` |
| Managed Resources | `docs/images/screenshots/06-resources.png` |
| Approvals (supervised) | `docs/images/screenshots/07-approvals.png` |
| History | `docs/images/screenshots/08-history.png` |
| Diff View | `docs/images/screenshots/09-diffview.png` |

## Intentionally omitted

- **Observability** — requires OpenShift `cluster-monitoring-view` bindings (see below). CRC often lacks full monitoring; CI/OCP clusters should bind console + controller SAs.

## OpenShift monitoring (Thanos)

```bash
oc create clusterrolebinding kairos-console-monitoring \
  --clusterrole=cluster-monitoring-view \
  --serviceaccount=kairos-system:kairos-console

oc create clusterrolebinding kairos-controller-monitoring \
  --clusterrole=cluster-monitoring-view \
  --serviceaccount=kairos-system:kairos-controller-manager
```

For SmartScalingPolicy PromQL against Thanos HTTPS, set `spec.metricsTLS.insecureSkipVerify: true`
(or `caSecretRef` with the service CA).

## Re-capture

```bash
oc port-forward -n kairos-system svc/kairos-console 8181:8080
hack/crc-scenarios/apply.sh          # if scenario not already applied
hack/crc-scenarios/capture-screenshots.sh
```

## Environment used for capture

- Operator / Console: `v2.2.0`
- 1× `KairosAgent` (`hub-agent`, supervised)
- 2× `SmartScalingPolicy`
- Demo fleet in `kairos-demo` + `demo-app`
- KairosEvents covering pending-approval, applied, dry-run, rejected

## Configure an AI agent (credentials stay local)

```bash
export LITELLM_URL='https://<your-openai-compatible-endpoint>/v1'
export LITELLM_API_KEY='<your-api-key>'

oc create secret generic kairos-ai-credentials -n kairos-system \
  --from-literal=api-key="$LITELLM_API_KEY"

# Edit sample apiURL, then:
oc apply -f config/samples/kairos_v1alpha1_kairosagent.yaml
```
