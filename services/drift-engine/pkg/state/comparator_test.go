package state_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/infraguard/drift-engine/pkg/events"
	"github.com/infraguard/drift-engine/pkg/state"
)

func baselineSG() events.SecurityGroupState {
	return events.SecurityGroupState{
		GroupID:   "sg-abc123",
		GroupName: "infraguard-app-sg",
		VPCID:     "vpc-001",
		IngressRules: []events.SGRule{
			{Protocol: "tcp", FromPort: 443, ToPort: 443, CIDRs: []string{"0.0.0.0/0"}},
		},
		EgressRules: []events.SGRule{
			{Protocol: "-1", FromPort: 0, ToPort: 0, CIDRs: []string{"0.0.0.0/0"}},
		},
	}
}

func TestCompareSecurityGroups_NoChange(t *testing.T) {
	c := state.NewComparator()
	sg := baselineSG()

	drifts := c.CompareSecurityGroups([]events.SecurityGroupState{sg}, []events.SecurityGroupState{sg})
	assert.Empty(t, drifts, "identical state must produce zero drift events")
}

func TestCompareSecurityGroups_IngressAdded(t *testing.T) {
	c := state.NewComparator()
	base := baselineSG()
	live := baselineSG()

	// Simulate: someone opened port 5432 to the internet
	live.IngressRules = append(live.IngressRules, events.SGRule{
		Protocol: "tcp", FromPort: 5432, ToPort: 5432, CIDRs: []string{"0.0.0.0/0"},
	})

	drifts := c.CompareSecurityGroups([]events.SecurityGroupState{live}, []events.SecurityGroupState{base})
	require.Len(t, drifts, 1)
	assert.Equal(t, events.ChangeTypeIngressAdded, drifts[0].ChangeType)
	assert.Equal(t, "sg-abc123", drifts[0].ResourceID)
	assert.Equal(t, "aws_security_group", drifts[0].ResourceType)
	assert.Equal(t, "aws", drifts[0].Cloud)
}

func TestCompareSecurityGroups_IngressRemoved(t *testing.T) {
	c := state.NewComparator()
	base := baselineSG()
	live := events.SecurityGroupState{
		GroupID:      "sg-abc123",
		GroupName:    "infraguard-app-sg",
		IngressRules: []events.SGRule{}, // all rules stripped
		EgressRules:  base.EgressRules,
	}

	drifts := c.CompareSecurityGroups([]events.SecurityGroupState{live}, []events.SecurityGroupState{base})
	require.Len(t, drifts, 1)
	assert.Equal(t, events.ChangeTypeIngressRemoved, drifts[0].ChangeType)
}

func TestCompareSecurityGroups_NewResourceNotBaselined_Skipped(t *testing.T) {
	c := state.NewComparator()
	newSG := events.SecurityGroupState{GroupID: "sg-brand-new", IngressRules: []events.SGRule{}}

	drifts := c.CompareSecurityGroups(
		[]events.SecurityGroupState{newSG},
		[]events.SecurityGroupState{}, // empty baseline
	)
	assert.Empty(t, drifts, "resources not in baseline are new, not drift")
}

func TestCompareS3Buckets_PublicAccessDisabled(t *testing.T) {
	c := state.NewComparator()

	baseline := events.S3BucketState{
		BucketName:            "infraguard-artifacts-dev",
		BlockPublicAcls:       true,
		BlockPublicPolicy:     true,
		IgnorePublicAcls:      true,
		RestrictPublicBuckets: true,
	}

	// Simulate: someone disabled block_public_acls
	live := baseline
	live.BlockPublicAcls = false

	drifts := c.CompareS3Buckets(live, baseline)
	require.Len(t, drifts, 1)
	assert.Equal(t, events.ChangeTypePublicAccess, drifts[0].ChangeType)
	assert.Equal(t, "aws_s3_bucket", drifts[0].ResourceType)
	assert.Equal(t, "infraguard-artifacts-dev", drifts[0].ResourceID)
}

func TestCompareS3Buckets_NoChange(t *testing.T) {
	c := state.NewComparator()
	bucket := events.S3BucketState{
		BucketName: "infraguard-artifacts-dev",
		BlockPublicAcls: true, BlockPublicPolicy: true,
		IgnorePublicAcls: true, RestrictPublicBuckets: true,
	}

	drifts := c.CompareS3Buckets(bucket, bucket)
	assert.Empty(t, drifts)
}
