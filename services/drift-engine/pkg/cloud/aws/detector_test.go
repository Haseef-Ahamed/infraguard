package aws_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	awsdetector "github.com/infraguard/drift-engine/pkg/cloud/aws"
)

func newTestDetector(t *testing.T) *awsdetector.Detector {
	log := zap.NewNop()
	d, err := awsdetector.NewDetector("test", "test", "us-east-1", "http://localhost:4566", log)
	require.NoError(t, err)
	return d
}

func TestGetAllSecurityGroups_FindsAppSG(t *testing.T) {
	d := newTestDetector(t)

	// Use a timeout so the test never hangs
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	groups, err := d.GetAllSecurityGroups(ctx)
	if err != nil {
		t.Skipf("LocalStack not available: %v", err)
	}

	require.NotEmpty(t, groups, "expected at least one security group")

	var found bool
	for _, sg := range groups {
		if sg.GroupName == "infraguard-app-sg" {
			found = true
			require.Len(t, sg.IngressRules, 1)
			assert.Equal(t, int32(443), sg.IngressRules[0].FromPort)
			assert.Equal(t, int32(443), sg.IngressRules[0].ToPort)
			assert.Contains(t, sg.IngressRules[0].CIDRs, "0.0.0.0/0")
		}
	}
	assert.True(t, found, "infraguard-app-sg not found — was tofu apply run?")
}

func TestGetSecurityGroupByName_NotFound(t *testing.T) {
	d := newTestDetector(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sg, err := d.GetSecurityGroupByName(ctx, "does-not-exist-sg")
	if err != nil {
		t.Skipf("LocalStack not available: %v", err)
	}
	assert.Nil(t, sg)
}

func TestGetS3BucketState_PublicAccessBlocked(t *testing.T) {
	d := newTestDetector(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	state, err := d.GetS3BucketState(ctx, "infraguard-artifacts-dev")
	if err != nil {
		t.Skipf("LocalStack not available: %v", err)
	}

	require.NotNil(t, state)
	assert.True(t, state.BlockPublicAcls)
	assert.True(t, state.BlockPublicPolicy)
	assert.True(t, state.IgnorePublicAcls)
	assert.True(t, state.RestrictPublicBuckets)
}
