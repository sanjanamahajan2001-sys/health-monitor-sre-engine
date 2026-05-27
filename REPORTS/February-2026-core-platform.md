# HEALTH-MONITORING AGENT
## Monthly Scoreboard: February 2026

**Profile/Team**: core-platform
**Overall Health Score**: 85.8% (At Risk)

## 1. Service Reliability Summary

| Service | Avail% | P1/P2 | MTTR | MTBF | %Done |
| :--- | :--- | :--- | :--- | :--- | :--- |
| identity_service | 96.80% | 4/0 | 7h29m0s | 174h0m0s | 100% Done |
| billing_api | 96.73% | 6/0 | 4h29m0s | 116h0m0s | 0% Done |
| payments_service | 99.84% | 2/0 | 0s | 359h0m0s | 0% Done |

## 2. Error Budget Summary

| Service | Goal | Actual | Rem (%) | Status |
| :--- | :--- | :--- | :--- | :--- |
| identity_service | 99.0% | 0.0% | 0.0% | ✅ OK |
| billing_api | 95.0% | 0.0% | 0.0% | ✅ OK |
| payments_service | 95.0% | 0.0% | 0.0% | ✅ OK |

## 3. Incident Distribution by Category

| Category | Count | % of Total |
| :--- | :--- | :--- |
| security | 1 | 8.3% |
| capacity | 2 | 16.7% |

## 4. ML Agent Insights

- **Top Feature**: capacity
- **Top Pattern**: memory_pressure
- **Avg Conf**: 22.9%
- **Actionable Hint Rate**: 25.0%

## 5. Executive Summary

- **Primary Risk**: billing_api
- **Frequent Pattern**: capacity
- **Focus Area**: Remediate capacity pattern in billing_api

## 6. Detailed Action Items

| ID | Pr | Service | Details | Owner | Status | Due |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| INC-20260225-115211 | P3 | payments_service | Memory usage monitorin... | DevOps | Overdue | Feb 20 |
| INC-20260225-115051 | P2 | identity_service | Add more security leve... | Backend | Completed | Feb 24 |
| INC-20260225-114925 | P2 | billing_api | Added monitoring level... | DevOps | Pending | Mar 09 |

