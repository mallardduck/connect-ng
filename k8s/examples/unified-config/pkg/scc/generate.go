package scc

// DEPRECATED: This file used the old go:generate approach.
//
// The recommended approach is to create cmd/generate-scc-types/main.go instead:
//
//   package main
//
//   import (
//       "github.com/SUSE/connect-ng/k8s/codegen"
//       "yourproduct/pkg/scc"
//   )
//
//   func main() {
//       codegen.Generate(scc.Config, nil)
//   }
//
// Then run: go run ./cmd/generate-scc-types
//
// This ensures ProductConfig is the single source of truth without AST parsing.
