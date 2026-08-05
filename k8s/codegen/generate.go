package codegen

import (
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/SUSE/connect-ng/k8s/productconfig"
)

// TemplateData contains the data passed to code generation templates.
type TemplateData struct {
	// Product is the base product identifier used for K8s resources
	Product string

	// SCCProductIdentifier is the product identifier sent to SCC API
	// Included in generated types for documentation purposes
	SCCProductIdentifier string

	// Group is the API group (e.g., "rancher.registration.suse.com")
	Group string

	// ShortNames are kubectl aliases for the CRD (e.g., "rancherreg", "rreg")
	ShortNames []string

	// SkipSchemeBuilder skips generating SchemeBuilder code (for Wrangler users)
	SkipSchemeBuilder bool
}

// GenerateOptions configures code generation behavior.
type GenerateOptions struct {
	// OutputDir overrides the default output directory from ProductConfig
	OutputDir string

	// TemplatesDir specifies a custom templates directory
	// If empty, uses embedded templates or default location
	TemplatesDir string

	// Verbose enables detailed output
	Verbose bool
}

// Generate generates CRD types for a product from its ProductConfig.
//
// This is the recommended way for products to generate their SCC registration types.
// Products should create a small codegen program that imports their config:
//
//	// cmd/generate-scc-types/main.go
//	package main
//
//	import (
//	    "github.com/SUSE/connect-ng/k8s/codegen"
//	    "myproduct/pkg/scc"
//	)
//
//	func main() {
//	    if err := codegen.Generate(scc.Config, nil); err != nil {
//	        panic(err)
//	    }
//	}
//
// Then run: go run ./cmd/generate-scc-types
//
// This ensures the ProductConfig is the single source of truth - no duplicate
// JSON/YAML config files needed.
func Generate(cfg productconfig.ProductConfig, opts *GenerateOptions) error {
	if opts == nil {
		opts = &GenerateOptions{}
	}

	// Apply defaults to config
	cfg.ApplyDefaults()

	// Validate config
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid product config: %w", err)
	}

	// Determine base output directory
	baseOutputDir := opts.OutputDir
	if baseOutputDir == "" {
		baseOutputDir = cfg.GenerateOutputDir
	}
	if baseOutputDir == "" {
		return fmt.Errorf("output directory not specified (set in config or options)")
	}

	// Build full path: baseDir/group/version
	// Example: ./pkg/apis/rancher.registration.suse.com/v1
	outputDir := filepath.Join(baseOutputDir, cfg.Group, "v1")

	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	// Prepare template data
	data := TemplateData{
		Product:              cfg.Product,
		SCCProductIdentifier: cfg.SCCProductIdentifier,
		Group:                cfg.Group,
		ShortNames:           cfg.ShortNames,
		SkipSchemeBuilder:    cfg.SkipSchemeBuilder,
	}

	// Find templates directory
	templatesDir := opts.TemplatesDir
	if templatesDir == "" {
		templatesDir = findTemplatesDir()
	}

	// Template files to generate
	templates := map[string]string{
		"types.go.tmpl": "productregistration_types.go",
		"doc.go.tmpl":   "doc.go",
	}

	// Generate each file
	for tmplFile, outFile := range templates {
		tmplPath := filepath.Join(templatesDir, tmplFile)
		outPath := filepath.Join(outputDir, outFile)

		if err := generateFile(tmplPath, outPath, data); err != nil {
			return fmt.Errorf("generating %s: %w", outFile, err)
		}

		if opts.Verbose {
			fmt.Printf("Generated %s\n", outPath)
		}
	}

	if opts.Verbose {
		fmt.Printf("\nSuccess! Generated wrapper type for product '%s' in %s\n", cfg.Product, outputDir)
		fmt.Println("\nNext steps:")
		fmt.Println("1. Run your codegen tooling to generate DeepCopy, List types, and CRDs:")
		fmt.Println("   - Wrangler: go generate")
		fmt.Printf("   - Kubebuilder: controller-gen object paths=%s/...\n", outputDir)
		fmt.Println("2. Import the generated types in your controller code")
	}

	return nil
}

// findTemplatesDir locates the templates directory.
// This handles different execution contexts (go run, installed binary, etc.)
func findTemplatesDir() string {
	// Try several possible locations relative to common execution contexts
	candidates := []string{
		// From connect-ng repo root
		filepath.Join("k8s", "codegen", "templates"),

		// From k8s/ subdirectory
		filepath.Join("codegen", "templates"),

		// From codegen/ subdirectory
		"templates",

		// From product repo importing this library (go.mod will resolve to module cache)
		// We need to look relative to this source file's location in the module cache
		filepath.Join(getModuleRoot(), "codegen", "templates"),

		// Fallback: relative to working directory
		filepath.Join("..", "codegen", "templates"),
		filepath.Join("..", "..", "codegen", "templates"),
	}

	for _, dir := range candidates {
		absDir, _ := filepath.Abs(dir)
		if _, err := os.Stat(absDir); err == nil {
			return absDir
		}
	}

	// Default fallback - will likely fail but provides a clear error
	return filepath.Join("codegen", "templates")
}

// getModuleRoot attempts to find the k8s module root by looking for go.mod
// This helps locate templates when running from a product repo that imports this library
func getModuleRoot() string {
	// Start from current working directory and walk up
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}

	for {
		// Check if this directory contains our module marker (codegen/templates dir)
		templatesPath := filepath.Join(dir, "k8s", "codegen", "templates")
		if _, err := os.Stat(templatesPath); err == nil {
			return filepath.Join(dir, "k8s")
		}

		// Also check for just "codegen/templates" (if we're already in k8s/)
		templatesPath = filepath.Join(dir, "codegen", "templates")
		if _, err := os.Stat(templatesPath); err == nil {
			return dir
		}

		// Move up one directory
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			break
		}
		dir = parent
	}

	return ""
}

// generateFile generates a single file from a template.
func generateFile(tmplPath, outPath string, data TemplateData) error {
	// Read template file
	tmplContent, err := os.ReadFile(tmplPath)
	if err != nil {
		return fmt.Errorf("reading template %s: %w", tmplPath, err)
	}

	// Parse template
	tmpl, err := template.New(filepath.Base(tmplPath)).Parse(string(tmplContent))
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}

	// Create output file
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer f.Close()

	// Execute template
	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("executing template: %w", err)
	}

	return nil
}
