# CRD Code Generation Path Configuration Examples

This document shows the different ways to configure the output path for CRD code generation.

## Default Behavior (No Configuration)

```go
var Config = productconfig.New(
    "rancher",
    "2.10.0",
    "cattle-system",
)
```

**Output:** `./apis/rancher.registration.suse.com/v1/`

The system automatically constructs the path using:
- Base directory: `./apis` (default)
- API group: `rancher.registration.suse.com` (derived from product)
- Version: `v1` (standard)

## Recommended: Specify Base API Directory

```go
var Config = productconfig.New(
    "rancher",
    "2.10.0",
    "cattle-system",
    productconfig.WithAPIBaseDir("./pkg/apis"),
)
```

**Output:** `./pkg/apis/rancher.registration.suse.com/v1/`

This is the recommended approach when you want to customize the output location:
- You only specify the base directory
- Product-specific path parts (group + version) are added automatically
- Reduces duplication and potential errors
- Automatically updates if the API group changes

## Advanced: Full Path Control

```go
var Config = productconfig.New(
    "rancher",
    "2.10.0",
    "cattle-system",
    productconfig.WithGenerateOutputDir("./internal/registration/api/v1"),
)
```

**Output:** `./internal/registration/api/v1/` (exactly as specified)

Use this when you need complete control over the exact path:
- Path is used exactly as provided
- No automatic path construction
- Useful for non-standard directory structures

## With Custom API Group

```go
var Config = productconfig.New(
    "rancher",
    "2.10.0",
    "cattle-system",
    productconfig.WithGroup("custom.registration.example.com"),
    productconfig.WithAPIBaseDir("./pkg/apis"),
)
```

**Output:** `./pkg/apis/custom.registration.example.com/v1/`

The API base directory works seamlessly with custom API groups.

## Precedence Rules

When both options are specified, `WithGenerateOutputDir()` takes precedence:

```go
var Config = productconfig.New(
    "rancher",
    "2.10.0",
    "cattle-system",
    productconfig.WithAPIBaseDir("./pkg/apis"),          // Ignored
    productconfig.WithGenerateOutputDir("./custom/path"), // Used
)
```

**Output:** `./custom/path/`

This ensures backward compatibility and allows migration from old to new patterns.
