package discovery

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sync"
	"github.com/aws/aws-sdk-go-v2/aws"
	aws_config "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/smithy-go/logging"
	"health-monitor/pkg/model"
)

// K8sConfig holds the configuration for Kubernetes discovery
type K8sConfig struct {
	Enabled    bool
	KubeConfig  string
	KubeContext string
	AWSProfile  string
	Region      string
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		// If running under sudo, use SUDO_USER's home directory if available
		if os.Geteuid() == 0 {
			if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
				if u, err := user.Lookup(sudoUser); err == nil && u.HomeDir != "" {
					return filepath.Join(u.HomeDir, path[2:])
				}
			}
		}

		dirname, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(dirname, path[2:])
		}
	}
	return path
}

type resourceInfo struct {
	Namespace string
	Type      string
	Timestamp time.Time
}

// DiscoveryEngine handles auto-discovery for K8s and EKS resources
type DiscoveryEngine struct {
	client    *kubernetes.Clientset
	eksClient *eks.Client
	config    K8sConfig
	
	cacheMu       sync.RWMutex
	resourceCache map[string]resourceInfo
}

// NewDiscoveryEngine creates a new discovery engine
func (e *DiscoveryEngine) GetConfig() K8sConfig {
	return e.config
}

func NewDiscoveryEngine(cfg K8sConfig) (*DiscoveryEngine, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("EKS/Kubernetes monitoring is not enabled")
	}

	kubeconfig := cfg.KubeConfig
	if kubeconfig == "" {
		home, _ := os.UserHomeDir()
		kubeconfig = filepath.Join(home, ".kube", "config")
	} else {
		kubeconfig = expandPath(kubeconfig)
	}

	// Load the kubeconfig and specifically select the context if provided
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.ExplicitPath = kubeconfig
	
	configOverrides := &clientcmd.ConfigOverrides{}
	if cfg.KubeContext != "" {
		configOverrides.CurrentContext = cfg.KubeContext
	}
	
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig (context: %q): %w", cfg.KubeContext, err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Initialize EKS client if region is provided
	var eksClient *eks.Client
	if cfg.Region != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		opts := []func(*aws_config.LoadOptions) error{
			aws_config.WithRegion(cfg.Region),
			aws_config.WithLogger(logging.LoggerFunc(func(classification logging.Classification, format string, v ...interface{}) {})),
		}

		if cfg.AWSProfile != "" {
			opts = append(opts, aws_config.WithSharedConfigProfile(cfg.AWSProfile))
		}

		// If running under sudo, ensure AWS SDK knows where to find the user's credentials
		if os.Geteuid() == 0 {
			if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
				if u, err := user.Lookup(sudoUser); err == nil && u.HomeDir != "" {
					// Don't override if already set, but provide defaults for the original user
					if os.Getenv("AWS_SHARED_CREDENTIALS_FILE") == "" {
						os.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(u.HomeDir, ".aws", "credentials"))
					}
					if os.Getenv("AWS_CONFIG_FILE") == "" {
						os.Setenv("AWS_CONFIG_FILE", filepath.Join(u.HomeDir, ".aws", "config"))
					}
					if cfg.AWSProfile != "" && os.Getenv("AWS_PROFILE") == "" {
						os.Setenv("AWS_PROFILE", cfg.AWSProfile)
					}
					if cfg.Region != "" && os.Getenv("AWS_REGION") == "" {
						os.Setenv("AWS_REGION", cfg.Region)
					}
					// Some AWS helpers (like get-token) strictly depend on HOME or specific config paths
					if os.Getenv("HOME") == "" || os.Getenv("HOME") == "/root" {
						os.Setenv("HOME", u.HomeDir)
					}
				}
			}
		}

		sdkConfig, err := aws_config.LoadDefaultConfig(ctx, opts...)
		if err == nil {
			eksClient = eks.NewFromConfig(sdkConfig)
		} else {
			// Log but don't fail hard - we might still have K8s access via kubeconfig directly
			fmt.Fprintf(os.Stderr, "⚠️  AWS configuration failed: %v. EKS-specific features may be limited.\n", err)
		}
	}

	return &DiscoveryEngine{
		client:        clientset,
		eksClient:     eksClient,
		config:        cfg,
		resourceCache: make(map[string]resourceInfo),
	}, nil
}

// GetCurrentContext returns the current context name from kubeconfig
func (e *DiscoveryEngine) GetCurrentContext() string {
	kubeconfig := e.config.KubeConfig
	if kubeconfig == "" {
		home, _ := os.UserHomeDir()
		kubeconfig = filepath.Join(home, ".kube", "config")
	} else {
		kubeconfig = expandPath(kubeconfig)
	}

	config, err := clientcmd.LoadFromFile(kubeconfig)
	if err != nil {
		return ""
	}
	return config.CurrentContext
}

// DiscoverEKSCluster retrieves metadata for an EKS cluster
func (e *DiscoveryEngine) DiscoverEKSCluster(name string) (*eks.DescribeClusterOutput, error) {
	if e.eksClient == nil {
		return nil, fmt.Errorf("EKS client not initialized (check region)")
	}

	input := &eks.DescribeClusterInput{
		Name: aws.String(name),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return e.eksClient.DescribeCluster(ctx, input)
}

// DiscoverNamespaces finds all accessible namespaces
func (e *DiscoveryEngine) DiscoverNamespaces() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	nsList, err := e.client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	var namespaces []string
	for _, ns := range nsList.Items {
		namespaces = append(namespaces, ns.Name)
	}
	return namespaces, nil
}

// DiscoverServices finds services in the specified namespaces using a worker pool for concurrency
func (e *DiscoveryEngine) DiscoverServices(namespaces []string) ([]string, error) {
	if len(namespaces) == 0 {
		return nil, nil
	}

	workerCount := 10
	if len(namespaces) < workerCount {
		workerCount = len(namespaces)
	}

	type result struct {
		services []string
		err      error
	}

	nsChan := make(chan string, len(namespaces))
	resChan := make(chan result, len(namespaces))
	var wg sync.WaitGroup

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start workers
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ns := range nsChan {
				svcList, err := e.client.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
				if err != nil {
					resChan <- result{err: err}
					continue
				}
				var found []string
				for _, svc := range svcList.Items {
					found = append(found, fmt.Sprintf("%s/%s", ns, svc.Name))
				}
				resChan <- result{services: found}
			}
		}()
	}

	// Feed namespaces to workers
	for _, ns := range namespaces {
		nsChan <- ns
	}
	close(nsChan)

	// Wait for workers in a separate goroutine
	go func() {
		wg.Wait()
		close(resChan)
	}()

	var allServices []string
	for res := range resChan {
		if res.err != nil {
			// We log and continue for individual namespace errors to remain resilient
			continue
		}
		allServices = append(allServices, res.services...)
	}

	return allServices, nil
}

// DiscoverEvents fetches recent events for a given resource
func (e *DiscoveryEngine) DiscoverEvents(namespace, name string) ([]string, error) {
	eventList, err := e.client.CoreV1().Events(namespace).List(context.TODO(), metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s", name),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list events: %w", err)
	}

	var events []string
	for _, event := range eventList.Items {
		events = append(events, fmt.Sprintf("[%s] %s: %s", event.LastTimestamp.Format("15:04:05"), event.Reason, event.Message))
	}
	return events, nil
}

// GetNodeHealth returns a summary of node readiness
func (e *DiscoveryEngine) GetNodeHealth() (struct{ Ready, Total int }, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	nodes, err := e.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return struct{ Ready, Total int }{}, err
	}

	status := struct{ Ready, Total int }{Total: len(nodes.Items)}
	for _, node := range nodes.Items {
		for _, cond := range node.Status.Conditions {
			if cond.Type == "Ready" && cond.Status == "True" {
				status.Ready++
				break
			}
		}
	}
	return status, nil
}

// GetNamespaceHealth returns health stats for a specific namespace
func (e *DiscoveryEngine) GetNamespaceHealth(ns string) (model.NamespaceHealth, []model.WorkloadHealth, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	pods, err := e.client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return model.NamespaceHealth{}, nil, err
	}

	health := model.NamespaceHealth{Name: ns, PodsTotal: len(pods.Items), Status: model.SAFE}
	for _, pod := range pods.Items {
		podReady := false
		for _, cond := range pod.Status.Conditions {
			if cond.Type == "Ready" && cond.Status == "True" {
				podReady = true
				break
			}
		}
		if podReady {
			health.PodsReady++
		}
		for _, container := range pod.Status.ContainerStatuses {
			health.Restarts += int(container.RestartCount)
		}
	}

	if health.PodsReady < health.PodsTotal {
		health.Status = model.RISK
	} else if health.Restarts > 0 {
		health.Status = model.CHECK
	}

	// Fetch Workloads (Deployments)
	var workloads []model.WorkloadHealth
	deps, err := e.client.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, d := range deps.Items {
			status := model.SAFE
			if d.Status.ReadyReplicas < d.Status.Replicas {
				status = model.RISK
			}
			workloads = append(workloads, model.WorkloadHealth{
				Name:      d.Name,
				Namespace: ns,
				Type:      "Deployment",
				Ready:     int(d.Status.ReadyReplicas),
				Desired:   int(d.Status.Replicas),
				Status:    status,
			})
		}
	}

	return health, workloads, nil
}

// FindNamespaceForResource searches all namespaces for a workload with the given name
// It prioritizes preferred namespaces and caches results for performance
func (e *DiscoveryEngine) FindNamespaceForResource(name string, preferredNamespaces []string) (string, string, error) {
	// 1. Check Cache (TTL 5 minutes)
	e.cacheMu.RLock()
	if info, ok := e.resourceCache[name]; ok && time.Since(info.Timestamp) < 5*time.Minute {
		e.cacheMu.RUnlock()
		return info.Namespace, info.Type, nil
	}
	e.cacheMu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Helper to check namespaces in priority order
	searchWorkload := func(targetNs string) (string, string) {
		if targetNs == "" {
			// Search All Namespaces using List with FieldSelector
			selector := fmt.Sprintf("metadata.name=%s", name)
			
			// Deployments
			if list, err := e.client.AppsV1().Deployments("").List(ctx, metav1.ListOptions{FieldSelector: selector}); err == nil && len(list.Items) > 0 {
				return list.Items[0].Namespace, "Deployment"
			}
			// StatefulSets
			if list, err := e.client.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{FieldSelector: selector}); err == nil && len(list.Items) > 0 {
				return list.Items[0].Namespace, "StatefulSet"
			}
			// DaemonSets
			if list, err := e.client.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{FieldSelector: selector}); err == nil && len(list.Items) > 0 {
				return list.Items[0].Namespace, "DaemonSet"
			}
			// Services
			if list, err := e.client.CoreV1().Services("").List(ctx, metav1.ListOptions{FieldSelector: selector}); err == nil && len(list.Items) > 0 {
				return list.Items[0].Namespace, "Service"
			}
			// Jobs
			if list, err := e.client.BatchV1().Jobs("").List(ctx, metav1.ListOptions{FieldSelector: selector}); err == nil && len(list.Items) > 0 {
				return list.Items[0].Namespace, "Job"
			}
		} else {
			// Directed search in specific namespace
			if d, err := e.client.AppsV1().Deployments(targetNs).Get(ctx, name, metav1.GetOptions{}); err == nil {
				return d.Namespace, "Deployment"
			}
			if ss, err := e.client.AppsV1().StatefulSets(targetNs).Get(ctx, name, metav1.GetOptions{}); err == nil {
				return ss.Namespace, "StatefulSet"
			}
			if ds, err := e.client.AppsV1().DaemonSets(targetNs).Get(ctx, name, metav1.GetOptions{}); err == nil {
				return ds.Namespace, "DaemonSet"
			}
			if svc, err := e.client.CoreV1().Services(targetNs).Get(ctx, name, metav1.GetOptions{}); err == nil {
				return svc.Namespace, "Service"
			}
			if j, err := e.client.BatchV1().Jobs(targetNs).Get(ctx, name, metav1.GetOptions{}); err == nil {
				return j.Namespace, "Job"
			}
		}
		return "", ""
	}

	// 2. Search Preferred Namespaces
	for _, ns := range preferredNamespaces {
		if foundNs, foundType := searchWorkload(ns); foundNs != "" {
			e.updateCache(name, foundNs, foundType)
			return foundNs, foundType, nil
		}
	}

	// 3. Search All Namespaces (if not found in preferred)
	if foundNs, foundType := searchWorkload(""); foundNs != "" {
		e.updateCache(name, foundNs, foundType)
		return foundNs, foundType, nil
	}

	return "", "", fmt.Errorf("workload %q not found in any namespace", name)
}

func (e *DiscoveryEngine) updateCache(name, ns, resType string) {
	e.cacheMu.Lock()
	defer e.cacheMu.Unlock()
	e.resourceCache[name] = resourceInfo{
		Namespace: ns,
		Type:      resType,
		Timestamp: time.Now(),
	}
}

// DiscoverPodEvents fetches events for pods matching the given workload name and namespace
func (e *DiscoveryEngine) DiscoverPodEvents(namespace, name string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Find pods by label (common across deployments/statefulsets/daemonsets)
	// We check for several common label keys
	labelSelectors := []string{
		fmt.Sprintf("app=%s", name),
		fmt.Sprintf("name=%s", name),
		fmt.Sprintf("app.kubernetes.io/name=%s", name),
	}

	var allEvents []string
	seenEvents := make(map[string]bool)

	for _, selector := range labelSelectors {
		pods, err := e.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: selector,
		})
		if err != nil || len(pods.Items) == 0 {
			continue
		}

		for _, pod := range pods.Items {
			eventList, err := e.client.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
				FieldSelector: fmt.Sprintf("involvedObject.name=%s", pod.Name),
			})
			if err != nil {
				continue
			}

			for _, event := range eventList.Items {
				// Avoid duplicates if labels overlap
				evtKey := fmt.Sprintf("%s:%s:%s", event.Reason, event.Message, pod.Name)
				if seenEvents[evtKey] {
					continue
				}
				seenEvents[evtKey] = true
				
				allEvents = append(allEvents, fmt.Sprintf("[%s] Pod/%s %s: %s", 
					event.LastTimestamp.Format("15:04:05"), pod.Name, event.Reason, event.Message))
			}
		}
	}

	return allEvents, nil
}
