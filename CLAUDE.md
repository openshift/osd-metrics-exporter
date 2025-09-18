# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is the **OpenShift Dedicated Metrics Exporter**, a Prometheus exporter that exposes metrics about various features used in OpenShift Dedicated clusters. The project is built using Go and follows OpenShift operator patterns with controller-runtime.

## Common Development Commands

### Building and Testing
- `make go-build` - Build the binary to `build/_output/bin/osd-metrics-exporter`
- `make go-test` - Run unit tests
- `make go-check` - Run linting and static analysis with golangci-lint
- `make test` - Run local unit and functional testing
- `make lint` - Perform static analysis including YAML validation

### Local Development
- `make clean` - Remove build artifacts
- `make generate` - Generate code (CRDs, OpenAPI, manifests)
- `make validate` - Ensure code generation is up-to-date and boilerplate is unmodified

### E2E Testing
E2E tests require environment variables:
- `OCM_TOKEN` - OCM token for authentication
- `OCM_CLUSTER_ID` - Target cluster ID

Run E2E tests from `test/e2e/`:
```bash
export OCM_TOKEN=$(ocm token)
export OCM_CLUSTER_ID=<YOUR-CLUSTER-ID>
DISABLE_JUNIT_REPORT=true ginkgo run --tags=osde2e,e2e --procs 4 --flake-attempts 3 --trace -vv .
```

## Architecture

### Core Components

**Main Entry Point** (`main.go:47-50`): Sets up controller manager and registers all metric controllers

**Controllers** (`controllers/`): Each controller watches specific OpenShift resources and exports metrics:
- `oauth/` - Identity provider metrics
- `proxy/` - Cluster proxy and CA certificate metrics
- `group/` - Cluster admin metrics
- `limited_support/` - Limited support status
- `clusterrole/` - Cluster role permissions
- `configmap/` - Configuration-based metrics
- `cpms/` - ControlPlaneMachineSet state
- `machine/` - Machine and node metrics

**Metrics System** (`pkg/metrics/`):
- `metrics.go` - Central metrics aggregator singleton
- `aggregator.go` - Collects and exposes metrics via Prometheus

### Metrics Exported

Current metrics include:
1. Identity Provider configuration
2. Cluster Admin presence
3. Limited Support status
4. Cluster Proxy configuration
5. Cluster Proxy CA expiry timestamp and validity
6. Cluster ID
7. ControlPlaneMachineSet state

### Build System

Uses OpenShift boilerplate system with:
- FIPS-enabled builds (`FIPS_ENABLED=true`)
- Konflux builds (`KONFLUX_BUILDS=true`)
- Go 1.24+ requirement
- Controller-runtime framework
- Prometheus client libraries

### Project Configuration

Key configuration in `config/config.go`:
- Operator name: `osd-metrics-exporter`
- Namespace: `openshift-osd-metrics`
- OLM skip-range enabled

### Dependencies

- OpenShift APIs and client libraries
- Controller-runtime for Kubernetes operator patterns
- Prometheus client for metrics exposition
- OCM SDK for OpenShift Cluster Manager integration
- Ginkgo/Gomega for testing