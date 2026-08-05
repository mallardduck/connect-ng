# Unified Config Pattern

This example shows the **recommended** way to integrate SCC registration with **zero configuration duplication**.

## The Pattern

Define your SCC configuration **once** in a Go file, then use it for both runtime and code generation.

### Step 1: Create `pkg/scc/config.go`

```go
package scc

import (
	"github.com/SUSE/connect-ng/k8s/productconfig"
	"github.com/myproduct/myproduct/pkg/version"  // Your existing version package
)

// Config is the single source of truth for SCC registration.
// Both runtime and code generation use this.
var Config = productconfig.ProductConfig{
	Product:           "myproduct",
	Version:           version.Version,  // ✨ Reference your existing version constant
	Namespace:         "myproduct-system",
	MetricsSecretName: "myproduct-scc-metrics", // Externally populated by telemetry
}
```

That's it! **One config, used everywhere.**

### Step 2: Runtime Usage

In your `main.go` or controller setup:

```go
package main

import (
	k8s "github.com/SUSE/connect-ng/k8s"
	"github.com/myproduct/myproduct/pkg/scc"
	ctrl "sigs.k8s.io/controller-runtime"
)

func main() {
	mgr, _ := ctrl.NewManager(...)
	
	// ✨ Use the same config that the generator uses
	k8s.Setup(mgr, k8s.ConfigFromProduct(scc.Config))
	
	mgr.Start(ctrl.SetupSignalHandler())
}
```

### Step 3: Code Generation

Create a small codegen program in `cmd/generate-scc-types/main.go`:

```go
package main

import (
    "github.com/SUSE/connect-ng/k8s/codegen"
    "yourproduct/pkg/scc"
)

func main() {
    if err := codegen.Generate(scc.Config, nil); err != nil {
        panic(err)
    }
}
```

Then run:

```bash
go run ./cmd/generate-scc-types
```

The generator:
1. Uses your `scc.Config` directly (no AST parsing needed)
2. Generates types in `./apis/<product>.registration.suse.com/v1/`
3. Works with functional options like `WithSCCProductIdentifier()`

**Why this works:**
- ✅ True single source of truth - imports your config directly
- ✅ No config duplication or parsing issues
- ✅ Type-safe - compile errors if config is invalid
- ✅ Works with all ProductConfig features (functional options, etc.)
- ✅ Can be wrapped in `Makefile` for full generation pipeline

## Benefits

✅ **Zero duplication** - Config defined once  
✅ **Type-safe** - Uses Go structs, not YAML/JSON  
✅ **Version-aware** - Can reference your existing `version.Version` constant  
✅ **Compile-time validation** - Errors caught by compiler  
✅ **No new tooling** - Just Go code  

## Advanced: Environment-Specific Config

You can create multiple configs if needed:

```go
package scc

import "github.com/SUSE/connect-ng/k8s/productconfig"

var (
	// Production config
	Config = productconfig.ProductConfig{
		Product:   "myproduct",
		Version:   version.Version,
		Namespace: "myproduct-system",
	}
	
	// Development config (optional)
	DevConfig = productconfig.ProductConfig{
		Product:   "myproduct-dev",
		Version:   version.Version + "-dev",
		Namespace: "myproduct-dev",
		SCCURL:    "https://scc-dev.example.com",  // Dev SCC instance
	}
)
```

Then choose at runtime:

```go
cfg := scc.Config
if os.Getenv("DEV_MODE") == "true" {
	cfg = scc.DevConfig
}
k8s.Setup(mgr, cfg.ToRuntimeConfig())
```

## Complete Workflow

```bash
# 1. Initial setup - create config once
cat > pkg/scc/config.go <<EOF
package scc
import "github.com/SUSE/connect-ng/k8s/productconfig"
var Config = productconfig.ProductConfig{
    Product: "myproduct",
    Version: version.Version,
    Namespace: "myproduct-system",
}
EOF

# 2. Create codegen program
cat > cmd/generate-scc-types/main.go <<EOF
package main
import (
    "github.com/SUSE/connect-ng/k8s/codegen"
    "myproduct/pkg/scc"
)
func main() {
    codegen.Generate(scc.Config, nil)
}
EOF

# 3. Generate all code
go run ./cmd/generate-scc-types         # Generate SCC types
controller-gen object paths=./apis/...  # Generate DeepCopy
controller-gen crd paths=./apis/...     # Generate CRD manifests

# 4. Use in runtime
# (scc.Config in main.go)

# 5. Version bump? Just update version.Version - everything else follows!
```

## Comparison to Old Pattern

### ❌ Old Way (Duplication)

```go
// main.go
k8s.Setup(mgr, k8s.Config{
	Product: "myproduct",
	Version: "1.0.0",  // ⚠️ Hardcoded version
})

// Separate codegen invocation
go run .../cmd/generate \
	--product myproduct \
	--version 1.0.0  # ⚠️ Duplicated!
```

**Problems:**
- Version hardcoded in two places
- Easy to forget updating both  
- No connection to your actual version constant
- Product config not the single source of truth

### ✅ New Way (Single Source of Truth)

```go
// pkg/scc/config.go
var Config = productconfig.ProductConfig{
	Product: "myproduct",
	Version: version.Version,  // ✨ Uses your actual version
}

// main.go
k8s.Setup(mgr, scc.Config.ToRuntimeConfig())

// Makefile
generate:
	go run .../generate  # ✨ Auto-finds config
```

**Benefits:**
- One config
- Version comes from your version constant
- Generator auto-discovers it
- Impossible to get out of sync

## Next Steps

See [../simple](../simple/) for a minimal example without code generation.
