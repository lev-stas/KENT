# KENT — Kubernetes Event Exporter for VictoriaLogs

[![Release](https://img.shields.io/github/v/release/lev-stas/KENT)](https://github.com/lev-stas/KENT/releases)
[![License](https://img.shields.io/github/license/lev-stas/KENT)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/lev-stas/KENT)](https://goreportcard.com/report/github.com/lev-stas/KENT)
[![Docker Pulls](https://img.shields.io/docker/pulls/staslevchenko/kent)](https://hub.docker.com/r/staslevchenko/kent)

**KENT (Kubernetes Events Notifier)** is a lightweight **Kubernetes event exporter** that ships cluster events — everything you see in `kubectl get events` — to **[VictoriaLogs](https://docs.victoriametrics.com/victorialogs/)** as structured, queryable log records. It can also emit events to stdout as JSON lines, so any log shipper (Vector, Fluent Bit, Promtail, …) can pick them up from container output.

Deploy with Helm in one minute and keep every `Warning`, `FailedScheduling`, `BackOff`, image pull error, and eviction searchable with LogsQL — long after the cluster has forgotten them.

## Why export Kubernetes events?

Kubernetes events are the first place cluster problems show up: failed scheduling, image pull errors, OOM kills, failing probes, evictions. But events are ephemeral — by default the cluster keeps them for only **one hour**. If you don't export Kubernetes events to persistent storage, they are gone before you start debugging.

## Why KENT?

Most Kubernetes event exporters (such as `kubernetes-event-exporter`) aim to be universal: routing trees, dozens of sinks, template engines. KENT was built at [Ivinco](https://www.ivinco.com) for production clusters with the opposite philosophy:

- **Simple to configure** — a handful of options that all work in practice; no routing trees to debug.
- **VictoriaLogs-native** — first-class support for the JSON lines ingestion API, [stream fields](https://docs.victoriametrics.com/victorialogs/keyconcepts/#stream-fields), and VictoriaLogs multitenancy (`AccountID` / `ProjectID`). Not a generic Elasticsearch adapter.
- **Minimalistic** — a small Go binary; 50m CPU / 64Mi memory requests are enough for a busy cluster.
- **Reliable delivery** — failed batches are retried with exponential backoff, the watch resumes from the last `resourceVersion` after reconnects, re-delivered events are deduplicated, and the buffer is flushed on shutdown.
- **Production-ready** — batching, Prometheus metrics, health probes, graceful shutdown.

## Features

- Collects Kubernetes events cluster-wide or from selected namespaces (`include_namespaces` / `exclude_namespaces`)
- Supports both the `events.k8s.io/v1` API and legacy `core/v1` events, with automatic API detection per cluster
- Exports events to VictoriaLogs via the JSON lines (`/insert/jsonline`) API, with configurable batching (`batchSize`, `flushTime`)
- Delivery reliability: exponential-backoff retries on transient failures (network, 429, 5xx), watch resume from the last `resourceVersion`, deduplication of re-delivered events, full pipeline drain on graceful shutdown; when VictoriaLogs is down, KENT applies backpressure instead of dropping events
- Authentication and TLS for the VictoriaLogs endpoint: basic auth, bearer token, custom headers, custom CA / client certificates
- VictoriaLogs multitenancy support (`AccountID`, `ProjectID`) plus a `clusterID` field for multi-cluster setups
- `stream_fields` — control how VictoriaLogs groups events into log streams
- `extra_fields` — attach static metadata (environment, region, team) to every event
- Optional stdout export: events as JSON lines tagged `recordType: "kubernetes_event"`; KENT's own service logs are tagged `recordType: "exporter_log"`, so the two are easy to separate in one stream
- Prometheus metrics on `/metrics` (see [Observability](#observability)); health endpoints: `/healthz` (liveness) and `/ready` (readiness)
- Helm chart included; multi-arch Docker images (`amd64`, `arm64`)

## Quick start

Install the Helm chart from the KENT repository:

```bash
helm repo add kent https://lev-stas.github.io/KENT
helm upgrade --install -n monitoring --create-namespace kent kent/kent-exporter \
  --set config.victorialogs.endpoint=http://vlogs.example.com:9428 \
  --set config.victorialogs.clusterID=k8s-prod
```

Or point the exporter at your VictoriaLogs instance in a values file:

```yaml
config:
  victorialogs:
    enabled: true
    endpoint: "http://vlogs.example.com:9428"
    clusterID: "k8s-prod"
```

## Configuration

All options live in the Helm chart's `values.yaml`:

| Option | Default | Description |
|---|---|---|
| `config.kubernetes.include_namespaces` | `[]` (all) | Export events only from these namespaces |
| `config.kubernetes.exclude_namespaces` | `[]` | Skip events from these namespaces |
| `config.victorialogs.enabled` | `true` | Enable the VictoriaLogs exporter |
| `config.victorialogs.endpoint` | — | VictoriaLogs base URL |
| `config.victorialogs.clusterID` | — | Cluster name added to every event; also a stream field |
| `config.victorialogs.accountID` / `projectID` | `0` / `0` | VictoriaLogs tenant |
| `config.victorialogs.batchSize` | `500` | Max events per insert request |
| `config.victorialogs.flushTime` | `30s` | Max time a batch waits before being sent |
| `config.victorialogs.streamFields` | `["k8s.namespace"]` | Extra fields to use as VictoriaLogs stream fields |
| `config.victorialogs.extraFields` | `{}` | Static fields added to every event |
| `config.victorialogs.queueSize` | `5000` | In-memory send queue capacity (events) |
| `config.victorialogs.headers` | `{}` | Extra HTTP headers for insert requests |
| `config.victorialogs.auth.basicUsername` / `basicPassword` | — | Basic auth credentials (password is better passed via `extraEnv` as `VL_AUTH_BASIC_PASSWORD`) |
| `config.victorialogs.auth.bearerToken` | — | Bearer token (mutually exclusive with basic auth) |
| `config.victorialogs.tls.*` | — | `caFile`, `certFile`, `keyFile`, `serverName`, `insecureSkipVerify`; mount files via `extraVolumes` / `extraVolumeMounts` |
| `config.stdout.enabled` | `false` | Also emit events to stdout as JSON lines |

## Observability

KENT exposes Prometheus metrics on the health port (`:8080/metrics`), ready to be scraped by vmagent or Prometheus:

- `kent_events_received_total`, `kent_events_filtered_total`, `kent_events_deduplicated_total`
- `kent_events_sent_total{writer}`, `kent_events_dropped_total{writer,reason}`, `kent_send_errors_total{writer}`
- `kent_send_queue_length{writer}`, `kent_batch_size` (histogram), `kent_watch_reconnects_total`

## Exported fields and LogsQL examples

Each Kubernetes event becomes a log record with these fields:

`_time`, `_msg` (event message), `level`, `clusterID`, `k8s.namespace`, `k8s.name`, `k8s.kind`, `event.reason`, `event.type`, `event.source`, `event.count` — plus anything you add via `extraFields`.

Query them in VictoriaLogs with [LogsQL](https://docs.victoriametrics.com/victorialogs/logsql/):

```logsql
# All Warning events in a namespace over the last hour
_time:1h {clusterID="k8s-prod", k8s.namespace="monitoring"} event.type:Warning

# Why pods are not scheduling
_time:24h {clusterID="k8s-prod"} event.reason:FailedScheduling

# Top event reasons across the cluster
_time:24h {clusterID="k8s-prod"} | stats by (event.reason) count() events | sort by (events) desc | limit 10
```

## Multi-cluster setup

Run one KENT instance per cluster, all pointing at the same VictoriaLogs endpoint, each with its own `clusterID` (and, if you use VictoriaLogs multitenancy, its own `accountID`/`projectID`). `clusterID` is always a stream field, so per-cluster queries stay cheap.

## KENT vs kubernetes-event-exporter

| | KENT | kubernetes-event-exporter |
|---|---|---|
| Focus | VictoriaLogs, done well | 20+ sinks (ES, Slack, Kafka, SQS, …) |
| VictoriaLogs support | Native: JSON lines API, stream fields, multitenancy | Indirect, via Elasticsearch/Loki-compatible sinks |
| Delivery guarantees | Retries with backoff, watch resume, dedup, shutdown flush | Events older than `maxEventAgeSeconds` (default 5s) are discarded |
| Configuration | Flat, a dozen keys | Routing tree + receivers + payload templates |
| Helm chart | Included in this repo | Third-party (Bitnami) |

If you need to fan events out to Slack, Opsgenie, or Kafka, `kubernetes-event-exporter` is the right tool. If you want Kubernetes events in VictoriaLogs with minimal moving parts — that is exactly what KENT does.

## Project structure

- `cmd/` — entrypoint (`main.go`)
- `internal/app/` — application orchestration
- `internal/adapters/` — adapters for Kubernetes and event delivery targets
- `internal/usecase/` — business logic (collecting and delivering events)
- `internal/domain/` — core entities (`Event`, `LogEntry`)
- `internal/http/` — health endpoints
- `deploy/chart/` — Helm chart for deployment

## License

Licensed under the [Apache 2.0 License](LICENSE).

© 2025 Stanislav Levchenko
