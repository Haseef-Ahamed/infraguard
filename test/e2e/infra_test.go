package e2e

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	ec2Types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newEC2Client(t *testing.T) *ec2.Client {
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err)
	return ec2.NewFromConfig(cfg, func(o *ec2.Options) {
		o.BaseEndpoint = aws.String("http://localhost:4566")
	})
}

// TestInfraBaseline_SecurityGroupExists proves the IaC baseline is applied
// and the security group InfraGuard monitors exists with the correct rules.
func TestInfraBaseline_SecurityGroupExists(t *testing.T) {
	client := newEC2Client(t)
	ctx := context.Background()

	resp, err := client.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		Filters: []ec2Types.Filter{{Name: aws.String("group-name"), Values: []string{"infraguard-app-sg"}}},
	})
	require.NoError(t, err)
	require.Len(t, resp.SecurityGroups, 1, "infraguard-app-sg must exist — run tofu apply first")

	sg := resp.SecurityGroups[0]
	require.Len(t, sg.IpPermissions, 1, "baseline SG should have exactly one ingress rule")
	assert.Equal(t, int32(443), aws.ToInt32(sg.IpPermissions[0].FromPort))
	assert.Equal(t, int32(443), aws.ToInt32(sg.IpPermissions[0].ToPort))
}

// TestInfraBaseline_NoUnauthorizedPorts proves no drift is currently present
// (this should pass on a clean environment, fail if drift wasn't reverted)
func TestInfraBaseline_NoUnauthorizedPorts(t *testing.T) {
	client := newEC2Client(t)
	ctx := context.Background()

	resp, err := client.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		Filters: []ec2Types.Filter{{Name: aws.String("group-name"), Values: []string{"infraguard-app-sg"}}},
	})
	require.NoError(t, err)
	require.Len(t, resp.SecurityGroups, 1)

	sg := resp.SecurityGroups[0]
	for _, rule := range sg.IpPermissions {
		port := aws.ToInt32(rule.FromPort)
		assert.NotEqual(t, int32(5432), port, "port 5432 should not be open — drift was not reverted")
		assert.NotEqual(t, int32(22), port, "port 22 should not be open — drift was not reverted")
	}
}

// describeAppSG is a shared helper used across E2E tests
func describeAppSG(ctx context.Context, client *ec2.Client) (*ec2.DescribeSecurityGroupsOutput, error) {
	return client.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		Filters: []ec2Types.Filter{{Name: aws.String("group-name"), Values: []string{"infraguard-app-sg"}}},
	})
}
