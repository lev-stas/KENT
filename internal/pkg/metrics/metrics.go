// Copyright 2025 Stas Levchenko
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0

package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	EventsReceived = promauto.NewCounter(prometheus.CounterOpts{
		Name: "kent_events_received_total",
		Help: "Kubernetes events received from the watch stream, before filtering.",
	})

	EventsFiltered = promauto.NewCounter(prometheus.CounterOpts{
		Name: "kent_events_filtered_total",
		Help: "Events skipped by namespace include/exclude filters.",
	})

	EventsDeduplicated = promauto.NewCounter(prometheus.CounterOpts{
		Name: "kent_events_deduplicated_total",
		Help: "Events skipped as duplicates (same UID and count seen before).",
	})

	EventsSent = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "kent_events_sent_total",
		Help: "Events successfully delivered, per writer.",
	}, []string{"writer"})

	EventsDropped = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "kent_events_dropped_total",
		Help: "Events dropped without delivery, per writer and reason.",
	}, []string{"writer", "reason"})

	SendErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "kent_send_errors_total",
		Help: "Failed delivery attempts, per writer (retries count individually).",
	}, []string{"writer"})

	QueueLength = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kent_send_queue_length",
		Help: "Events waiting in the writer queue.",
	}, []string{"writer"})

	WatchReconnects = promauto.NewCounter(prometheus.CounterOpts{
		Name: "kent_watch_reconnects_total",
		Help: "Kubernetes watch reconnects.",
	})

	BatchSize = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "kent_batch_size",
		Help:    "Number of events per delivered batch.",
		Buckets: []float64{1, 10, 50, 100, 250, 500, 1000},
	})
)

// Handler serves the default Prometheus registry.
func Handler() http.Handler {
	return promhttp.Handler()
}
