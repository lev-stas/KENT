# Changelog

All notable changes to this project will be documented in this file.
This project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

### Fixed

---

## [0.1.1] – 2025-10-06

### Added
- **Automatic API detection** — KENT now supports both `events.k8s.io/v1` and legacy `core/v1` APIs.  
  The exporter automatically selects the best available API version depending on cluster capabilities.
- **Makefile** for local and multi-architecture builds (`amd64`, `arm64`), including:  
  - `make build` for cross-compilation  
  - `make docker` for multi-platform Docker builds  

### Fixed
- Improved event validation logic to minimize dropped events (missing `source`, `EventTime`, etc.).  
  Events without deprecated fields are now correctly processed.

---

## [0.1.0] – 2025-10-05

### Added
- Initial release of **KENT (Kubernetes Events Notifier)**.
- Collects Kubernetes events and exports them to VictoriaLogs.
- Configurable namespaces, extra fields, and stream fields.
- Health endpoints (`/healthz`, `/ready`).
- Helm chart for deployment.
- Multi-arch Docker images (`amd64`, `arm64`).
