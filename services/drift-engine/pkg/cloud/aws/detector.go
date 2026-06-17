package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"

	"github.com/infraguard/drift-engine/pkg/events"
)

// Detector reads live AWS resource state via LocalStack
type Detector struct {
	ec2Client *ec2.Client
	s3Client  *s3.Client
	log       *zap.Logger
	region    string
}

// NewDetector creates a detector pointed at the given endpoint
// (LocalStack: http://localhost:4566)
func NewDetector(accessKey, secretKey, region, endpoint string, log *zap.Logger) (*Detector, error) {
	customResolver := func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{URL: endpoint, HostnameImmutable: true}, nil
	}

	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(region),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		),
		config.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(customResolver)),
	)
	if err != nil {
		return nil, fmt.Errorf("aws: load config: %w", err)
	}

	ec2Client := ec2.NewFromConfig(cfg, func(o *ec2.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})

	s3Client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true // required for LocalStack S3
	})

	return &Detector{
		ec2Client: ec2Client,
		s3Client:  s3Client,
		log:       log,
		region:    region,
	}, nil
}

// GetAllSecurityGroups returns canonical state for every security group
func (d *Detector) GetAllSecurityGroups(ctx context.Context) ([]events.SecurityGroupState, error) {
	resp, err := d.ec2Client.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{})
	if err != nil {
		return nil, fmt.Errorf("aws: describe security groups: %w", err)
	}

	var out []events.SecurityGroupState
	for _, sg := range resp.SecurityGroups {
		state := events.SecurityGroupState{
			GroupID:     aws.ToString(sg.GroupId),
			GroupName:   aws.ToString(sg.GroupName),
			VPCID:       aws.ToString(sg.VpcId),
			Description: aws.ToString(sg.Description),
			Tags:        make(map[string]string),
		}

		for _, tag := range sg.Tags {
			state.Tags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
		}

		for _, rule := range sg.IpPermissions {
			r := events.SGRule{
				Protocol: aws.ToString(rule.IpProtocol),
				FromPort: aws.ToInt32(rule.FromPort),
				ToPort:   aws.ToInt32(rule.ToPort),
			}
			for _, cidr := range rule.IpRanges {
				r.CIDRs = append(r.CIDRs, aws.ToString(cidr.CidrIp))
			}
			for _, cidr6 := range rule.Ipv6Ranges {
				r.IPv6CIDRs = append(r.IPv6CIDRs, aws.ToString(cidr6.CidrIpv6))
			}
			state.IngressRules = append(state.IngressRules, r)
		}

		for _, rule := range sg.IpPermissionsEgress {
			r := events.SGRule{
				Protocol: aws.ToString(rule.IpProtocol),
				FromPort: aws.ToInt32(rule.FromPort),
				ToPort:   aws.ToInt32(rule.ToPort),
			}
			for _, cidr := range rule.IpRanges {
				r.CIDRs = append(r.CIDRs, aws.ToString(cidr.CidrIp))
			}
			state.EgressRules = append(state.EgressRules, r)
		}

		out = append(out, state)
	}

	d.log.Debug("fetched security groups", zap.Int("count", len(out)))
	return out, nil
}

// GetSecurityGroupByName returns the state of one security group, or nil if not found
func (d *Detector) GetSecurityGroupByName(ctx context.Context, name string) (*events.SecurityGroupState, error) {
	all, err := d.GetAllSecurityGroups(ctx)
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].GroupName == name {
			return &all[i], nil
		}
	}
	return nil, nil
}

// GetS3BucketState returns the canonical state of an S3 bucket's public access settings
func (d *Detector) GetS3BucketState(ctx context.Context, bucketName string) (*events.S3BucketState, error) {
	state := &events.S3BucketState{BucketName: bucketName}

	// Public access block
	pab, err := d.s3Client.GetPublicAccessBlock(ctx, &s3.GetPublicAccessBlockInput{
		Bucket: aws.String(bucketName),
	})
	if err == nil && pab.PublicAccessBlockConfiguration != nil {
		cfg := pab.PublicAccessBlockConfiguration
		state.BlockPublicAcls = aws.ToBool(cfg.BlockPublicAcls)
		state.BlockPublicPolicy = aws.ToBool(cfg.BlockPublicPolicy)
		state.IgnorePublicAcls = aws.ToBool(cfg.IgnorePublicAcls)
		state.RestrictPublicBuckets = aws.ToBool(cfg.RestrictPublicBuckets)
	} else if err != nil {
		d.log.Warn("could not read public access block", zap.String("bucket", bucketName), zap.Error(err))
	}

	// Versioning
	ver, err := d.s3Client.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{
		Bucket: aws.String(bucketName),
	})
	if err == nil {
		state.VersioningEnabled = ver.Status == "Enabled"
	}

	return state, nil
}
