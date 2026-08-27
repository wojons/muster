# DEPS-001: Held Indirect Dependencies (36 remaining)

**Date:** 2026-08-27
**Reason:** All 36 remaining outdated modules are indirect-only — none appear in `go.mod`'s
direct `require` block. Upgrading them individually would add churn without user-visible
benefit. They are pulled transitively by testcontainers-go, viper, otel, and docker
client libraries. MVS (Minimum Version Selection) already resolves their transitive
constraints; bumping them would only change their pinned version without affecting the
muster build or runtime.

## Held by category

### google.golang.org/* (SDK family — pinned)
- `google.golang.org/grpc v1.67.0 → v1.83.2`: huge jump; only via testcontainers-go
- `google.golang.org/genproto/googleapis/rpc`: protobuf companion; locked to grpc version
- `google.golang.org/protobuf v1.36.11 → v1.36.12`: patch; proto toolchain pinned

### golang.org/x/* (stdlib extensions — minor bumps only)
- `golang.org/x/net v0.57.0 → v0.58.0`: minor; pulled by grpc/otel
- `golang.org/x/mod v0.38.0 → v0.40.0`: tools only; not called at runtime
- `golang.org/x/time v0.11.0 → v0.15.0`: rate limiting; compatible at current
- `golang.org/x/tools v0.48.0 → v0.49.0`: dev tooling; not runtime

### Redis (testcontainers module only)
- `github.com/redis/go-redis/v9 v9.7.3 → v9.22.0`: large jump; only via testcontainers redis module; no direct usage in muster

### Docker/Moby/Containerd (testcontainers transitive)
- `github.com/docker/go-connections v0.7.0 → v0.8.1`
- `github.com/moby/go-archive v0.3.0 → v0.3.3`
- `github.com/moby/moby/client v0.5.0 → v0.5.1`
- `github.com/moby/sys/userns v0.1.0 → v0.2.0`
- `github.com/containerd/typeurl/v2 v2.2.0 → v2.3.0`

### go-openapi (kin-openapi transitive)
- `github.com/go-openapi/jsonpointer`, `swag/jsonname`, `testify/v2`: openapi ecosystem

### CLI/UX transitive
- `github.com/cpuguy83/go-md2man/v2`: man page rendering (cobra dep)
- `github.com/sirupsen/logrus v1.9.4 → v1.10.2`: used by docker client; muster uses zerolog
- `github.com/shirou/gopsutil`: system stats (cobra dep)
- `github.com/magiconair/properties`: viper config source
- `github.com/pelletier/go-toml/v2`: viper config parser
- `github.com/go-viper/mapstructure/v2`: viper config binding
- `github.com/sagikazarmark/locafero`: viper config search

### Other
- `github.com/golang/protobuf v1.5.0 → v1.5.4`: legacy proto (grpc compat)
- `github.com/gorilla/mux v1.8.0 → v1.8.1`: testcontainers HTTP
- `github.com/klauspost/compress v1.19.1 → v1.19.2`: compression; patch only
- `github.com/ebitengine/purego v0.10.1 → v0.10.2`: wasmtime dep; patch only
- `github.com/mattn/go-colorable`, `go-isatty`: zerolog deps (already latest used)
- `github.com/power-devops/perfstat`: cobra dep (system stats)
- `github.com/rogpeppe/go-internal`: testing tool; not runtime
- `github.com/xeipuuv/gojsonpointer`: openapi validation
- `github.com/dlclark/regexp2`: wasmtime regex; minor bump
- `github.com/lufia/plan9stats`: golang.org/x/internal; plan9 platform only
- `pgregory.net/rapid`: property testing; dev only
- `go.opentelemetry.io/contrib/.../otelhttp v0.69.0 → v0.71.0`: otel http contrib; tied to otel SDK version
