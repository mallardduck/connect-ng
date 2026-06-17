package main

import (
	"fmt"
	"os"

	"github.com/SUSE/connect-ng/k8s/codegen"
	"github.com/SUSE/connect-ng/k8s/examples/rancher/pkg/scc"
)

// This is the recommended pattern for generating SCC CRD types.
//
// Benefits:
//   - Single source of truth: scc.Config is used for both runtime and codegen
//   - No duplicate JSON/YAML config files
//   - Type-safe configuration
//   - Works with functional options (WithSCCProductIdentifier, etc.)
//
// Usage:
//
//	go run ./cmd/generate-scc-types
//
// This will generate types in ./apis/rancher.registration.suse.com/v1/
// (or wherever Config.GenerateOutputDir points)
func main() {
	opts := &codegen.GenerateOptions{
		Verbose: true,
	}

	if err := codegen.Generate(scc.Config, opts); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating types: %v\n", err)
		os.Exit(1)
	}
}
