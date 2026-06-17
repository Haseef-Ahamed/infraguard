package state

import (
	"encoding/json"
	"reflect"
	"time"

	"github.com/google/uuid"

	"github.com/infraguard/drift-engine/pkg/events"
)

// Comparator diffs live cloud state against IaC baselines
type Comparator struct{}

// NewComparator returns a new Comparator
func NewComparator() *Comparator {
	return &Comparator{}
}

// CompareSecurityGroups returns a DriftEvent for every security group
// whose live ingress/egress rules differ from its baseline.
// Resources present in `live` but absent from `baseline` are skipped —
// they are new resources, not drift.
func (c *Comparator) CompareSecurityGroups(
	live []events.SecurityGroupState,
	baseline []events.SecurityGroupState,
) []events.DriftEvent {
	baselineMap := make(map[string]events.SecurityGroupState, len(baseline))
	for _, b := range baseline {
		baselineMap[b.GroupID] = b
	}

	var drifts []events.DriftEvent
	for _, liveSG := range live {
		base, exists := baselineMap[liveSG.GroupID]
		if !exists {
			continue
		}

		if !reflect.DeepEqual(normaliseRules(liveSG.IngressRules), normaliseRules(base.IngressRules)) {
			drifts = append(drifts, events.DriftEvent{
				ID:            uuid.New(),
				Cloud:         "aws",
				ResourceType:  "aws_security_group",
				ResourceID:    liveSG.GroupID,
				ChangeType:    detectSGIngressChangeType(base.IngressRules, liveSG.IngressRules),
				PreviousState: base,
				NewState:      liveSG,
				DetectedAt:    time.Now().UTC(),
			})
		}

		if !reflect.DeepEqual(normaliseRules(liveSG.EgressRules), normaliseRules(base.EgressRules)) {
			drifts = append(drifts, events.DriftEvent{
				ID:            uuid.New(),
				Cloud:         "aws",
				ResourceType:  "aws_security_group",
				ResourceID:    liveSG.GroupID,
				ChangeType:    events.ChangeTypeEgressAdded,
				PreviousState: base,
				NewState:      liveSG,
				DetectedAt:    time.Now().UTC(),
			})
		}
	}
	return drifts
}

// CompareS3Buckets returns a DriftEvent if public access settings
// have changed from the baseline (any block becoming false is drift)
func (c *Comparator) CompareS3Buckets(
	live events.S3BucketState,
	baseline events.S3BucketState,
) []events.DriftEvent {
	var drifts []events.DriftEvent

	publicAccessChanged := live.BlockPublicAcls != baseline.BlockPublicAcls ||
		live.BlockPublicPolicy != baseline.BlockPublicPolicy ||
		live.IgnorePublicAcls != baseline.IgnorePublicAcls ||
		live.RestrictPublicBuckets != baseline.RestrictPublicBuckets

	if publicAccessChanged {
		drifts = append(drifts, events.DriftEvent{
			ID:            uuid.New(),
			Cloud:         "aws",
			ResourceType:  "aws_s3_bucket",
			ResourceID:    live.BucketName,
			ChangeType:    events.ChangeTypePublicAccess,
			PreviousState: baseline,
			NewState:      live,
			DetectedAt:    time.Now().UTC(),
		})
	}

	return drifts
}

// detectSGIngressChangeType determines whether rules were added or removed
func detectSGIngressChangeType(baseline, live []events.SGRule) events.ChangeType {
	if len(live) > len(baseline) {
		return events.ChangeTypeIngressAdded
	}
	if len(live) < len(baseline) {
		return events.ChangeTypeIngressRemoved
	}
	return events.ChangeTypeGeneric
}

// normaliseRules produces a stable JSON representation for deep comparison
func normaliseRules(rules []events.SGRule) string {
	b, _ := json.Marshal(rules)
	return string(b)
}
