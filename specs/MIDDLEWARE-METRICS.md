# Middleware: Metrics Collection

## Overview

Metrics collection middleware exposes Prometheus-compatible metrics for monitoring HTTP request performance, status code distributions, and system health with configurable histogram buckets and label dimensions.

## Configuration

```go
type MetricsConfig struct {
    Enabled           bool            `json:"enabled"`
    Path              string          `json:"path" validate:"required"`
    Namespace         string          `json:"namespace"`
    Subsystem         string          `json:"subsystem"`
    DurationBuckets   []float64       `json:"duration_buckets"`
    SizeBuckets       []float64       `json:"size_buckets"`
    ExcludePaths      []string        `json:"exclude_paths"`
    IncludeQuery      bool            `json:"include_query"`
    MaxPathCardinality int            `json:"max_path_cardinality" validate:"gte=0"`
    LabelNames        []string        `json:"label_names"`
}

var DefaultMetricsConfig = MetricsConfig{
    Enabled:            true,
    Path:               "/metrics",
    Namespace:          "muster",
    Subsystem:          "http",
    DurationBuckets:    []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
    SizeBuckets:        []float64{100, 1000, 10000, 100000, 1e6, 1e7, 1e8, 1e9},
    ExcludePaths:       []string{"/metrics", "/health", "/ready"},
    IncludeQuery:       false,
    MaxPathCardinality: 1000,
    LabelNames:         []string{"method", "path", "status"},
}
```

## Metrics Definition

```go
type HTTPMetrics struct {
    requestDuration   *prometheus.HistogramVec
    requestSize       *prometheus.HistogramVec
    responseSize      *prometheus.HistogramVec
    requestsTotal     *prometheus.CounterVec
    requestsInFlight  prometheus.Gauge
    connectionsOpen   prometheus.Gauge
}

func NewHTTPMetrics(config MetricsConfig) *HTTPMetrics {
    namespace := config.Namespace
    subsystem := config.Subsystem

    m := &HTTPMetrics{
        requestDuration: prometheus.NewHistogramVec(
            prometheus.HistogramOpts{
                Namespace: namespace,
                Subsystem: subsystem,
                Name:      "request_duration_seconds",
                Help:      "HTTP request latency in seconds",
                Buckets:   config.DurationBuckets,
            },
            config.LabelNames,
        ),
        requestSize: prometheus.NewHistogramVec(
            prometheus.HistogramOpts{
                Namespace: namespace,
                Subsystem: subsystem,
                Name:      "request_size_bytes",
                Help:      "HTTP request size in bytes",
                Buckets:   config.SizeBuckets,
            },
            config.LabelNames,
        ),
        responseSize: prometheus.NewHistogramVec(
            prometheus.HistogramOpts{
                Namespace: namespace,
                Subsystem: subsystem,
                Name:      "response_size_bytes",
                Help:      "HTTP response size in bytes",
                Buckets:   config.SizeBuckets,
            },
            config.LabelNames,
        ),
        requestsTotal: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Namespace: namespace,
                Subsystem: subsystem,
                Name:      "requests_total",
                Help:      "Total number of HTTP requests",
            },
            append(config.LabelNames, "status_class"),
        ),
        requestsInFlight: prometheus.NewGauge(
            prometheus.GaugeOpts{
                Namespace: namespace,
                Subsystem: subsystem,
                Name:      "requests_in_flight",
                Help:      "Current number of HTTP requests being served",
            },
        ),
        connectionsOpen: prometheus.NewGauge(
            prometheus.GaugeOpts{
                Namespace: namespace,
                Subsystem: subsystem,
                Name:      "connections_open",
                Help:      "Current number of open HTTP connections",
            },
        ),
    }

    return m
}

func (m *HTTPMetrics) Register(registry prometheus.Registerer) error {
    collectors := []prometheus.Collector{
        m.requestDuration,
        m.requestSize,
        m.responseSize,
        m.requestsTotal,
        m.requestsInFlight,
        m.connectionsOpen,
    }
    
    for _, c := range collectors {
        if err := registry.Register(c); err != nil {
            return err
        }
    }
    return nil
}
```

## Middleware Implementation

```go
package middleware

import (
    "net/http"
    "strconv"
    "strings"
    "time"

    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

type MetricsMiddleware struct {
    config  MetricsConfig
    metrics *HTTPMetrics
    pathMap *PathNormalizer
}

type PathNormalizer struct {
    maxCardinality int
    knownPaths     map[string]string
    pathCounters   map[string]int
    mu             sync.RWMutex
}

func NewPathNormalizer(maxCardinality int) *PathNormalizer {
    return &PathNormalizer{
        maxCardinality: maxCardinality,
        knownPaths:     make(map[string]string),
        pathCounters:   make(map[string]int),
    }
}

func (pn *PathNormalizer) Normalize(path string) string {
    pn.mu.RLock()
    if normalized, ok := pn.knownPaths[path]; ok {
        pn.mu.RUnlock()
        return normalized
    }
    pn.mu.RUnlock()

    pn.mu.Lock()
    defer pn.mu.Unlock()

    if normalized, ok := pn.knownPaths[path]; ok {
        return normalized
    }

    if pn.maxCardinality > 0 && len(pn.knownPaths) >= pn.maxCardinality {
        return "/other"
    }

    normalized := pn.normalizePath(path)
    pn.knownPaths[path] = normalized
    return normalized
}

func (pn *PathNormalizer) normalizePath(path string) string {
    segments := strings.Split(path, "/")
    for i, seg := range segments {
        if isDynamicSegment(seg) {
            segments[i] = ":param"
        }
    }
    return strings.Join(segments, "/")
}

func isDynamicSegment(seg string) bool {
    if seg == "" {
        return false
    }
    if _, err := strconv.Atoi(seg); err == nil {
        return true
    }
    if matched, _ := regexp.MatchString(`^[0-9a-f]{8}-[0-9a-f]{4}`, seg); matched {
        return true
    }
    return false
}

func NewMetricsMiddleware(config MetricsConfig, metrics *HTTPMetrics) *MetricsMiddleware {
    return &MetricsMiddleware{
        config:  config,
        metrics: metrics,
        pathMap: NewPathNormalizer(config.MaxPathCardinality),
    }
}

func (m *MetricsMiddleware) Handler(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if !m.config.Enabled {
            next.ServeHTTP(w, r)
            return
        }

        for _, path := range m.config.ExcludePaths {
            if r.URL.Path == path {
                next.ServeHTTP(w, r)
                return
            }
        }

        m.metrics.requestsInFlight.Inc()
        defer m.metrics.requestsInFlight.Dec()

        start := time.Now()
        path := m.pathMap.Normalize(r.URL.Path)
        if m.config.IncludeQuery && r.URL.RawQuery != "" {
            path = path + "?" + r.URL.RawQuery
        }

        recorder := &metricsResponseWriter{ResponseWriter: w}
        next.ServeHTTP(recorder, r)

        duration := time.Since(start).Seconds()
        status := strconv.Itoa(recorder.statusCode)
        statusClass := strconv.Itoa(recorder.statusCode/100) + "xx"

        labels := prometheus.Labels{
            "method": r.Method,
            "path":   path,
            "status": status,
        }

        m.metrics.requestDuration.With(labels).Observe(duration)
        m.metrics.responseSize.With(labels).Observe(float64(recorder.responseSize))
        
        totalLabels := prometheus.Labels{
            "method":       r.Method,
            "path":         path,
            "status":       status,
            "status_class": statusClass,
        }
        m.metrics.requestsTotal.With(totalLabels).Inc()
    })
}

type metricsResponseWriter struct {
    http.ResponseWriter
    statusCode   int
    responseSize int64
}

func (w *metricsResponseWriter) WriteHeader(code int) {
    w.statusCode = code
    w.ResponseWriter.WriteHeader(code)
}

func (w *metricsResponseWriter) Write(b []byte) (int, error) {
    n, err := w.ResponseWriter.Write(b)
    w.responseSize += int64(n)
    return n, err
}

func (w *metricsResponseWriter) Status() int {
    if w.statusCode == 0 {
        return http.StatusOK
    }
    return w.statusCode
}

func MetricsHandler(registry prometheus.Gatherer) http.Handler {
    return promhttp.HandlerFor(registry, promhttp.HandlerOpts{
        ErrorHandling:       promhttp.ContinueOnError,
        MaxRequestsInFlight: 10,
        Timeout:             time.Second * 5,
    })
}
```
