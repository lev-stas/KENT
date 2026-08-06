# Changelog

All notable changes to this project will be documented in this file.
This project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- **Delivery reliability**:
  - Failed batches are retried with exponential backoff (1s → 30s) on transient errors
    (network failures, HTTP 429/5xx); permanently rejected batches (other 4xx) are dropped
    with an error log and a metric instead of blocking the pipeline.
  - The Kubernetes watch resumes from the last seen `resourceVersion` after reconnects
    (with `AllowWatchBookmarks`), so events emitted while disconnected are recovered as
    long as the version is still within the API server's retention window. Expired
    versions (HTTP 410 Gone) fall back to a fresh watch. The checkpoint is in-memory:
    a pod restart starts fresh and re-exports the currently stored events.
  - Events re-delivered by the watch (reconnects, TTL deletions) are deduplicated by
    UID + count with a bounded two-generation cache (2×8192 UIDs).
  - Graceful shutdown now drains the whole pipeline: the collector hands already-fetched
    events to the writers (bounded grace period), then the writer flushes its queue and
    buffer chunk-by-chunk with fresh bounded contexts; `Stop()` blocks until done.
  - `queue_size` config option (`VL_QUEUE_SIZE`, default 5000) for the in-memory send queue.
- **Prometheus metrics** on the health port (`/metrics`): events received/filtered/
  deduplicated/sent/dropped, send errors, queue length, batch size histogram, watch reconnects.
- **Authentication and TLS** for the VictoriaLogs endpoint: basic auth, bearer token,
  custom HTTP headers, custom CA, client certificates, `insecure_skip_verify`, `server_name`.
- Helm chart: `podAnnotations`, `extraVolumes`/`extraVolumeMounts` (for TLS files), container
  `ports` declaration for metrics scraping; new `queueSize`, `headers`, `auth.*`, `tls.*` values.

### Fixed
- Recurring events are now stamped with their last-observed time instead of the first
  occurrence, so count increments of a long-lived event no longer land days in the past
  (events/v1 additionally falls back to `series.lastObservedTime`).
- Helm: the deployment now carries a `checksum/config` annotation, so `helm upgrade`
  with changed config restarts the pod instead of leaving it on the old configuration.
- A batch that failed to send to VictoriaLogs was silently dropped; it is now retried.
- Reconnecting to the watch could silently lose events emitted during the disconnect window.
- Shutdown discarded buffered events because the final flush ran with an already-cancelled
  context; deletions of expired Event objects were re-exported as fresh events.
- Debug logging no longer prints request headers and payloads (they may contain credentials).

---

## [0.2.0] – 2026-03-29

### Added
- Optional `stdout` exporter with `stdout.enabled`, allowing events to be emitted as JSON lines alongside or instead of VictoriaLogs.
- Explicit `recordType` markers in stdout output:
  - `kubernetes_event` for exported Kubernetes events
  - `exporter_log` for service logs produced by KENT itself

### Fixed
- Documentation now reflects the current Helm chart path and stdout configuration.

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
