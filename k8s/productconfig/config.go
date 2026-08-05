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
// Example (basic):
//
//	// pkg/scc/config.go
//	package scc
//
//	import (
//	    "github.com/SUSE/connect-ng/k8s/productconfig"
//	    "github.com/rancher/rancher/pkg/version"
//	)
//
//	var Config = productconfig.New("rancher", version.Version, "cattle-system",
//	    productconfig.WithMetricsSecret("rancher-scc-metrics"),
//	)
//
// Example (with SCC product variant):
//
//	// When the SCC product identifier differs from the base product
//	var Config = productconfig.New("rancher", version.Version, "cattle-system",
//	    productconfig.WithSCCProductIdentifier("rancher-prime"), // SCC API uses "rancher-prime"
//	    productconfig.WithMetricsSecret("rancher-scc-metrics"),
//	)
//	// Result: K8s resources use "rancher", SCC API uses "rancher-prime"
type ProductConfig struct {
	// Product is the base product identifier for Kubernetes resources (e.g., "rancher", "neuvector")
	// Used for: API groups (<product>.registration.suse.com), secret names, CRD naming
	// This should be consistent across all K8s resources for a given product
	Product string `json:"product" yaml:"product"`

	// SCCProductIdentifier is the product identifier sent to SCC API (e.g., "rancher-prime", "neuvector-enterprise")
	// Defaults to Product if not specified.
	// Use this when the SCC product variant differs from the base product identifier.
	// Example: Product="rancher" for all K8s resources, SCCProductIdentifier="rancher-prime" for SCC API
	SCCProductIdentifier string `json:"sccProductIdentifier,omitempty" yaml:"sccProductIdentifier,omitempty"`

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

	// APIBaseDir is the base directory for generated API types (optional, for codegen only)
	// The full path will be constructed as: <APIBaseDir>/<group>/v1
	// Example: "./pkg/apis" results in "./pkg/apis/rancher.registration.suse.com/v1"
	// Defaults to "./apis" if not specified.
	// Note: GenerateOutputDir takes precedence if set (for full path control).
	APIBaseDir string `json:"apiBaseDir,omitempty" yaml:"apiBaseDir,omitempty"`

	// GenerateOutputDir is the full output path where codegen writes files (optional, for codegen only)
	// When set, this overrides APIBaseDir and provides full control over the output location.
	// If neither APIBaseDir nor GenerateOutputDir is set, defaults to "./apis/<group>/v1"
	GenerateOutputDir string `json:"generateOutputDir,omitempty" yaml:"generateOutputDir,omitempty"`

	// ShortNames are kubectl shorthand aliases for the CRD (optional, for codegen only)
	// Example: []string{"rancherreg", "rreg"} allows "kubectl get rancherreg"
	// Useful for disambiguating when multiple products use ProductRegistration in the same cluster
	ShortNames []string `json:"shortNames,omitempty" yaml:"shortNames,omitempty"`

	// SkipSchemeBuilder skips generating SchemeBuilder registration code in doc.go (optional, for codegen only)
	// Set to true if using Wrangler's controller-gen, which generates its own SchemeBuilder in zz_generated.register.go
	// Default (false) generates SchemeBuilder for use with kubebuilder's controller-gen
	SkipSchemeBuilder bool `json:"skipSchemeBuilder,omitempty" yaml:"skipSchemeBuilder,omitempty"`
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
	if c.SCCProductIdentifier == "" {
		c.SCCProductIdentifier = c.Product
	}
	if c.MetricsSecretNamespace == "" {
		c.MetricsSecretNamespace = c.Namespace
	}
	if c.MetricsSecretName == "" {
		c.MetricsSecretName = fmt.Sprintf("%s-scc-metrics", c.Product)
	}

	// Handle codegen output directory:
	// 1. If GenerateOutputDir is explicitly set, use it (full control)
	// 2. Otherwise, construct from APIBaseDir + Group + version
	// 3. If neither is set, default to "./apis"
	if c.GenerateOutputDir == "" {
		baseDir := c.APIBaseDir
		if baseDir == "" {
			baseDir = "./apis"
		}
		c.GenerateOutputDir = fmt.Sprintf("%s/%s/v1", baseDir, c.Group)
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

// WithSCCProductIdentifier sets the SCC product identifier for API calls.
// Use this when the SCC product variant differs from the base product.
// Example: WithSCCProductIdentifier("rancher-prime") when Product is "rancher"
func WithSCCProductIdentifier(identifier string) Option {
	return func(c *ProductConfig) {
		c.SCCProductIdentifier = identifier
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

// WithAPIBaseDir sets the base directory for generated API types.
// The full output path will be constructed as: <baseDir>/<group>/v1
// Example: WithAPIBaseDir("./pkg/apis") results in "./pkg/apis/rancher.registration.suse.com/v1"
// This is the recommended way to customize the output location.
func WithAPIBaseDir(baseDir string) Option {
	return func(c *ProductConfig) {
		c.APIBaseDir = baseDir
	}
}

// WithGenerateOutputDir sets the full output directory path for code generation.
// This provides complete control over the output location, overriding APIBaseDir.
// Use WithAPIBaseDir() instead unless you need to specify the exact full path.
// Defaults to "./apis/<group>/v1" if neither this nor APIBaseDir is specified.
func WithGenerateOutputDir(dir string) Option {
	return func(c *ProductConfig) {
		c.GenerateOutputDir = dir
	}
}

// WithShortNames sets kubectl shorthand aliases for the CRD.
// Example: WithShortNames("rancherreg", "rreg") allows "kubectl get rancherreg"
// This is useful when multiple products use ProductRegistration in the same cluster,
// as the unqualified "kubectl get productregistration" will be ambiguous.
func WithShortNames(names ...string) Option {
	return func(c *ProductConfig) {
		c.ShortNames = names
	}
}

// WithSkipSchemeBuilder skips generating SchemeBuilder registration code.
// Use this when using Wrangler's controller-gen, which generates its own
// SchemeBuilder in zz_generated.register.go
// By default (without this option), SchemeBuilder is generated for kubebuilder's controller-gen.
func WithSkipSchemeBuilder() Option {
	return func(c *ProductConfig) {
		c.SkipSchemeBuilder = true
	}
}

// Note: ToRuntimeConfig() is defined in the parent k8s package
// to avoid circular dependencies. Import the k8s package and call:
//
//	k8s.Setup(mgr, k8s.ConfigFromProduct(myconfig.Config))
//
// Or use the Config type directly if not using ProductConfig.
