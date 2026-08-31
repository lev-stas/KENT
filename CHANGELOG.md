# Changelog

All notable changes to this project will be documented in this file.
This project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

---

## [0.4.0] – 2026-08-31

### Added
- **Event filters** beside the existing namespace ones, all optional and all off by default
  (an upgrade without a values change exports exactly what it exported before):
  - `include_event_types` — keep only `Normal` or `Warning` events (matched case-insensitively).
  - `include_kinds` — keep only events about the given object kinds (`Pod`, `Node`, …).
  - `include_reasons` / `exclude_reasons` — RE2 patterns over the event reason. Two keys
    rather than one because Go's regexp has no negative lookahead, so a single pattern
    cannot express both "only these reasons" and "everything but these".

  An event is exported when it passes every configured rule; `exclude_*` wins over
  `include_*`. An invalid pattern fails at startup instead of silently dropping events,
  and the effective filter is logged when the exporter starts.
- **`k8s.object.name`** — the name of the object an event is about. `k8s.name` holds the
  name of the Event object itself (`<object>.<hex>`), so until now the object could not be
  grouped or filtered on, even though `k8s.kind` described it. `k8s.name` is unchanged.
- **`event.action`** (what was attempted regarding the object) and
  **`event.reporting_instance`** (the ID of the reporting controller instance, falling back
  to the reporting node name for recorders that only report one). Both are present only when
  the cluster reports them — `events.k8s.io/v1` recorders fill them in, older ones often do
  not — and empty values are omitted rather than written as empty fields.

### Changed
- `kent_events_filtered_total` now counts every filter, not just the namespace ones
  (metric name and type are unchanged).

### Fixed
- On the `core/v1` path, `event.source` was empty for events recorded through
  `events.k8s.io/v1`, which name their reporter in `reportingComponent` and leave the
  legacy `source` empty; it now falls back to that field, matching what the
  `events.k8s.io/v1` path has always reported.

---

## [0.3.1] – 2026-08-10

### Added
- Helm chart 0.3.1: a ClusterIP Service in front of the metrics port, and an optional
  `ServiceMonitor` (`serviceMonitor.enabled`, off by default since the Prometheus Operator
  CRD is not present in every cluster) with `interval`, `scrapeTimeout` and
  `additionalLabels` options. Chart-only change — the exporter itself is unchanged.

---

## [0.3.0] – 2026-08-07

Validated with an e2e suite on kind (VictoriaLogs outage, poison batches, graceful
shutdown, auth/TLS) and a 12-hour side-by-side run against v0.1.1 in a live cluster:
full event coverage, zero duplicates, zero losses.

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
