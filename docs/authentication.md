# Console Authentication

The Kairos Console supports three authentication modes. By default, it runs **without authentication** for easy initial setup.

## Authentication Modes

| Mode | `spec.auth.type` | Description | When to Use |
|---|---|---|---|
| **None** | `none` | No authentication — open access | Development, initial setup, internal networks |
| **OpenShift OAuth** | `openshift-oauth` | Uses OpenShift's built-in OAuth server | Production on OpenShift (recommended) |
| **Token** | `token` | Static bearer token authentication | Air-gapped environments without OAuth |

## Mode 1: No Authentication (Default)

The console is accessible without any login. Ideal for first-time setup and development.

```yaml
apiVersion: kairos.maximilianopizarro.github.io/v1alpha1
kind: KairosConsole
metadata:
  name: kairos
  namespace: kairos-system
spec:
  replicas: 1
  route:
    enabled: true
    host: "kairos-console.apps.your-cluster.example.com"
    tlsEnabled: true
  auth:
    type: none
```

## Mode 2: OpenShift OAuth (Recommended for Production)

Uses the OpenShift OAuth proxy sidecar — the same pattern used by Kiali, Grafana, and other OpenShift-integrated UIs. Users authenticate via the OpenShift login page and must have the required cluster role.

### How It Works

```
┌─────────────────────────────────────────────────────────────┐
│                    Kairos Console Pod                         │
│                                                              │
│  ┌───────────────┐         ┌──────────────────────┐        │
│  │ oauth-proxy   │ ──────▶ │  console (Go+React)  │        │
│  │ :8443 (HTTPS) │ proxy   │  :8080 (HTTP)        │        │
│  └───────┬───────┘         └──────────────────────┘        │
│          │                                                   │
└──────────┼───────────────────────────────────────────────────┘
           │
           ▼
┌─────────────────────┐
│ OpenShift OAuth      │
│ Server               │
│ (built-in)           │
└─────────────────────┘
           │
           ▼
┌─────────────────────┐
│ User authenticates   │
│ with OpenShift login │
│ (cluster-admin role  │
│  required)           │
└─────────────────────┘
```

### Configuration

```yaml
apiVersion: kairos.maximilianopizarro.github.io/v1alpha1
kind: KairosConsole
metadata:
  name: kairos
  namespace: kairos-system
spec:
  replicas: 1
  image: "quay.io/maximilianopizarro/kairos-console:latest"
  route:
    enabled: true
    host: "kairos-console.apps.your-cluster.example.com"
    tlsEnabled: true
  auth:
    type: openshift-oauth
    oauth:
      # Role required to access the console (default: cluster-admin)
      requiredRole: "cluster-admin"
      # HTTPS port for the OAuth proxy (default: 8443)
      httpsPort: 8443
      # Optional: custom OAuth proxy image
      # image: "registry.redhat.io/openshift4/ose-oauth-proxy:latest"
      # Optional: pre-existing cookie secret name
      # cookieSecret: "my-custom-cookie-secret"
  clusters:
  - name: hub
    region: central
    apiURL: "https://api.hub-cluster:6443"
```

### What the Operator Creates Automatically

When `auth.type: openshift-oauth` is configured, the Kairos operator automatically:

1. **Annotates the ServiceAccount** with `serviceaccounts.openshift.io/oauth-redirectreference.kairos` so OpenShift registers it as an OAuth client
2. **Annotates the Service** with `service.beta.openshift.io/serving-cert-secret-name` to auto-generate a TLS certificate
3. **Creates a cookie secret** (`kairos-console-oauth-cookie`) for session encryption
4. **Adds an `oauth-proxy` sidecar** to the console Deployment that:
   - Terminates TLS on port 8443
   - Authenticates users via OpenShift OAuth
   - Checks that the user has the required role (default: `cluster-admin`)
   - Proxies authenticated requests to the console on port 8080

### Prerequisites

- OpenShift 4.14+ (OAuth server is built-in)
- The `ose-oauth-proxy` image must be accessible (included in OpenShift by default)
- The console Route must use TLS (reencrypt termination)

### Enabling OAuth on an Existing Console

If you deployed the console without authentication and want to enable OAuth:

```bash
# Patch the existing KairosConsole CR
oc patch kairosconsole kairos -n kairos-system --type='merge' -p '{
  "spec": {
    "auth": {
      "type": "openshift-oauth",
      "oauth": {
        "requiredRole": "cluster-admin"
      }
    }
  }
}'

# The operator will automatically:
# 1. Update the ServiceAccount with OAuth annotations
# 2. Add serving-cert annotation to Service
# 3. Create cookie secret
# 4. Add oauth-proxy sidecar to Deployment
# 5. Restart the pod

# Verify the Route now serves via OAuth
oc get route kairos-console -n kairos-system
```

### Changing the Required Role

By default, only `cluster-admin` users can access the console. To allow a different role:

```yaml
spec:
  auth:
    type: openshift-oauth
    oauth:
      requiredRole: "cluster-monitoring-view"  # More permissive
```

Common roles:
| Role | Who Has It | Access Level |
|---|---|---|
| `cluster-admin` | Cluster administrators | Full access (default) |
| `admin` | Namespace administrators | Project-level admin |
| `cluster-monitoring-view` | Monitoring users | Read-only monitoring |
| `view` | Any authenticated user | Read-only (least restrictive) |

### Troubleshooting OAuth

```bash
# Check oauth-proxy sidecar logs
oc logs deployment/kairos-console -n kairos-system -c oauth-proxy

# Verify ServiceAccount has OAuth annotations
oc get sa kairos-console -n kairos-system -o yaml | grep oauth

# Check TLS secret was auto-generated
oc get secret kairos-console-tls -n kairos-system

# Check cookie secret exists
oc get secret kairos-console-oauth-cookie -n kairos-system

# Test the Route (should redirect to OpenShift login)
curl -Ik https://kairos-console.apps.your-cluster.example.com
# Expected: HTTP/1.1 302 Found → Location: .../oauth/authorize
```

### Disabling OAuth (Reverting to Open Access)

```bash
oc patch kairosconsole kairos -n kairos-system --type='merge' -p '{
  "spec": {
    "auth": {
      "type": "none"
    }
  }
}'
```

The operator will remove the oauth-proxy sidecar and revert to direct HTTP access.

## Mode 3: Static Token Authentication

For air-gapped environments where OpenShift OAuth is not available:

```yaml
apiVersion: kairos.maximilianopizarro.github.io/v1alpha1
kind: KairosConsole
metadata:
  name: kairos
  namespace: kairos-system
spec:
  auth:
    type: token
    tokenSecret:
      name: kairos-console-token
      key: token
```

Create the token secret:
```bash
oc create secret generic kairos-console-token \
  -n kairos-system \
  --from-literal=token=$(openssl rand -hex 32)
```

Access the console by adding the `Authorization: Bearer <token>` header.

## Security Considerations

- **OAuth mode** is recommended for production because it leverages OpenShift's built-in identity provider (LDAP, OIDC, HTPasswd, etc.)
- The OAuth proxy validates both authentication (who you are) and authorization (what role you have)
- TLS certificates are auto-managed by OpenShift's service-ca operator
- Cookie secrets are auto-generated by the Kairos operator if not pre-configured
- The console backend (`localhost:8080`) is never directly exposed — all external traffic goes through the OAuth proxy
