# Kairos Operator — community-operators-prod Artifacts

Pre-built directory structure for submitting the Kairos Operator to
[community-operators-prod](https://github.com/redhat-openshift-ecosystem/community-operators-prod)
with **FBC (File-Based Catalog) automerge** support.

## Directory Layout

```
operators/kairos-operator/
├── ci.yaml                          # Reviewer list + FBC catalog mapping
├── Makefile                         # FBC Makefile (from operator-pipelines)
├── catalog-templates/
│   └── semver.yaml                  # OLM semver channel template
└── 2.1.0/
    ├── release-config.yaml          # Automerge release configuration
    ├── manifests/                   # ← copy from bundle/manifests/
    │   ├── kairos-operator.clusterserviceversion.yaml
    │   └── *.yaml (CRDs, etc.)
    └── metadata/                    # ← copy from bundle/metadata/
        └── annotations.yaml
```

## How to Use

1. **Build the bundle locally** (from the repo root):

   ```bash
   make bundle IMG=quay.io/maximilianopizarro/kairos-operator:v2.1.0
   ```

2. **Fork** `redhat-openshift-ecosystem/community-operators-prod`.

3. **Copy this tree** into your fork:

   ```bash
   cp -r community-operators-prod-artifacts/operators/kairos-operator \
     /path/to/community-operators-prod/operators/kairos-operator
   ```

4. **Copy the generated bundle manifests and metadata**:

   ```bash
   cp -r bundle/manifests \
     /path/to/community-operators-prod/operators/kairos-operator/2.1.0/manifests
   cp -r bundle/metadata \
     /path/to/community-operators-prod/operators/kairos-operator/2.1.0/metadata
   ```

5. **Open a PR** against `community-operators-prod` `main` branch.

   The `ci.yaml` file lists `maximilianoPizarro` as a reviewer and enables
   FBC with semver catalog mapping across OpenShift 4.12–4.22. The
   `release-config.yaml` in the version directory enables the automerge
   pipeline.

## Key Files

| File | Purpose |
|------|---------|
| `ci.yaml` | Configures reviewers and FBC catalog mapping (required for automerge) |
| `catalog-templates/semver.yaml` | Defines the OLM semver channel with all bundle images |
| `2.1.0/release-config.yaml` | Tells the pipeline which catalog template and channels to use |
| `Makefile` | FBC Makefile from [operator-pipelines](https://github.com/redhat-openshift-ecosystem/operator-pipelines) |

## Updating for Future Versions

To add a new version (e.g. 2.2.0):

1. Add a new bundle image entry to `catalog-templates/semver.yaml`
2. Create a `2.2.0/` directory with its own `release-config.yaml`
3. Copy the new `bundle/manifests/` and `bundle/metadata/` into `2.2.0/`
