package scc

import (
	"github.com/SUSE/connect-ng/k8s/productconfig"
)

// Config is the single source of truth for Rancher SCC registration configuration.
//
// This config is pure data with NO dependencies on generated types.
// This allows it to be used by:
//   - Runtime: k8s.Setup() in main.go
//   - Codegen: cmd/generate when generating CRD types (if needed)
//
// Benefits:
//   - No chicken-and-egg problem: compiles standalone before types are generated
//   - No duplication between runtime and codegen
//   - Type-safe configuration
//   - Defaults automatically applied via productconfig.New()
//
// Note: Version is a placeholder here and should be overridden at runtime
// from your actual version constant or CLI flag.
// In main.go: cfg := scc.Config; cfg.Version = actualVersion
var Config = productconfig.New(
	"rancher",       // product - used for K8s resources (API group, secret names, etc.)
	"0.0.0-dev",     // version (override at runtime)
	"cattle-system", // namespace
	productconfig.WithMetricsSecret("rancher-scc-metrics"),
	productconfig.WithGenerateOutputDir("./pkg/apis"),
	productconfig.WithShortNames("rancherreg", "rreg"),
	// productconfig.WithSkipSchemeBuilder(), // Optional: use this if using Wrangler's controller-gen
	// productconfig.WithSCCProductIdentifier("rancher-prime"), // Optional: for product variants (e.g., Rancher Prime)
)

// Note: The rancher-scc-metrics secret is externally populated by Rancher's telemetry system.
// It must contain a "payload" key with JSON-encoded system information including
// runtime details like hostname and architecture.
//
// See k8s/METRICS_SECRET.md for details on the metrics secret pattern.
