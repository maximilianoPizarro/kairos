# Kairos Console v2.2.0

Live screenshots are published on GitHub Pages:

**https://maximilianoPizarro.github.io/kairos/** → tab **Console v2.2.0**

## What is shown

Only screens with real cluster data (captured on OpenShift local / CRC):

| Screen | File |
|---|---|
| Dashboard (charts + status) | `docs/images/screenshots/01-dashboard.png` |
| AI Agents (`Granite-Vision-3.2`) | `docs/images/screenshots/02-agents.png` |
| Scaling Policies | `docs/images/screenshots/03-policies.png` |
| Rules expanded | `docs/images/screenshots/03b-policies-rules-expanded.png` |
| Managed Resources | `docs/images/screenshots/06-resources.png` |

## Intentionally omitted

These views were empty or disconnected on the capture cluster and are **not** published as product screenshots:

- **Observability** — Thanos / monitoring not wired on CRC (`Disconnected`)
- **Approvals / History / Diff View** — no pending `KairosEvent` approvals or applied diffs yet

They will be added once a scenario with real events/approvals and monitoring is available.

## Environment used for capture

- Operator / Console: `v2.2.0`
- 1× `KairosAgent` (`hub-agent`)
- 2× `SmartScalingPolicy`
- Demo fleet in `kairos-demo` (Deployments with `kairos.io/managed: "true"`)

## Configure an AI agent (credentials stay local)

```bash
export LITELLM_URL='https://<your-openai-compatible-endpoint>/v1'
export LITELLM_API_KEY='<your-api-key>'

oc create secret generic kairos-ai-credentials -n kairos-system \
  --from-literal=api-key="$LITELLM_API_KEY"

# Edit sample apiURL, then:
oc apply -f config/samples/kairos_v1alpha1_kairosagent.yaml
```
