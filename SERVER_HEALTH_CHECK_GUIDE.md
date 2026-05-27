# Server Health Check Guide
## Disk • RAM • GPU • CPU • I/O • SSH • System

This document lists the **exact read-only commands** that the Health-Monitor agent runs to verify server health and detect potential issues like disk, RAM, GPU memory leaks, and security threats.

---

## 1️⃣ Disk Health Check

### Primary Command
```bash
df -h / | awk 'NR==2 {print $2,$3,$5}'
```
**Purpose:** Shows root filesystem disk usage (total, used, percentage)

**What the agent checks:**
- Disk usage percentage
- Total and used space

**What to look for:**
- Disk usage should ideally be below **80%**
- Agent flags as **RISK** if usage >= **85%**
- Above **90%** is critical

**Healthy sign:**
```
/dev/root   1007G total, 31G used, 4% used
Status: SAFE (< 85%)
```

**Red flag:**
```
/dev/root   1007G total, 900G used, 90% used
Status: RISK (>= 85%)
```

### Identify Large Directories in /var
```bash
du -sm /var/* 2>/dev/null | sort -n | tail -3
```
**Purpose:** Identifies top 3 largest directories under `/var`

**Use when:** Disk usage is high or cleanup suggestions are needed

**What to look for:**
- `/var/log` should be manageable (ideally < 500MB)
- `/var/cache` can be cleaned safely if needed
- `/var/lib` growth should be monitored

**Healthy sign:**
```
138MB  /var/cache
388MB  /var/log
2930MB /var/lib
```

**Red flag:**
```
5000MB  /var/log    (log rotation may be broken)
10000MB /var/cache  (cache not being cleaned)
```

---

## 2️⃣ RAM (Memory) Health Check

### Primary Command
```bash
free -m | awk '/Mem:/ {print $2, $7}'
```
**Purpose:** Shows total RAM and available memory in MB

**What the agent checks:**
- Total memory
- Available memory (not just free, but actually available for use)

**What to look for:**
- Available memory should not be near zero
- Some cache usage is normal (Linux uses free RAM for cache)
- Consistently low available memory = possible leak

**Healthy sign:**
```
Mem:  15697MB total, 7306MB available
```
(Good available memory, system can handle workloads)

**Red flag:**
```
Mem:  15697MB total, 200MB available
```
(Consistently low available memory = possible RAM leak)

**Threshold:** Agent flags as RISK if available memory < 500MB

---

## 3️⃣ GPU Memory Health Check (MOST IMPORTANT for leaks)

### Primary Command
```bash
nvidia-smi --query-gpu=memory.used,memory.total,utilization.gpu,temperature.gpu --format=csv,noheader
```
**Purpose:** Shows GPU memory usage, total memory, GPU utilization, and temperature

**What the agent checks:**
- GPU memory used vs total
- GPU utilization percentage
- GPU temperature
- Detects memory retention when GPU is idle

**What to look for:**
- GPU memory should match running processes
- Memory should stay stable when idle
- High memory usage with 0% utilization = possible leak

**Healthy sign:**
```
Memory-Usage: 1688MiB / 15360MiB
GPU-Util: 45%
Temperature: 32°C
```
(Active usage, normal temperature)

**Red flag (GPU leak):**
```
Memory-Usage: 12000MiB / 15360MiB
GPU-Util: 0%
Temperature: 32°C
```
(High memory usage with 0% utilization = memory retention/leak)

**Agent detection:** Flags as CHECK if `utilization == 0%` AND `memory.used > 500MB`

### Check GPU Temperature
**What to look for:**
- Normal operating: 30-70°C
- High load: 70-80°C (acceptable under load)
- Critical: > 80°C

**Agent flags:** CHECK status if temperature >= 80°C

### What is NOT a GPU memory leak (Important)
The following are normal and expected:
- CUDA context memory
- Triton server base memory
- Python backend stub memory
- Persistence mode baseline memory

These do not grow continuously. The agent only flags if memory is high **while GPU utilization is 0%**.

---

## 4️⃣ CPU Usage & System Load

### CPU Usage Command
```bash
top -bn1 | grep 'Cpu(s)' | sed 's/.*, *\([0-9.]*\)%* id.*/\1/' | awk '{print 100 - $1}'
```
**Purpose:** Shows current CPU usage percentage

**What to look for:**
- Normal: 1-50% (varies by workload)
- High: 50-90% (under load, acceptable)
- Critical: > 90% consistently (may indicate issue)

**Healthy sign:**
```
CPU Usage: 1.2%
```
(Low usage, system idle or light load)

**Red flag:**
```
CPU Usage: 95%
```
(Consistently high, may indicate runaway process)

### System Uptime Command
```bash
cat /proc/uptime | awk '{print $1/86400}'
```
**Purpose:** Shows system uptime in days

**What to look for:**
- Shows how long system has been running
- Useful for identifying if recent restarts occurred

**Healthy sign:**
```
System Uptime: 7.2 days
```
(Stable system, no unexpected restarts)

### Load Average Command
```bash
uptime | awk -F'load average:' '{print $2}'
```
**Purpose:** Shows 1-minute, 5-minute, and 15-minute load averages

**What to look for:**
- Load average should be reasonable for CPU count
- Load of 1.0 = 1 CPU fully utilized
- Load > CPU count = system overloaded

**Healthy sign:**
```
Load Average: 0.20, 0.11, 0.07
```
(Low load, system handling workload well)

**Red flag:**
```
Load Average: 8.50, 7.20, 6.80
```
(On 4-core system, load > 4 = overloaded)

---

## 5️⃣ Disk I/O Wait

### Primary Command
```bash
iostat -c 1 1 | awk 'NR==4 {print $4}'
```
**Fallback Command:**
```bash
vmstat 1 2 | tail -1 | awk '{print $16}'
```
**Purpose:** Shows percentage of CPU time spent waiting for I/O operations

**What to look for:**
- I/O wait should be low (< 5%)
- High I/O wait = disk bottleneck
- Can indicate disk failure or overload

**Healthy sign:**
```
Disk I/O Wait: 0.00% (SAFE)
```
(No I/O bottlenecks)

**Red flag:**
```
Disk I/O Wait: 15.00% (RISK)
```
(High I/O wait = disk performance issue)

**Agent thresholds:**
- SAFE: < 5%
- CHECK: 5-10%
- RISK: >= 10%

---

## 6️⃣ Zombie Processes

### Command
```bash
ps aux | awk '$8=="Z"' | wc -l
```
**Purpose:** Counts zombie processes (defunct processes not cleaned up)

**What to look for:**
- Should be 0 or very low
- Zombie processes indicate parent process not cleaning up children
- Usually harmless but indicates buggy process

**Healthy sign:**
```
Zombie Processes: 0 (SAFE)
```

**Red flag:**
```
Zombie Processes: 15 (RISK)
```
(Multiple zombies = process management issue)

**Agent flags:** CHECK if count > 0 (not RISK, but needs attention)

---

## 7️⃣ SSH Security Check (Brute Force Detection)

### Primary Command
```bash
grep 'Accepted password' /var/log/auth.log | egrep 'ubuntu|root' | tail -1
```
**Purpose:** Detects if password-based authentication succeeded (security risk)

**What to look for:**
- Should return empty (no password logins)
- Password-based logins are a security risk
- Should only use public key authentication

**Healthy sign:**
```
(No output - no password logins detected)
```
(Only public key authentication in use)

**Red flag:**
```
Accepted password for ubuntu from 192.168.1.100
```
(Password login succeeded = security breach risk)

**Agent flags:** RISK if password login detected

### Additional SSH Checks (Manual)

**Check successful logins:**
```bash
sudo grep "Accepted" /var/log/auth.log | tail -20
```
**Healthy sign:**
```
Accepted publickey for ubuntu from 192.168.1.50
```
(Only public key authentication)

**Red flag:**
```
Accepted password for ubuntu from unknown-ip
Accepted password for root from unknown-ip
```
(Password-based success = investigate immediately)

**Check invalid login attempts:**
```bash
sudo grep "Invalid user" /var/log/auth.log | tail -20
```
**Interpretation:**
- Random usernames (admin, test, oracle, vpnuser) → normal internet noise
- These attempts are blocked automatically
- Not a concern unless volume is extremely high

**Check failed password attempts:**
```bash
sudo grep "Failed password" /var/log/auth.log | tail -20
```
**Healthy sign:**
- No failed password attempts for real users
- Entries from automated scans are normal

**Check active SSH sessions:**
```bash
who
# or
w
```
**Purpose:** Confirm only known users are logged in, verify IP addresses

---

## 8️⃣ /var Directory Analysis

### Command
```bash
du -sm /var/* 2>/dev/null | sort -n | tail -3
```
**Purpose:** Identifies top 3 largest consumers in `/var` directory

**What to look for:**
- `/var/log` - Should be manageable (< 500MB ideally)
- `/var/cache` - Can be cleaned safely
- `/var/lib` - Application data, monitor growth

**Healthy sign:**
```
138MB  /var/cache
388MB  /var/log
2930MB /var/lib
```

**Red flag:**
```
5000MB  /var/log    (log rotation broken)
10000MB /var/cache  (cache not cleaning)
```

**Agent cleanup suggestions:**
- If `/var/log > 500MB`: "Ensure log rotation is configured"
- If `/var/cache > 200MB`: "/var/cache can be cleaned safely if required"
- If disk usage > 80%: "Disk usage is high — consider cleaning logs or unused data"

---

## 9️⃣ Quick Health Checklist

| Area | Command | Healthy If | Red Flag |
|------|---------|-----------|----------|
| **Disk** | `df -h /` | < 85% used | >= 85% used |
| **RAM** | `free -m` | Available memory present | < 500MB available |
| **GPU Memory** | `nvidia-smi` | Memory stable, matches utilization | High memory + 0% util |
| **GPU Temp** | `nvidia-smi` | < 80°C | >= 80°C |
| **CPU** | `top -bn1` | Reasonable usage | > 90% consistently |
| **Load Avg** | `uptime` | < CPU count | > CPU count |
| **I/O Wait** | `iostat` or `vmstat` | < 5% | > 10% |
| **Zombies** | `ps aux` | 0 processes | > 0 processes (CHECK status) |
| **SSH** | `grep auth.log` | Only publickey | Password logins |
| **Uptime** | `cat /proc/uptime` | Stable | Frequent restarts |

---

## 🔟 Status Levels (Agent Interpretation)

The agent categorizes each check into three status levels:

**🟢 SAFE** - Operating within normal thresholds
- Disk: < 85% used
- Memory: > 500MB available
- GPU: Normal usage or no GPU
- I/O Wait: < 5%
- Zombies: 0
- SSH: No password logins

**🟡 CHECK** - Needs attention, not critical
- GPU: High memory with 0% utilization (possible leak)
- GPU Temp: >= 80°C
- I/O Wait: 5-10%
- Zombies: > 0

**🔴 RISK** - Immediate action recommended
- Disk: >= 85% used
- Memory: <= 500MB available
- I/O Wait: >= 10%
- SSH: Password login detected

---

## 1️⃣1️⃣ Recommended Frequency

**Daily:**
- Run `health-monitor` agent (runs all checks automatically)
- Or manually: `df -h`, `free -h`, `nvidia-smi`, SSH accepted log check

**Weekly:**
- Monitor GPU memory trends: `watch -n 5 nvidia-smi`
- Review `/var` directory sizes
- Check for zombie process accumulation

**After deployments:**
- Full GPU + SSH review
- Verify no memory leaks introduced
- Check disk space impact

**When issues detected:**
- Investigate immediately if RISK status
- Review CHECK items within 24 hours
- Monitor trends if SAFE but approaching thresholds

---

## 1️⃣2️⃣ Manual Verification Commands

If you want to manually verify what the agent is checking:

```bash
# Disk
df -h /

# Memory
free -h

# GPU
nvidia-smi

# CPU & Load
top -bn1
uptime

# I/O Wait
iostat -c 1 1
# or
vmstat 1 2

# Zombies
ps aux | awk '$8=="Z"'

# /var sizes
du -sh /var/* 2>/dev/null | sort -h

# SSH security
sudo grep "Accepted password" /var/log/auth.log | egrep 'ubuntu|root'
```

---

## Notes

- All commands are **read-only** - they don't modify the system
- The agent runs these commands automatically and presents results in an interactive TUI
- Status levels (SAFE/CHECK/RISK) are automatically determined based on thresholds
- The agent provides cleanup suggestions when issues are detected
- All checks are non-blocking - if one fails, others continue
