# Kairos + ArgoCD Coexistence

Kairos is designed to work alongside ArgoCD without causing sync conflicts. This document explains the pattern.

## The Problem

When ArgoCD manages a Deployment and another controller (Kairos) modifies its `resources` field, ArgoCD detects "drift" and attempts to revert the change on the next sync cycle. This creates a fight between the two controllers.

## The Solution: Resources Empty Pattern

### Step 1: Leave resources empty in Git

In your Git-managed manifests (which ArgoCD syncs), leave the `resources` field empty:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-service
  annotations:
    kairos.io/managed: "true"
    kairos.io/policy: "my-scaling-policy"
spec:
  template:
    spec:
      containers:
      - name: app
        image: my-image:latest
        resources: {}   # Empty - Kairos manages this
```

### Step 2: Configure ArgoCD ignoreDifferences

In your ArgoCD Application, add `ignoreDifferences` for the resources field:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: my-app
spec:
  ignoreDifferences:
  - group: apps
    kind: Deployment
    jqPathExpressions:
    - .spec.template.spec.containers[].resources
  - group: apps
    kind: StatefulSet
    jqPathExpressions:
    - .spec.template.spec.containers[].resources
```

For system-wide configuration, use ArgoCD's `resource.customizations` in the ConfigMap:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: argocd-cm
  namespace: openshift-gitops
data:
  resource.customizations.ignoreDifferences.apps_Deployment: |
    jqPathExpressions:
    - .spec.template.spec.containers[].resources
```

### Step 3: Server-Side Apply (how Kairos works)

Kairos uses Kubernetes Server-Side Apply with field manager `kairos-operator`. This means:

- ArgoCD owns all fields EXCEPT `.spec.template.spec.containers[].resources`
- Kairos owns ONLY the `resources` field
- Kubernetes API server tracks ownership per-field, preventing conflicts

### Step 4: Opt-in via annotation

Only resources with `kairos.io/managed: "true"` annotation are touched by the operator. Without this annotation, Kairos completely ignores the resource.

## Integration with platform-hub-spoke-config

For the hub-spoke architecture, add the ignoreDifferences to your ArgoCD ApplicationSet:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: industrial-edge
spec:
  template:
    spec:
      ignoreDifferences:
      - group: apps
        kind: Deployment
        jqPathExpressions:
        - .spec.template.spec.containers[].resources
```

## Verification

After deploying, verify the field ownership:

```bash
kubectl get deployment my-service -o json | jq '.metadata.managedFields[] | select(.manager == "kairos-operator")'
```

You should see Kairos owning only the resources field.
