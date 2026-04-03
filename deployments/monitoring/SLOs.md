# SLO Definitions for Muster

This document defines Service Level Objectives (SLOs) for all critical paths in the Muster system.

## Overview

| Category | SLO Target | Measurement Window | Error Budget |
|----------|-----------|-------------------|--------------|
| API Availability | 99.9% | 30 days | 0.1% (43.2 minutes) |
| API Latency (p95) | < 200ms | 7 days | 5% of requests |
| API Error Rate | < 0.1% | 7 days | - |
| MCP Availability | 99.5% | 30 days | 0.5% (216 minutes) |
| MCP Latency (p95) | < 500ms | 7 days | 5% of requests |
| Daemon Availability | 99.99% | 30 days | 0.01% (4.32 minutes) |
| Cache Hit Rate | > 95% | 7 days | 5% |

---

## 1. API SLOs

### 1.1 Availability

**SLO:** 99.9% of API requests return a successful response (2xx or 4xx status codes).

**SLI:**
```promql
sum(rate(http_requests_total{status_code!~"5.."}[5m])) 
/ 
sum(rate(http_requests_total[5m]))
```

**Burn Rate Alerts:**
- Fast Burn (1% budget in 1 hour): Error rate > 14.4%
- Slow Burn (2% budget in 6 hours): Error rate > 3%

### 1.2 Latency

**SLO:** 95% of API requests complete within 200ms.

**SLI:**
```promql
histogram_quantile(0.95, 
  sum(rate(request_duration_seconds_bucket[5m])) by (le)
)
```

**Alert Thresholds:**
- Warning: p95 > 150ms
- Critical: p95 > 200ms

### 1.3 Error Rate

**SLO:** Less than 0.1% of API requests return 5xx errors.

**SLI:**
```promql
sum(rate(http_requests_total{status_code=~"5.."}[5m]))
/
sum(rate(http_requests_total[5m]))
```

---

## 2. MCP (Model Context Protocol) SLOs

### 2.1 Availability

**SLO:** 99.5% of MCP tool calls return a successful response.

**SLI:**
```promql
sum(rate(mcp_tool_calls_total{status="success"}[5m]))
/
sum(rate(mcp_tool_calls_total[5m]))
```

### 2.2 Latency

**SLO:** 95% of MCP tool calls complete within 500ms.

**SLI:**
```promql
histogram_quantile(0.95,
  sum(rate(mcp_tool_call_duration_seconds_bucket[5m])) by (le)
)
```

### 2.3 Tool Registration Success Rate

**SLO:** 99.9% of tool registrations succeed.

**SLI:**
```promql
sum(rate(mcp_tool_registration_total{status="success"}[5m]))
/
sum(rate(mcp_tool_registration_total[5m]))
```

---

## 3. Daemon SLOs

### 3.1 Availability

**SLO:** 99.99% of daemon health checks pass.

**SLI:**
```promql
avg_over_time(up{job="muster-daemon"}[5m])
```

### 3.2 Socket Response Time

**SLO:** 99.9% of socket requests complete within 50ms.

**SLI:**
```promql
histogram_quantile(0.999,
  sum(rate(daemon_socket_request_duration_seconds_bucket[5m])) by (le)
)
```

---

## 4. Cache SLOs

### 4.1 Hit Rate

**SLO:** Cache hit rate > 95% for inventory lookups.

**SLI:**
```promql
sum(rate(cache_hits_total[5m]))
/
(sum(rate(cache_hits_total[5m])) + sum(rate(cache_misses_total[5m])))
```

### 4.2 TTL Compliance

**SLO:** < 1% of cache entries are stale (past TTL but not evicted).

**SLI:**
```promql
sum(rate(cache_stale_entries_total[5m]))
/
sum(rate(cache_entries_total[5m]))
```

---

## 5. Command Store SLOs

### 5.1 Load Time

**SLO:** 99% of command store loads complete within 100ms.

**SLI:**
```promql
histogram_quantile(0.99,
  sum(rate(cmdstore_load_duration_seconds_bucket[5m])) by (le)
)
```

### 5.2 Persistence Success Rate

**SLO:** 99.99% of command store persistence operations succeed.

**SLI:**
```promql
sum(rate(cmdstore_persist_total{status="success"}[5m]))
/
sum(rate(cmdstore_persist_total[5m]))
```

---

## 6. Auth SLOs

### 6.1 OAuth Token Refresh Success

**SLO:** 99.5% of OAuth token refreshes succeed.

**SLI:**
```promql
sum(rate(oauth_token_refresh_total{status="success"}[5m]))
/
sum(rate(oauth_token_refresh_total[5m]))
```

### 6.2 Auth Latency

**SLO:** 95% of authentication operations complete within 100ms.

**SLI:**
```promql
histogram_quantile(0.95,
  sum(rate(auth_operation_duration_seconds_bucket[5m])) by (le)
)
```

---

## 7. Error Budget Policy

### 7.1 Error Budget Calculation

Error budget is calculated as:
```
Error Budget = (1 - SLO Target) × Total Requests
```

### 7.2 Burn Rate Alerts

| Burn Rate | Time to Exhaust Budget | Alert Threshold |
|-----------|----------------------|----------------|
| 1x | 30 days | - |
| 2x | 15 days | Page on-call |
| 10x | 3 days | Page on-call + war room |
| 100x | 7 hours | Emergency response |

### 7.3 Policy

When error budget is exhausted:

1. **0-25% consumed:** Monitor closely, no action required
2. **25-50% consumed:** Review recent changes, prepare rollback
3. **50-75% consumed:** Freeze non-critical deployments
4. **75-100% consumed:** Stop all deployments, focus on reliability
5. **>100% consumed:** Emergency response, consider circuit breaker

---

## 8. Dashboard Links

- **API Overview:** http://grafana:3000/d/muster-api
- **MCP Dashboard:** http://grafana:3000/d/muster-mcp
- **Daemon Health:** http://grafana:3000/d/muster-daemon
- **Cache Metrics:** http://grafana:3000/d/muster-cache
- **SLI Dashboard:** http://grafana:3000/d/muster-sli

---

## 9. Review Schedule

- **Weekly:** Review SLO dashboards
- **Monthly:** SLO performance review
- **Quarterly:** SLO target adjustment (if needed)
- **Annually:** Full SLO framework review

---

*Last Updated: 2026-04-02*
*Owner: SRE Team*
