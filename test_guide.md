# EKS & Kubernetes Integration Test Guide

This guide provides step-by-step instructions for verifying the new EKS, Generic Kubernetes, and Bare Metal monitoring features.

## 1. Prerequisites
- **Go installed**: Ensure Go is installed and available in your path.
- **Kubeconfig**: A valid `~/.kube/config` file with at least one active cluster context.
- **AWS Credentials** (Optional): If testing EKS, ensure AWS credentials are configured (via `aws configure` or environment variables).

## 2. Dependency Setup
Since new dependencies were added, run the following command in your terminal:
```bash
go mod tidy
```

## 3. Verifying the Setup Wizard (TUI)
Test the interactive setup for different infrastructure types.

### Test A: Bare Metal / VM
1. Run the wizard: `./health-monitor --wizard-team`
2. Select **Bare Metal / VM**.
3. Complete the profile setup.
4. **Expected**: The wizard should skip Kubernetes/EKS specific questions and save a standard profile.

### Test B: Generic Kubernetes
1. Run the wizard: `./health-monitor --wizard-team`
2. Select **Generic Kubernetes**.
3. **Expected**: The wizard should prompt for `Kubeconfig Path` and `Cluster Context`.
4. Enter your details and complete the setup.
5. **Expected**: The profile should be created with `eks.enabled: true` but `cloudwatch_enabled: false`.

### Test C: Amazon EKS
1. Run the wizard: `./health-monitor --wizard-team --infra eks --region us-east-1`
2. **Expected**: The wizard should pre-populate the infrastructure type and AWS region.
3. Complete the setup.
4. **Expected**: The profile should have `eks.enabled: true` and `cloudwatch_enabled: true`.

### Test D: Validation & Connectivity Testing
1. Run the wizard and enter your K8s Context/EKS ARN, Prometheus, and Loki URLs.
2. When the **"Testing Connectivity"** screen appears:
   - **Expected**: The wizard should show real-time status (✅ or ❌) for Kubernetes connectivity.
   - **Expected**: For EKS, it should validate AWS connectivity and cluster presence.
   - **Expected**: The wizard should test Prometheus, Loki, Slack, and PagerDuty endpoints.
   - **Retry**: If a check fails, press `r` to retry after fixing the environment.
   - **Skip**: Press `s` to ignore a failed check and proceed to profile completion (use this for testing without a live cluster).

## 4. Verifying Dashboard Infrastructure Health
Verify that the agent dashboard correctly displays metadata discovered from EKS/Kubernetes.

1. Ensure you have successfully configured an EKS or Generic Kubernetes profile.
2. Start the health monitor dashboard:
   ```bash
   ./health-monitor
   ```
3. **Expected**: A new "🛡️ Infrastructure Health" section should appear on the dashboard.
4. **Validation**: It should display cluster summary cards (Nodes Ready, Pods Ready, Total Restarts).
5. **Validation**: It should list a "Namespace Health" table showing readiness and restarts per namespace.
6. **Validation**: It should show a "Top Workloads" table with deployment names, namespaces, and their current replica status.
The SLO service now pulls metrics dynamically based on your infrastructure.

1.  **Prometheus (Bare Metal/K8s)**:
    - Ensure your profile has a `PrometheusURL`.
    - Run: `./health-monitor slo list`
    - **Expected**: Metrics are fetched via the `PrometheusProvider`.
2.  **CloudWatch (EKS)**:
    - Ensure `eks.enabled` and `cloudwatch_enabled` are true in your profile.
    - Run: `./health-monitor slo list`
    - **Expected**: Metrics are fetched via the `CloudWatchProvider` (requires AWS credentials).

## 5. Verifying CLI Flags
Test the new initialization flags directly:
```bash
./health-monitor --init --infra kubernetes --kubeconfig ~/.kube/config
```
**Expected**: The wizard should start with the Kubernetes infrastructure and path already selected.

## 6. Verifying Incident Enrichment
Verify that incidents are enriched with Kubernetes events.

1. Ensure you are using a Kubernetes/EKS profile.
2. Start an incident for a known K8s service:
   ```bash
   ./health-monitor incident start --service "default/my-service" --title "High Latency" --severity CRITICAL
   ```
3. View the incident details:
   ```bash
   ./health-monitor incident show --id <INC-ID>
   ```
4. **Expected**: The incident analysis/RCA section should contain a "Possible K8s Events" list fetched from your cluster.

## 7. Build Verification
Ensure the project still builds correctly:
```bash
/usr/local/go/bin/go build -o health-monitor ./cmd/health-monitor
```

## 8. Configuration Check
Verify the structure of a generated profile (e.g., `~/.health-monitor/profiles/my-eks-profile.yaml`):
```yaml
eks:
  enabled: true
  kubeconfig: /home/user/.kube/config
  region: us-east-1
  cloudwatch_enabled: true
  auto_discover: true
  thresholds:
    pod_ready_percentage: 90
    node_pressure_allowed: false
    max_restart_count: 5
```

## 9. Verifying Collaborative Sessions (Tunnels)

### Test A: Public IP Discovery & Token Auth
1. Run the session: `sudo health-monitor incident view --tui --collaborative`
2. **Expected**: The header banner displays a **Public IP** and a **Join Token**.
3. **Guest Join**: Run `ssh <IP> -p 9022` from another terminal.
4. **Expected**: Prompted for a password. Use the **Join Token**.
5. **Expected**: Guest sees the adaptive incident viewport.

### Test B: Reverse SSH Tunnel (Behind NAT)
1. **Local Host**: `sudo health-monitor incident view --tui --collaborative`
2. **Setup Tunnel**: `ssh -i <KEY> -R 9022:localhost:9022 ec2-user@<EC2_IP>`
3. **EC2 Guest**: `ssh localhost -p 9022`
4. **Expected**: Successful connection to the local session from EC2.

### Test C: Adaptive Rendering
1. Resize the guest terminal window.
2. **Expected**: The viewport content re-renders to match the new width.
