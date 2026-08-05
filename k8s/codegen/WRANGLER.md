# Using with Wrangler

Wrangler is Rancher's code generation tool built on top of controller-gen. It's simpler to use than raw controller-gen and handles List types, GroupVersion info, and more automatically.

## Setup

1. **Generate the wrapper type with our codegen**:

```bash
go run ./cmd/generate-scc-types
```

This creates:
- `apis/<group>/v1/productregistration_types.go` - ProductRegistration wrapper
- `apis/<group>/v1/doc.go` - Package documentation

2. **Add Wrangler go:generate directive**:

In your `main.go` or `tools.go`:

```go
//go:generate go run github.com/rancher/wrangler/pkg/controller-gen
```

Or create a `generate.go` file:

```go
package main

//go:generate go run github.com/rancher/wrangler/pkg/controller-gen
```

3. **Run Wrangler**:

```bash
go generate
```

Wrangler will automatically:
- Generate `zz_generated_deepcopy.go` - DeepCopy methods for ProductRegistration
- Generate `zz_generated_list.go` - ProductRegistrationList type
- Generate CRD manifests in `crds/`
- Generate clientsets, listers, informers (if configured)

## Configuration

Wrangler uses `+wrangler:` comments in your code to control generation. The ProductRegistration type already has the necessary kubebuilder markers that Wrangler understands.

### Custom CRD Output Location

By default, Wrangler outputs CRDs to `./crds/`. To customize:

```go
// In your doc.go or types file
// +wrangler:crd:outputDir=config/crd/bases
```

### Disable Clientset Generation

If you only want CRDs and DeepCopy (no clientsets):

```go
// In your doc.go
// +wrangler:skipClientset=true
```

## Complete Example

```go
// cmd/generate-types/main.go
package main

import (
    "github.com/SUSE/connect-ng/k8s/codegen"
    "myproduct/pkg/scc"
)

func main() {
    if err := codegen.Generate(scc.Config, nil); err != nil {
        panic(err)
    }
}
```

```go
// generate.go (at repo root)
package main

//go:generate go run ./cmd/generate-types
//go:generate go run github.com/rancher/wrangler/pkg/controller-gen
```

Then just run:

```bash
go generate
```

This will:
1. Run our codegen to create the wrapper type
2. Run Wrangler to generate DeepCopy, List types, and CRDs

## What Wrangler Generates

```
your-product/
├── apis/<group>/v1/
│   ├── productregistration_types.go    # Our codegen
│   ├── doc.go                          # Our codegen
│   ├── zz_generated_deepcopy.go        # Wrangler
│   └── zz_generated_list.go            # Wrangler
├── crds/
│   └── <group>_productregistration.yaml # Wrangler
└── pkg/generated/                       # Wrangler (if clientsets enabled)
    ├── clientset/
    ├── informers/
    └── listers/
```

## Why Wrangler Works

Our generated ProductRegistration type includes standard kubebuilder markers:

```go
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
type ProductRegistration struct {
    // ...
}
```

Wrangler (which uses controller-gen internally) understands these markers and generates everything needed.

## Comparison to controller-gen

| Feature | Wrangler | controller-gen |
|---------|----------|----------------|
| DeepCopy generation | ✅ Automatic | ✅ Manual command |
| List types | ✅ Generated | ❌ Must write manually |
| CRD manifests | ✅ Auto-output to crds/ | ✅ Manual output dir |
| Clientsets | ✅ Optional | ❌ Need separate tool |
| GroupVersion info | ✅ Generated | ❌ Must write manually |
| Configuration | go:generate directives | CLI flags |

For Rancher products, **Wrangler is recommended** as it handles more boilerplate automatically.
