package metrics_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"

	"github.com/infraguard/drift-engine/pkg/metrics"
)

func TestDriftEventsTotal_Increment(t *testing.T) {
	metrics.DriftEventsTotal.WithLabelValues(
		"aws", "aws_security_group", "CRITICAL", "INGRESS_RULE_ADDED",
	).Inc()

	result := testutil.CollectAndCount(metrics.DriftEventsTotal)
	assert.GreaterOrEqual(t, result, 1)
}

func TestResourcesMonitored_SetAndReset(t *testing.T) {
	metrics.ResourcesMonitored.WithLabelValues("aws", "aws_security_group").Set(5)
	metrics.ResourcesMonitored.WithLabelValues("aws", "aws_s3_bucket").Set(3)

	count := testutil.CollectAndCount(metrics.ResourcesMonitored)
	assert.GreaterOrEqual(t, count, 2)
}

func TestNATSPublishErrors_Increment(t *testing.T) {
	before := testutil.ToFloat64(metrics.NATSPublishErrors)
	metrics.NATSPublishErrors.Inc()
	after := testutil.ToFloat64(metrics.NATSPublishErrors)
	assert.Equal(t, before+1, after)
}

func TestDetectionLatency_Observe(t *testing.T) {
	// Should not panic on any observation
	metrics.DetectionLatency.Observe(0.5)
	metrics.DetectionLatency.Observe(15.3)
	metrics.DetectionLatency.Observe(60.0)

	count := testutil.CollectAndCount(metrics.DetectionLatency)
	assert.Equal(t, 1, count)
}

func TestScanDuration_Observe(t *testing.T) {
	metrics.ScanDuration.Observe(2.5)

	count := testutil.CollectAndCount(metrics.ScanDuration)
	assert.Equal(t, 1, count)
}
