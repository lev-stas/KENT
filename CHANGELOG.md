# Changelog

All notable changes to this project will be documented in this file.
This project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

### Fixed

---

## [0.1.0] – 2025-10-05

### Added
- Initial release of **KENT (Kubernetes Events Notifier)**.
- Collects Kubernetes events and exports them to VictoriaLogs.
- Configurable namespaces, extra fields, and stream fields.
- Health endpoints (`/healthz`, `/ready`).
- Helm chart for deployment.
- Multi-arch Docker images (`amd64`, `arm64`).
