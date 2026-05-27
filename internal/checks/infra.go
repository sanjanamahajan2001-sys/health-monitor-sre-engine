package checks

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	infra_analyse "health-monitor/internal/analyse/infra"
	"health-monitor/internal/analyse/prometheus"
	"health-monitor/internal/config"
	"health-monitor/internal/discovery"
	"health-monitor/pkg/model"
)

// Infra performs infrastructure health checks for Kubernetes and EKS
func Infra(report *model.Report) {
	cfg, err := config.Load()
	if err != nil {
		report.InfraStatus = &model.InfraStatus{
			NoDataReasons: []string{"Internal error: failed to load config"},
		}
		return
	}

	if !cfg.EKS.Enabled {
		return
	}

	// Check for aws CLI and credentials if needed
	if cfg.EKS.AutoDiscover || strings.Contains(cfg.EKS.KubeConfig, "aws") {
		config.DebugLog("Infra: checking for aws CLI")
		if _, err := exec.LookPath("aws"); err != nil {
			report.InfraStatus = &model.InfraStatus{
				NoDataReasons: []string{
					"Infrastructure Health check requires the 'aws' CLI to be installed and in your PATH for authentication.",
					"Please install the AWS CLI and ensure it is accessible.",
				},
			}
			return
		}

		// Proactively check for valid credentials using a timeout.
		// Increased to 15s as 5s was causing "signal: killed" on slower networks/local environments.
		importCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		config.DebugLog("Infra: running aws sts get-caller-identity")
		if out, err := exec.CommandContext(importCtx, "aws", "sts", "get-caller-identity").CombinedOutput(); err != nil {
			config.DebugLog("Infra: aws sts failed: %v", err)
			reason := "AWS authentication failed. Please run 'aws configure' or check your credentials."
			
			if importCtx.Err() == context.DeadlineExceeded {
				reason = "AWS authentication timed out (15s). Check your internet connection or AWS SSO session."
			} else {
				outStr := string(out)
				if strings.Contains(outStr, "ExpiredToken") {
					reason = "AWS session has expired. Please log in again (e.g., 'aws sso login')."
				} else if strings.Contains(outStr, "NoCredentials") || strings.Contains(outStr, "Unable to locate credentials") || strings.Contains(err.Error(), "exit status 253") {
					reason = "AWS credentials not found. Sudo resets your $HOME to /root; use 'sudo -E health-monitor' to preserve your ~/.aws/credentials."
				} else if len(outStr) > 0 {
					reason = fmt.Sprintf("AWS error: %s", strings.TrimSpace(outStr))
				}
			}
			report.InfraStatus = &model.InfraStatus{
				NoDataReasons: []string{reason},
			}
			return
		}
		config.DebugLog("Infra: aws sts success")
	}

	config.DebugLog("Infra: initializing discovery engine for context: %s", cfg.EKS.KubeContext)

	engine, err := discovery.NewDiscoveryEngine(discovery.K8sConfig{
		Enabled:    true,
		KubeConfig: cfg.EKS.KubeConfig,
		Region:     cfg.EKS.Region,
	})

	if err != nil {
		reason := fmt.Sprintf("K8s/EKS discovery failed: %v", err)
		if strings.Contains(err.Error(), "executable file not found") {
			reason = "Kubernetes authentication failed: a required credential plugin (e.g., aws-iam-authenticator) was not found in your PATH."
		} else if strings.Contains(err.Error(), "NoCredentials") {
			reason = "AWS credentials not found. Please run 'aws configure' or set AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY."
		}

		report.InfraStatus = &model.InfraStatus{
			NoDataReasons: []string{reason},
		}
		return
	}

	clusterName := cfg.EKS.KubeContext
	if clusterName == "" {
		clusterName = engine.GetCurrentContext()
	}
	
	// Extract just the cluster name from full ARN if needed
	clusterName = extractClusterName(clusterName)

	infra := &model.InfraStatus{
		ClusterName: clusterName,
		Provider:    cfg.Provider,
		Region:      cfg.EKS.Region,
	}

	// 1. Check Node Health
	nodes, err := engine.GetNodeHealth()
	if err == nil {
		infra.NodesReady = nodes.Ready
		infra.NodesTotal = nodes.Total
	} else {
		infra.NoDataReasons = append(infra.NoDataReasons, fmt.Sprintf("Node health failed: %v", err))
	}

	// 2. Check Pod/Workload Health in monitored namespaces
	namespaces := cfg.EKS.Namespaces
	if len(namespaces) == 0 {
		// Auto-discover all namespaces if none specified
		config.DebugLog("Infra: no namespaces configured, discovering all")
		discovered, err := engine.DiscoverNamespaces()
		if err == nil && len(discovered) > 0 {
			namespaces = discovered
			config.DebugLog("Infra: auto-discovered %d namespaces", len(namespaces))
		} else {
			config.DebugLog("Infra: namespace discovery failed or empty, defaulting to 'default'")
			namespaces = []string{"default"}
		}
	}

	var totalPods, readyPods, totalRestarts int
	for _, ns := range namespaces {
		nsHealth, workloads, err := engine.GetNamespaceHealth(ns)
		if err != nil {
			infra.NoDataReasons = append(infra.NoDataReasons, fmt.Sprintf("Namespace %s health failed: %v", ns, err))
			continue
		}
		infra.NamespaceHealth = append(infra.NamespaceHealth, nsHealth)
		infra.WorkloadHealth = append(infra.WorkloadHealth, workloads...)

		totalPods += nsHealth.PodsTotal
		readyPods += nsHealth.PodsReady
		totalRestarts += nsHealth.Restarts
	}

	infra.PodsTotal = totalPods
	infra.PodsReady = readyPods
	infra.Restarts = totalRestarts

	// 3. Optional: Fetch Cluster Metrics (CPU/Mem/Disk) from Prometheus
	if cfg.PrometheusURL != "" {
		promClient := &prometheus.Client{
			BaseURL: cfg.PrometheusURL,
			Token:   cfg.PrometheusToken,
			User:    cfg.PrometheusUser,
			Pass:    cfg.PrometheusPass,
			Timeout: 5 * time.Second,
			QPS:     cfg.PrometheusQPS,
		}
		if metricsAdmin, err := infra_analyse.CollectClusterMetrics(promClient, cfg); err == nil {
			infra.ClusterMetrics = metricsAdmin
		} else {
			config.DebugLog("Infra: Cluster metrics collection failed: %v", err)
		}
	}

	report.InfraStatus = infra
}

// extractClusterName extracts just the cluster name from full ARN or context name
// Examples:
// - "arn:aws:eks:us-west-2:123456789012:cluster/my-cluster" -> "my-cluster"
// - "my-cluster" -> "my-cluster"
// - "gke_my-project_us-west-1_my-cluster" -> "my-cluster"
func extractClusterName(fullName string) string {
	if fullName == "" {
		return fullName
	}
	
	// Handle EKS ARN format: arn:aws:eks:region:account:cluster/cluster-name
	if strings.HasPrefix(fullName, "arn:aws:eks:") {
		parts := strings.Split(fullName, "/")
		if len(parts) >= 2 {
			return parts[len(parts)-1]
		}
	}
	
	// Handle GKE format: gke_project_zone_cluster
	if strings.HasPrefix(fullName, "gke_") {
		parts := strings.Split(fullName, "_")
		if len(parts) >= 4 {
			return parts[len(parts)-1]
		}
	}
	
	// Handle generic context format: cluster-name
	// If it contains path separators, take the last part
	if strings.Contains(fullName, "/") {
		parts := strings.Split(fullName, "/")
		return parts[len(parts)-1]
	}
	
	// Return as-is if no patterns match
	return fullName
}
