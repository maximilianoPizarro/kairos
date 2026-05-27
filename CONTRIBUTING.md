# Contributing to Kairos Operator

Thank you for your interest in contributing to Kairos! This document provides guidelines and instructions for contributing.

## Code of Conduct

By participating in this project, you agree to maintain a respectful and inclusive environment for everyone.

## How to Contribute

### Reporting Issues

- Use [GitHub Issues](https://github.com/maximilianoPizarro/kairos/issues) to report bugs or request features
- Include the Kairos version, OpenShift version, and steps to reproduce
- For security vulnerabilities, please email directly instead of opening a public issue

### Development Setup

#### Prerequisites

- Go 1.22+
- OpenShift 4.14+ cluster (or kind/minikube for local dev)
- Operator SDK v1.40.0
- Podman or Docker
- Node.js 20+ (for console frontend)
- `golangci-lint` (for linting)

#### Clone and Build

```bash
git clone https://github.com/maximilianoPizarro/kairos.git
cd kairos

# Install CRDs into your cluster
make install

# Run the operator locally (uses your kubeconfig)
make run

# Run tests
make test

# Run linter
make lint
```

#### Console Development

```bash
cd console/frontend
npm install
npm run dev        # Start dev server with hot reload

# In another terminal, run the console backend:
cd console
go run main.go     # Starts on :8080
```

### Submitting Changes

1. **Fork** the repository
2. **Create a branch** from `main`:
   ```bash
   git checkout -b feature/my-feature
   ```
3. **Make your changes** following the coding standards below
4. **Run tests and lint**:
   ```bash
   make test
   make lint
   ```
5. **Commit** with a clear message:
   ```bash
   git commit -m "feat: add support for custom metrics source"
   ```
6. **Push** and open a Pull Request

### Commit Message Format

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>

[optional body]
```

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `refactor`: Code restructuring without behavior change
- `test`: Adding or modifying tests
- `ci`: CI/CD changes
- `chore`: Maintenance tasks

Examples:
```
feat(agent): add namespace label selector filtering
fix(controller): prevent crash loop on invalid CRD spec
docs(pages): add multi-cluster configuration examples
ci(actions): add GHCR push to build workflow
```

## Coding Standards

### Go Code

- Follow standard Go conventions (`gofmt`, `go vet`)
- Pass `golangci-lint` with the project's `.golangci.yml` configuration
- Use constants for repeated string literals (enforced by `goconst`)
- Pre-allocate slices when size is known (enforced by `prealloc`)
- Always handle errors (`errcheck` enforced)
- Use `context.Context` for all controller operations
- Controllers must follow the anti-crash pattern:
  - Use `ExponentialFailureRateLimiter` (5s base, 5min max)
  - Never requeue immediately on error
  - Limit `MaxConcurrentReconciles`

### TypeScript/React (Console Frontend)

- Use TypeScript strict mode
- Use PatternFly 5 components for all UI elements
- Follow React functional component patterns with hooks
- Keep pages in `console/frontend/src/pages/`

### CRD Changes

When modifying CRD types in `api/v1alpha1/`:

```bash
# After editing *_types.go files:
make generate manifests

# Verify CRDs apply cleanly:
oc apply -f config/crd/bases/ --dry-run=server
```

### Container Images

- All images MUST use Red Hat UBI9 base images
- Builder: `registry.access.redhat.com/ubi9/go-toolset:latest`
- Frontend: `registry.access.redhat.com/ubi9/nodejs-20:latest`
- Runtime: `registry.access.redhat.com/ubi9/ubi-micro:latest`

## Project Structure

```
api/v1alpha1/           CRD type definitions
internal/controller/    Reconciler implementations
internal/metrics/       OTel + Prometheus/Thanos clients
internal/ai/            AI model client (OpenAI-compatible)
internal/scaler/        SSA scaling coordinator
console/                Governance console (Go backend)
console/frontend/       React/PatternFly 5 frontend
config/crd/bases/       Generated CRD manifests
config/samples/         Example CRs for testing
config/rbac/            RBAC manifests
bundle/                 OLM operator bundle
docs/                   Architecture diagrams (SVG)
docs/pages/             GitHub Pages site
.github/workflows/      CI/CD pipelines
```

## Testing

### Unit Tests

```bash
make test
```

Tests use `envtest` (controller-runtime's test framework) with a local API server.

### End-to-End Tests

```bash
make test-e2e
```

E2E tests use `kind` to create a local cluster, deploy the operator, and validate CRD behavior.

### Local Testing with OpenShift

```bash
# Deploy operator
make deploy IMG=quay.io/maximilianopizarro/kairos-operator:dev

# Apply sample CRs
oc apply -f config/samples/

# Watch operator logs
oc logs -f deployment/kairos-controller-manager -n kairos-system

# Clean up
make undeploy
```

## Areas for Contribution

We welcome contributions in these areas:

- **New metrics sources**: Additional integrations beyond OTel/Thanos/Prometheus
- **AI model improvements**: Better prompt engineering, caching, model-specific optimizations
- **Console enhancements**: New visualizations, real-time updates via WebSocket
- **Documentation**: Tutorials, use case guides, video walkthroughs
- **Testing**: More comprehensive e2e scenarios, chaos testing
- **Performance**: Profiling and optimization of reconcile loops
- **Security**: mTLS between components, OIDC authentication for console

## Release Process

Releases are created from the `main` branch:

1. Update `CHANGELOG.md` with new version section
2. Tag: `git tag v0.X.0`
3. Push tag: `git push origin v0.X.0`
4. GitHub Actions builds and pushes images with version tag
5. Create GitHub Release with changelog notes

## Questions?

- Open a [GitHub Discussion](https://github.com/maximilianoPizarro/kairos/discussions)
- Check the [Documentation](https://maximilianoPizarro.github.io/kairos)

---

*Thank you for contributing to Kairos! καιρός — The Opportune Moment*
