package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

type TemplateData struct {
	Product string
}

func main() {
	var product string
	var output string

	flag.StringVar(&product, "product", "", "Product name (e.g., 'rancher', 'neuvector')")
	flag.StringVar(&output, "output", "", "Output directory for generated files")
	flag.Parse()

	if product == "" {
		fmt.Fprintln(os.Stderr, "Error: --product is required")
		flag.Usage()
		os.Exit(1)
	}

	if output == "" {
		fmt.Fprintln(os.Stderr, "Error: --output is required")
		flag.Usage()
		os.Exit(1)
	}

	data := TemplateData{
		Product: product,
	}

	// Ensure output directory exists
	if err := os.MkdirAll(output, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	// Find template directory relative to this executable
	execPath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting executable path: %v\n", err)
		os.Exit(1)
	}

	// Templates are in ../../templates relative to cmd/generate
	templatesDir := filepath.Join(filepath.Dir(execPath), "..", "..", "templates")

	// If running via 'go run', templates are relative to source
	if _, err := os.Stat(templatesDir); os.IsNotExist(err) {
		// Try relative to source file location
		templatesDir = filepath.Join(filepath.Dir(execPath), "..", "..", "templates")
		if _, err := os.Stat(templatesDir); os.IsNotExist(err) {
			// Last resort: check from current working directory
			wd, _ := os.Getwd()
			templatesDir = filepath.Join(wd, "templates")
		}
	}

	// Template files to generate
	templates := map[string]string{
		"types.go.tmpl":        "productregistration_types.go",
		"groupversion.go.tmpl": "groupversion_info.go",
		"doc.go.tmpl":          "doc.go",
	}

	for tmplFile, outFile := range templates {
		tmplPath := filepath.Join(templatesDir, tmplFile)
		outPath := filepath.Join(output, outFile)

		if err := generateFile(tmplPath, outPath, data); err != nil {
			fmt.Fprintf(os.Stderr, "Error generating %s: %v\n", outFile, err)
			os.Exit(1)
		}

		fmt.Printf("Generated %s\n", outPath)
	}

	fmt.Printf("\nSuccess! Generated types for product '%s' in %s\n", product, output)
	fmt.Println("\nNext steps:")
	fmt.Println("1. Run controller-gen to generate DeepCopy methods and CRD manifests:")
	fmt.Printf("   controller-gen object:headerFile=hack/boilerplate.go.txt paths=%s/...\n", output)
	fmt.Printf("   controller-gen crd:crdVersions=v1 paths=%s/... output:crd:dir=config/crd/bases\n", output)
	fmt.Println("2. Import the generated types in your controller code")
	fmt.Println("3. Initialize the registration controller with your product config")
}

func generateFile(tmplPath, outPath string, data TemplateData) error {
	// Read template file
	tmplContent, err := os.ReadFile(tmplPath)
	if err != nil {
		return fmt.Errorf("reading template: %w", err)
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
