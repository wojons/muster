# OPS-ALERTING: Alert Rules and Notifications Specification

## Overview
Prometheus Alertmanager configuration for automated incident response.

## Alert Rules

```yaml
# alerts.yml
groups:
  - name: muster_alerts
    rules:
      # High error rate
      - alert: HighErrorRate
        expr: |
          (
            sum(rate(http_requests_total{status_code=~"5.."}[5m]))
            /
            sum(rate(http_requests_total[5m]))
          ) > 0.05
        for: 5m
        labels:
          severity: critical
          service: api
        annotations:
          summary: "High error rate detected"
          description: "Error rate is {{ $value | humanizePercentage }}"
          runbook_url: "https://wiki/runbooks/high-error-rate"

      # High latency
      - alert: HighLatency
        expr: |
          histogram_quantile(0.95,
            sum(rate(request_duration_seconds_bucket[5m])) by (le)
          ) > 0.5
        for: 10m
        labels:
          severity: warning
          service: api
        annotations:
          summary: "High latency detected"
          description: "95th percentile latency is {{ $value }}s"

      # Low disk space
      - alert: LowDiskSpace
        expr: node_filesystem_avail_bytes / node_filesystem_size_bytes < 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Low disk space on {{ $labels.instance }}"
          description: "Disk space is below 10%"

      # High memory usage
      - alert: HighMemoryUsage
        expr: (node_memory_MemTotal_bytes - node_memory_MemAvailable_bytes) / node_memory_MemTotal_bytes > 0.85
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High memory usage"
          description: "Memory usage is above 85%"

      # Database connection failures
      - alert: DatabaseConnectionFailures
        expr: increase(db_connection_errors_total[5m]) > 10
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "Database connection failures"
          description: "{{ $value }} connection failures in 5 minutes"

      # Service down
      - alert: ServiceDown
        expr: up == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Service {{ $labels.job }} is down"
          description: "Service has been down for more than 1 minute"
```

## Alertmanager Configuration

```yaml
# alertmanager.yml
global:
  smtp_smarthost: 'smtp.gmail.com:587'
  smtp_from: 'alerts@muster.io'
  smtp_auth_username: 'alerts@muster.io'
  smtp_auth_password: '${SMTP_PASSWORD}'
  slack_api_url: '${SLACK_WEBHOOK_URL}'
  pagerduty_url: 'https://events.pagerduty.com/v2/enqueue'

templates:
  - '/etc/alertmanager/templates/*.tmpl'

route:
  receiver: 'default'
  group_by: ['alertname', 'severity', 'service']
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 4h
  routes:
    - match:
        severity: critical
      receiver: 'pagerduty'
      continue: true
    - match:
        severity: warning
      receiver: 'slack'
      continue: true

receivers:
  - name: 'default'
    email_configs:
      - to: 'ops@muster.io'
        send_resolved: true

  - name: 'slack'
    slack_configs:
      - channel: '#alerts'
        send_resolved: true
        title: '{{ range .Alerts }}{{ .Annotations.summary }}{{ end }}'
        text: |
          {{ range .Alerts }}
          *Alert:* {{ .Annotations.summary }}
          *Description:* {{ .Annotations.description }}
          *Severity:* {{ .Labels.severity }}
          *Runbook:* {{ .Annotations.runbook_url }}
          {{ end }}

  - name: 'pagerduty'
    pagerduty_configs:
      - service_key: '${PAGERDUTY_KEY}'
        severity: '{{ .Labels.severity }}'
        description: '{{ .Annotations.summary }}'
        details:
          firing: '{{ template "pagerduty.default.instances" .Alerts.Firing }}'
          resolved: '{{ template "pagerduty.default.instances" .Alerts.Resolved }}'

inhibit_rules:
  - source_match:
      severity: 'critical'
    target_match:
      severity: 'warning'
    equal: ['alertname', 'service']
```

## Alert Severity Levels

| Severity | Response Time | Notification |
|----------|---------------|--------------|
| Critical | Immediate | PagerDuty + Slack |
| Warning | 15 minutes | Slack + Email |
| Info | 1 hour | Email only |

## Silencing

```yaml
# Silence maintenance windows
- matchers:
    - name: alertname
      value: HighMemoryUsage
    - name: environment
      value: staging
  startsAt: 2024-01-01T00:00:00Z
  endsAt: 2024-01-01T04:00:00Z
  createdBy: ops-team
  comment: "Scheduled maintenance"
```

## Template Example

```yaml
# templates/default.tmpl
{{ define "slack.default.text" }}
{{ range .Alerts }}
{{ if eq .Status "firing" }}🔥{{ else }}✅{{ end }} *{{ .Labels.severity | toUpper }}*
*Alert:* {{ .Annotations.summary }}
*Description:* {{ .Annotations.description }}
*Started:* {{ .StartsAt.Format "2006-01-02 15:04:05" }}
{{ end }}
{{ end }}
```
