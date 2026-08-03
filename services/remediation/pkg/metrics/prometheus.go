package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var PRsOpenedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "infraguard_remediation_prs_opened_total",
	Help: "Total remediation PRs opened",
}, []string{"severity"})

var SlackAlertsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "infraguard_slack_alerts_total",
	Help: "Total Slack alerts sent",
}, []string{"severity"})

var SLABreachesTotal = promauto.NewCounter(prometheus.CounterOpts{
	Name: "infraguard_sla_breaches_total",
	Help: "Total SLA breaches triggering escalation",
})

var TrackedEventsGauge = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "infraguard_sla_tracked_events",
	Help: "Number of drift events currently tracked for SLA",
})

var DashboardClientsGauge = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "infraguard_dashboard_clients_connected",
	Help: "Number of connected WebSocket dashboard clients",
})
