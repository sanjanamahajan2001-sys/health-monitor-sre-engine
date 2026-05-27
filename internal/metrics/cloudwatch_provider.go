package metrics

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"health-monitor/internal/output"
)

// CloudWatchProvider fetches metrics from AWS CloudWatch
type CloudWatchProvider struct {
	client *cloudwatch.Client
	region string
}

// NewCloudWatchProvider creates a new CloudWatch provider
func NewCloudWatchProvider(region string) (*CloudWatchProvider, error) {
	sdkConfig, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("failed to load aws config: %w", err)
	}

	return &CloudWatchProvider{
		client: cloudwatch.NewFromConfig(sdkConfig),
		region: region,
	}, nil
}

// QueryInstant implements the MetricProvider interface
func (p *CloudWatchProvider) QueryInstant(query string) (float64, error) {
	// Simple mapping for demonstration
	// In production, this would parse the Prometheus query and map to CloudWatch GetMetricData
	output.Debugf("CloudWatchProvider: QueryInstant: %s", query)
	
	// Example: sum(up{service="my-service"}) -> map to a known metric
	return 1.0, nil // Mocked for now
}

// QueryVector implements the MetricProvider interface
func (p *CloudWatchProvider) QueryVector(query string) ([]MetricSample, error) {
	output.Debugf("CloudWatchProvider: QueryVector: %s", query)
	return []MetricSample{}, nil // Mocked for now
}

// Check implements the MetricProvider interface
func (p *CloudWatchProvider) Check() (string, error) {
	// Perform a simple API call to verify connection
	_, err := p.client.DescribeAlarms(context.TODO(), &cloudwatch.DescribeAlarmsInput{
		MaxRecords: aws.Int32(1),
	})
	if err != nil {
		return "", fmt.Errorf("cloudwatch connection failed: %w", err)
	}
	return "OK", nil
}

// FetchContainerInsightMetric is a helper for native CloudWatch metric fetching
func (p *CloudWatchProvider) FetchContainerInsightMetric(namespace, cluster, metric string, period int32) (float64, error) {
	now := time.Now()
	startTime := now.Add(-time.Duration(period) * time.Second)

	input := &cloudwatch.GetMetricDataInput{
		StartTime: aws.Time(startTime),
		EndTime:   aws.Time(now),
		MetricDataQueries: []types.MetricDataQuery{
			{
				Id: aws.String("m1"),
				MetricStat: &types.MetricStat{
					Metric: &types.Metric{
						Namespace:  aws.String("ContainerInsights"),
						MetricName: aws.String(metric),
						Dimensions: []types.Dimension{
							{Name: aws.String("ClusterName"), Value: aws.String(cluster)},
							{Name: aws.String("Namespace"), Value: aws.String(namespace)},
						},
					},
					Period: aws.Int32(period),
					Stat:   aws.String("Average"),
				},
			},
		},
	}

	resp, err := p.client.GetMetricData(context.TODO(), input)
	if err != nil {
		return 0, err
	}

	if len(resp.MetricDataResults) > 0 && len(resp.MetricDataResults[0].Values) > 0 {
		return resp.MetricDataResults[0].Values[0], nil
	}

	return 0, fmt.Errorf("no data found")
}
