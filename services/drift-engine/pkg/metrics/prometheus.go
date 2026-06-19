package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// DriftEventsTotal counts every drift event detected,
// labelled by cloud provider, resource type, severity, and change type.
var DriftEventsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "infraguard_drift_events_total",
	Help: "Total number of drift events detected",
}, []string{"cloud", "resource_type", "severity", "change_type"})

// DetectionLatency measures the time from cloud event emission
// to NATS publish, in seconds.
var DetectionLatency = promauto.NewHistogram(prometheus.HistogramOpts{
	Name:    "infraguard_detection_latency_seconds",
	Help:    "Time from cloud scan start to NATS publish for detected drift",
	Buckets: []float64{0.5, 1, 2, 5, 10, 20, 30, 60, 90, 120},
})

// ResourcesMonitored tracks how many cloud resources are
// currently under active monitoring.
var ResourcesMonitored = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "infraguard_resources_monitored_total",
	Help: "Number of cloud resources currently being monitored",
}, []string{"cloud", "resource_type"})

// ScanDuration measures how long each full scan cycle takes.
var ScanDuration = promauto.NewHistogram(prometheus.HistogramOpts{
	Name:    "infraguard_scan_duration_seconds",
	Help:    "Time taken to complete one full drift scan cycle",
	Buckets: []float64{1, 5, 10, 30, 60, 120, 300},
})

// NATSPublishErrors counts failed NATS publish attempts.
// Alert if this is non-zero.
var NATSPublishErrors = promauto.NewCounter(prometheus.CounterOpts{
	Name: "infraguard_nats_publish_errors_total",
	Help: "Number of failed NATS publish attempts",
})
