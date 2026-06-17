package productconfig

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// FindConfig searches for a ProductConfig in the current module.
//
// It looks for files matching common patterns:
//   - pkg/scc/config.go
//   - internal/scc/config.go
//   - pkg/registration/config.go
//   - config/scc/config.go
//
// And searches for a variable assignment like:
//
//	var Config = productconfig.ProductConfig{...}
//	var SCCConfig = productconfig.ProductConfig{...}
//
// Returns the config and the file path where it was found.
func FindConfig() (*ProductConfig, string, error) {
	// Common locations where products might define config
	searchPaths := []string{
		"pkg/scc/config.go",
		"internal/scc/config.go",
		"pkg/registration/config.go",
		"internal/registration/config.go",
		"config/scc/config.go",
		"scc.config.go",
	}

	for _, path := range searchPaths {
		if _, err := os.Stat(path); err == nil {
			cfg, err := LoadFromFile(path)
			if err != nil {
				continue // Try next path
			}
			return cfg, path, nil
		}
	}

	return nil, "", fmt.Errorf("no ProductConfig found in common locations (searched: %s)", strings.Join(searchPaths, ", "))
}

// LoadFromFile parses a Go file and extracts ProductConfig literal.
//
// This uses AST parsing to extract the config WITHOUT executing the code,
// so it's safe to run during code generation.
func LoadFromFile(path string) (*ProductConfig, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}

	var cfg *ProductConfig
	ast.Inspect(node, func(n ast.Node) bool {
		// Look for var declarations
		genDecl, ok := n.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.VAR {
			return true
		}

		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok || len(valueSpec.Values) == 0 {
				continue
			}

			// Check if the value is a composite literal (struct initialization)
			compositeLit, ok := valueSpec.Values[0].(*ast.CompositeLit)
			if !ok {
				continue
			}

			// Check if it's a ProductConfig type
			if !isProductConfigType(compositeLit.Type) {
				continue
			}

			// Extract field values
			cfg = extractProductConfig(compositeLit)
			return false // Stop searching
		}

		return true
	})

	if cfg == nil {
		return nil, fmt.Errorf("no ProductConfig found in %s", path)
	}

	return cfg, nil
}

// isProductConfigType checks if an AST type expression refers to ProductConfig.
func isProductConfigType(expr ast.Expr) bool {
	switch t := expr.(type) {
	case *ast.SelectorExpr:
		// packagename.ProductConfig
		ident, ok := t.X.(*ast.Ident)
		return ok && t.Sel.Name == "ProductConfig" && ident.Name == "productconfig"
	case *ast.Ident:
		// ProductConfig (if imported with dot import)
		return t.Name == "ProductConfig"
	}
	return false
}

// extractProductConfig extracts field values from a composite literal.
func extractProductConfig(lit *ast.CompositeLit) *ProductConfig {
	cfg := &ProductConfig{}

	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}

		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}

		value := extractStringValue(kv.Value)

		switch key.Name {
		case "Product":
			cfg.Product = value
		case "Version":
			cfg.Version = value
		case "Namespace":
			cfg.Namespace = value
		case "Group":
			cfg.Group = value
		case "MetricsSecretNamespace":
			cfg.MetricsSecretNamespace = value
		case "MetricsSecretName":
			cfg.MetricsSecretName = value
		case "GenerateOutputDir":
			cfg.GenerateOutputDir = value
		}
	}

	return cfg
}

// extractStringValue attempts to extract a string value from an AST expression.
func extractStringValue(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.BasicLit:
		if v.Kind == token.STRING {
			// Remove quotes
			str, _ := strconv.Unquote(v.Value)
			return str
		}
	case *ast.SelectorExpr:
		// For cases like version.Version, we can't resolve at parse time
		// Just return a placeholder that indicates dynamic value
		return fmt.Sprintf("<dynamic:%s>", v.Sel.Name)
	}
	return ""
}

// LoadFromFileOrFind tries to load from a specific file, falling back to search.
func LoadFromFileOrFind(path string) (*ProductConfig, string, error) {
	if path != "" {
		// Try specific path
		absPath, err := filepath.Abs(path)
		if err != nil {
			return nil, "", err
		}
		cfg, err := LoadFromFile(absPath)
		if err != nil {
			return nil, "", fmt.Errorf("failed to load config from %s: %w", absPath, err)
		}
		return cfg, absPath, nil
	}

	// Fall back to search
	return FindConfig()
}
