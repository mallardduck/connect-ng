package scc

import (
	"github.com/SUSE/connect-ng/k8s/productconfig"
)

// Config is the single source of truth for SCC registration configuration.
//
// This config is used by:
//   - Runtime: k8s.Setup() in main.go
//   - Codegen: cmd/generate when generating CRD types
//
// Benefits:
//   - No duplication between runtime and codegen config
//   - Type-safe (compiler validated)
//   - Defaults automatically applied via productconfig.New()
//   - Version can reference your existing version.Version constant
//
// Usage:
//   - Runtime: k8s.Setup(mgr, scc.Config)
//   - Codegen: go run github.com/SUSE/connect-ng/k8s/cmd/generate (auto-discovers this file)
var Config = productconfig.New(
	"myproduct",        // product
	"1.0.0",            // version - in real code: version.Version
	"myproduct-system", // namespace
	productconfig.WithMetricsSecret("myproduct-scc-metrics"),
	// Optional settings:
	// productconfig.WithMetricsSecretNamespace("custom-namespace"),
	// productconfig.WithGroup("custom.registration.example.com"),
)

// Note: The metrics secret is externally populated by your product's telemetry system.
// It must contain a "payload" key with JSON-encoded system information.
// This data is passed directly to SCC during registration and keepalive.
//
// IMPORTANT: The metrics payload MUST include architecture information if your
// product has an architecture axis (e.g., {"arch": "x86_64", ...}).
// Architecture is runtime information, not static configuration.
//
// See k8s/METRICS_SECRET.md for details on the metrics secret pattern.
