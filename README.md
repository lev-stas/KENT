# KENT – Kubernetes Events Notifier

### KENT (Kubernetes Events Notifier) is a minimalistic Kubernetes events exporter.

At this stage, KENT ships events to [VictoriaLogs] (https://docs.victoriametrics.com/victorialogs/), but its architecture allows for adding other log storage systems in the future.

### Why KENT?

At [Ivinco] (https://www.ivinco.com), we regularly deal with complex infrastructure and monitoring challenges.
While solving one of them, we came up with the idea of creating a tool that would be:

- Simple to configure – no dozens of parameters that never work in practice.
- Minimalistic – providing only the essential features.
- Flexible – support for namespace filters, stream_fields, and extra_fields.
- Production-ready – works reliably in Kubernetes, with health probes and graceful shutdown.

That’s how KENT was born.

#### Features

- Collects Kubernetes events (same as kubectl get events) from all or selected namespaces.
- Exports events to VictoriaLogs via JSONLine API.
- Supports multi-tenancy (AccountID, ProjectID).
- Configurable options:
  * include_namespaces / exclude_namespaces
  * stream_fields (define how logs are grouped into streams)
  * extra_fields (attach custom metadata to all events)
- Health endpoints:
  * /healthz – liveness probe
  * /ready – readiness probe

#### Installation

Clone the repository and install helm chart

```
git clone https://github.com/lev-stas/KENT.git
cd KENT/chart
helm upgrade --install -n monitoring -f values.yaml kent .
```


#### Project Structure
`cmd/` – entrypoint (main.go)
`internal/app/` – application orchestration
`internal/adapters/` – adapters for Kubernetes and log storage (currently VictoriaLogs)
`internal/usecase/` – business logic (collecting and delivering events)
`internal/domain/` – core entities (Event, LogEntry)
`internal/http/` – health endpoints
`deploy/chart/` – Helm chart for deployment

#### License

Licensed under the Apache 2.0 License

© 2025 Stanislav Levchenko