package productconfig

import "fmt"

// Option is a functional option for configuring ProductConfig.
type Option func(*ProductConfig)

// ProductConfig defines SCC registration configuration for a product.
//
// Products should create a single instance of this in their codebase
// (e.g., pkg/scc/config.go) and use it for both:
//   - Runtime: k8s.Setup(mgr, myproduct.SCCConfig.ToSetupConfig())
//   - Codegen: generator automatically finds and loads it
//
// Example:
//
//	// pkg/scc/config.go
//	package scc
//
//	import (
//	    "github.com/SUSE/connect-ng/k8s/productconfig"
//	    "github.com/rancher/rancher/pkg/version"
//	)
//
//	var Config = productconfig.ProductConfig{
//	    Product:           "rancher",
//	    Version:           version.Version, // Use existing version constant
//	    Namespace:         "cattle-system",
//	    MetricsSecretName: "rancher-scc-metrics", // Externally populated by telemetry
//	}
type ProductConfig struct {
	// Product is the SCC product identifier (e.g., "rancher", "neuvector")
	// This determines the API group: <product>.registration.suse.com
	Product string `json:"product" yaml:"product"`

	// Version is the product version (e.g., "2.10.0")
	// Can reference your existing version constant
	Version string `json:"version" yaml:"version"`

	// Namespace is where SCC secrets will be created
	Namespace string `json:"namespace" yaml:"namespace"`

	// Group is the API group (optional, defaults to "<product>.registration.suse.com")
	Group string `json:"group,omitempty" yaml:"group,omitempty"`

	// MetricsSecretNamespace is the namespace containing the metrics secret.
	// The metrics secret is externally populated by the product's telemetry system.
	// Defaults to Namespace if not specified.
	MetricsSecretNamespace string `json:"metricsSecretNamespace,omitempty" yaml:"metricsSecretNamespace,omitempty"`

	// MetricsSecretName is the name of the metrics secret.
	// The secret must contain a "payload" key with JSON-encoded telemetry data.
	// This MUST include:
	//   - architecture information if the product has an architecture axis
	//   - hostname or system identifier for SCC registration
	// Required for SCC registration.
	// Example: "rancher-scc-metrics"
	MetricsSecretName string `json:"metricsSecretName,omitempty" yaml:"metricsSecretName,omitempty"`

	// GenerateOutputDir is where codegen writes files (optional, for codegen only)
	// Defaults to "./apis/<group>/v1"
	GenerateOutputDir string `json:"generateOutputDir,omitempty" yaml:"generateOutputDir,omitempty"`
}

// Validate checks that required fields are set.
func (c *ProductConfig) Validate() error {
	if c.Product == "" {
		return fmt.Errorf("product is required")
	}
	if c.Version == "" {
		return fmt.Errorf("version is required")
	}
	if c.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	return nil
}

// ApplyDefaults fills in default values for optional fields.
func (c *ProductConfig) ApplyDefaults() {
	if c.Group == "" {
		c.Group = fmt.Sprintf("%s.registration.suse.com", c.Product)
	}
	if c.MetricsSecretNamespace == "" {
		c.MetricsSecretNamespace = c.Namespace
	}
	if c.GenerateOutputDir == "" {
		c.GenerateOutputDir = fmt.Sprintf("./apis/%s/v1", c.Group)
	}
}

// New creates a ProductConfig with defaults applied.
// This is the recommended way to create ProductConfig instances to ensure
// defaults are always applied.
//
// Example:
//
//	cfg := productconfig.New("rancher", "2.10.0", "cattle-system",
//	    productconfig.WithMetricsSecret("rancher-scc-metrics"),
//	)
func New(product, version, namespace string, opts ...Option) ProductConfig {
	cfg := ProductConfig{
		Product:   product,
		Version:   version,
		Namespace: namespace,
	}

	// Apply functional options
	for _, opt := range opts {
		opt(&cfg)
	}

	// Apply defaults for any unset fields
	cfg.ApplyDefaults()

	return cfg
}

// WithGroup sets a custom API group (overrides default "<product>.registration.suse.com").
func WithGroup(group string) Option {
	return func(c *ProductConfig) {
		c.Group = group
	}
}

// WithMetricsSecret sets the metrics secret name.
// The metrics secret namespace defaults to the product namespace.
func WithMetricsSecret(name string) Option {
	return func(c *ProductConfig) {
		c.MetricsSecretName = name
	}
}

// WithMetricsSecretNamespace sets the metrics secret namespace.
// If not specified, defaults to the product namespace.
func WithMetricsSecretNamespace(namespace string) Option {
	return func(c *ProductConfig) {
		c.MetricsSecretNamespace = namespace
	}
}

// WithGenerateOutputDir sets the output directory for code generation.
// Defaults to "./apis/<group>/v1" if not specified.
func WithGenerateOutputDir(dir string) Option {
	return func(c *ProductConfig) {
		c.GenerateOutputDir = dir
	}
}

// Note: ToRuntimeConfig() is defined in the parent k8s package
// to avoid circular dependencies. Import the k8s package and call:
//
//	k8s.Setup(mgr, k8s.ConfigFromProduct(myconfig.Config))
//
// Or use the Config type directly if not using ProductConfig.
