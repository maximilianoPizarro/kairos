# Kairos Console

Web UI and API for multi-cluster Kairos (hub aggregates spoke agent-report traffic).

## Deployment

- **Hub only:** one `KairosConsole` CR on the ACM hub. Spokes should not run a second console Route.
- **Hub URL:** `https://kairos-console-kairos-system.<hub-apps-domain>`.

## API behaviour

| Endpoint | Source |
| -------- | ------ |
| `GET /api/v1/policies` | `SmartScalingPolicy` in `kairos-system` on the local cluster, or `[]`. Demo row only if `KAIROS_CONSOLE_DEMO_DATA=true`. |
| `GET /api/v1/approvals`, `/api/v1/history` | Demo/workshop data until wired to the operator. |

Platform sensor policies are on **spoke** clusters; the hub list is empty unless CRs exist on the hub.
