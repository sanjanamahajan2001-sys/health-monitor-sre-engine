package infra

import (
	"fmt"
	"math"

	"health-monitor/internal/analyse/prometheus"
	"health-monitor/internal/config"
	"health-monitor/pkg/model"
)

// CollectClusterMetrics fetches cluster-wide metrics like CPU, Memory, Disk, and Network
func CollectClusterMetrics(client *prometheus.Client, cfg config.Config) (*model.ClusterMetrics, error) {
	if client == nil {
		return nil, fmt.Errorf("prometheus client not initialized")
	}

	metrics := &model.ClusterMetrics{}

	// 1. CPU Usage
	// Query: sum(rate(node_cpu_seconds_total{mode!="idle"}[5m])) / sum(rate(node_cpu_seconds_total[5m])) * 100
	cpuQuery := `sum(rate(node_cpu_seconds_total{mode!="idle"}[5m])) / sum(rate(node_cpu_seconds_total[5m])) * 100`
	if val, err := client.QueryInstant(cpuQuery); err == nil {
		metrics.CPUUsage = val
	} else {
		// Fallback for K8s/cAdvisor style if node_exporter is not available
		cpuQuery = `sum(rate(container_cpu_usage_seconds_total{container!=""}[5m])) / sum(kube_node_status_allocatable{resource="cpu"}) * 100`
		if val, err := client.QueryInstant(cpuQuery); err == nil {
			metrics.CPUUsage = val
		}
	}

	// 2. Memory Usage
	// Query: (node_memory_MemTotal_bytes - node_memory_MemAvailable_bytes) / node_memory_MemTotal_bytes * 100
	memQuery := `(sum(node_memory_MemTotal_bytes) - sum(node_memory_MemAvailable_bytes)) / sum(node_memory_MemTotal_bytes) * 100`
	if val, err := client.QueryInstant(memQuery); err == nil {
		metrics.MemoryUsage = val
	} else {
		// Fallback for K8s
		memQuery = `sum(container_memory_usage_bytes{container!=""}) / sum(kube_node_status_allocatable{resource="memory"}) * 100`
		if val, err := client.QueryInstant(memQuery); err == nil {
			metrics.MemoryUsage = val
		}
	}

	// 3. Disk Usage
	// Query: (node_filesystem_size_bytes{mountpoint="/"} - node_filesystem_free_bytes{mountpoint="/"}) / node_filesystem_size_bytes{mountpoint="/"} * 100
	diskQuery := `sum(node_filesystem_size_bytes{mountpoint="/"}) - sum(node_filesystem_free_bytes{mountpoint="/"}) / sum(node_filesystem_size_bytes{mountpoint="/"}) * 100`
	if val, err := client.QueryInstant(diskQuery); err == nil {
		metrics.DiskUsage = val
	}

	// 4. Network In/Out
	// Query: sum(rate(node_network_receive_bytes_total[5m]))
	netInQuery := `sum(rate(node_network_receive_bytes_total{device!~"lo"}[5m])) / 1024`
	if val, err := client.QueryInstant(netInQuery); err == nil {
		metrics.NetworkInRate = val
	}

	netOutQuery := `sum(rate(node_network_transmit_bytes_total{device!~"lo"}[5m])) / 1024`
	if val, err := client.QueryInstant(netOutQuery); err == nil {
		metrics.NetworkOutRate = val
	}

	// 5. Node Status
	nodeTotalQuery := `count(kube_node_info)`
	if val, err := client.QueryInstant(nodeTotalQuery); err == nil {
		metrics.NodesTotal = int(val)
	}
	nodeReadyQuery := `sum(kube_node_status_condition{condition="Ready", status="true"})`
	if val, err := client.QueryInstant(nodeReadyQuery); err == nil {
		metrics.NodesReady = int(val)
	}

	// 6. Top Namespaces (CPU)
	topNSQuery := `topk(5, sum(rate(container_cpu_usage_seconds_total{container!=""}[5m])) by (namespace))`
	if results, err := client.QueryVector(topNSQuery); err == nil {
		for _, res := range results {
			ns := model.NamespaceUsage{
				Name:     res.Metric["namespace"],
				CPUUsage: res.Value,
			}
			// Also grab memory for this namespace
			memQuery := fmt.Sprintf(`sum(container_memory_usage_bytes{namespace="%s", container!=""})`, ns.Name)
			if mVal, err := client.QueryInstant(memQuery); err == nil {
				ns.MemoryUsage = mVal / (1024 * 1024) // MB
			}
			metrics.TopNamespaces = append(metrics.TopNamespaces, ns)
		}
	}

	// Sanitize NaNs
	if math.IsNaN(metrics.CPUUsage) { metrics.CPUUsage = 0 }
	if math.IsNaN(metrics.MemoryUsage) { metrics.MemoryUsage = 0 }
	if math.IsNaN(metrics.DiskUsage) { metrics.DiskUsage = 0 }
	if math.IsNaN(metrics.NetworkInRate) { metrics.NetworkInRate = 0 }
	if math.IsNaN(metrics.NetworkOutRate) { metrics.NetworkOutRate = 0 }

	return metrics, nil
}
