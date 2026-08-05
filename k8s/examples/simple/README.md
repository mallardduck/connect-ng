# Simple Integration Example

This example shows the minimal code required to add SCC registration to your Kubernetes operator.

## The Code

```go
import k8s "github.com/SUSE/connect-ng/k8s"

func main() {
    mgr, _ := ctrl.NewManager(...)
    
    k8s.Setup(mgr, k8s.Config{
        Product:   "myproduct",
        Version:   "1.0.0",
        Namespace: "myproduct-system",
    })
    
    mgr.Start(ctrl.SetupSignalHandler())
}
```

That's it! **13 lines of code** to add full SCC registration support.

## What You Get

The library automatically handles:

- ✅ Online registration (direct SCC API)
- ✅ Offline registration (air-gapped environments)
- ✅ Product activation
- ✅ Keepalive heartbeats (every ~20 hours)
- ✅ Credential management (stored in Kubernetes secrets)
- ✅ Error handling and retries

## User Workflow

### Online Mode

Users create a ProductRegistration CRD:

```yaml
apiVersion: myproduct.registration.suse.com/v1
kind: ProductRegistration
metadata:
  name: myproduct
spec:
  mode: online
  registrationRequest:
    registrationCodeSecretRef:
      name: myproduct-regcode
      namespace: myproduct-system
```

And a secret with their registration code:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: myproduct-regcode
  namespace: myproduct-system
stringData:
  registrationCode: "YOUR-SCC-REGISTRATION-CODE"
```

The controller automatically:
1. Registers the system with SCC
2. Activates the product
3. Stores credentials in a secret
4. Performs keepalive checks every ~20 hours

### Offline Mode

For air-gapped environments, users set `spec.mode: offline`:

```yaml
apiVersion: myproduct.registration.suse.com/v1
kind: ProductRegistration
metadata:
  name: myproduct
spec:
  mode: offline
  registrationRequest:
    registrationCodeSecretRef:
      name: myproduct-regcode
      namespace: myproduct-system
```

The controller:
1. Generates an offline registration request (base64-encoded)
2. Stores it in a secret for the user to download
3. User uploads the request to https://scc.suse.com/register-offline
4. User downloads the certificate and creates a secret
5. Controller validates the certificate and completes activation

## Advanced Configuration

```go
k8s.Setup(mgr, k8s.Config{
    Product:   "myproduct",
    Version:   "1.0.0",
    Namespace: "myproduct-system",
    
    // Optional: custom SCC URL (e.g., for RMT proxy)
    SCCURL:    "https://rmt.example.com",
    
    // Optional: custom hostname
    Hostname:  "my-cluster-01",
    
    // Optional: custom architecture
    Arch:      "aarch64",
    
    // Optional: custom API group
    Group:     "custom.registration.example.com",
})
```

## Next Steps

- See [examples/rancher](../rancher/) for a more complete example
- See [ARCHITECTURE.md](../../ARCHITECTURE.md) for implementation details
- See [API documentation](https://pkg.go.dev/github.com/SUSE/connect-ng/k8s) for all configuration options
