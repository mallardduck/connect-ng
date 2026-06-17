// Package k8s provides a batteries-included Kubernetes library for SUSE Customer Center (SCC) registration.
//
// Platform tools (Rancher, NeuVector, Harvester) import this library to handle SCC registration
// without deploying a separate operator. Registration state is managed via product-specific CRDs.
//
// # Quick Start
//
// The simplest way to integrate:
//
//	import k8s "github.com/SUSE/connect-ng/k8s"
//	import "github.com/SUSE/connect-ng/k8s/productconfig"
//	import "github.com/myproduct/myproduct/pkg/version"
//
//	func main() {
//	    mgr, _ := ctrl.NewManager(...)
//
//	    // Setup creates both reconcilers, you control For() and Complete()
//	    regReconciler, entrypointReconciler, _ := k8s.Setup(mgr, productconfig.ProductConfig{
//	        Product:           "myproduct",
//	        Version:           version.Version,
//	        Namespace:         "myproduct-system",
//	        MetricsSecretName: "myproduct-scc-metrics",
//	    })
//
//	    // Register ProductRegistration controller - use your typed CRD or unstructured
//	    ctrl.NewControllerManagedBy(mgr).
//	        For(&myproductv1.ProductRegistration{}).
//	        Complete(regReconciler)
//
//	    // Register entrypoint secret controller - watches {product}-scc-registration secret
//	    ctrl.NewControllerManagedBy(mgr).
//	        For(&corev1.Secret{}).
//	        Complete(entrypointReconciler)
//
//	    mgr.Start(ctrl.SetupSignalHandler())
//	}
//
// This automatically:
//   - Creates an SCC client
//   - Configures online/offline handlers
//   - Sets up the CRD reconciler
//   - Watches <product>.registration.suse.com/v1 ProductRegistration objects
//
// # Architecture: Building Blocks, Not Monolithic Framework
//
// The library provides building blocks that ensure "all products have unified logic and interfaces"
// while giving products control over their implementation choices:
//
// What the Library Provides:
//   - Reconciler: Handles state machine and reconciliation loop
//   - Handlers: OnlineHandler and OfflineHandler for registration modes
//   - SCC Client: Abstraction over SCC API calls
//   - Interfaces: ProductRegistrationObject, ProductRegistrationSpec, ProductRegistrationStatus
//   - Shared Types: Common spec/status implementations in api/v1
//
// What Products Control:
//   - Manager setup and configuration
//   - Scheme registration
//   - CRD generation and ownership
//   - Whether to use Setup() convenience or manual controller setup
//   - Whether to use unstructured CRDs or typed CRDs
//
// Interface-Based Design:
//   - Library defines interfaces (ProductRegistrationObject, ProductRegistrationSpec, ProductRegistrationStatus)
//   - Products can generate thin wrapper CRDs that embed library types
//   - Controller works via interfaces, never imports product types
//   - Handlers created per-reconcile based on spec.mode (online/offline)
//
// # Advanced Usage: Typed CRDs
//
// For full control with compile-time type safety, generate and use typed CRDs:
//
// 1. Generate product-specific CRD types:
//
//	go run github.com/SUSE/connect-ng/k8s/cmd/generate \
//	  --product <product-name> \
//	  --output <output-dir>
//
// 2. Register types and create controller manually:
//
//	import (
//	    myproductv1 "github.com/myproduct/apis/myproduct.registration.suse.com/v1"
//	    "github.com/SUSE/connect-ng/k8s/controller"
//	)
//
//	func main() {
//	    scheme := runtime.NewScheme()
//	    utilruntime.Must(myproductv1.SchemeBuilder.AddToScheme(scheme))
//
//	    mgr, _ := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{Scheme: scheme})
//
//	    reconciler := controller.NewRegistrationReconciler(
//	        mgr.GetClient(),
//	        mgr.GetScheme(),
//	        schema.GroupVersionKind{
//	            Group:   "myproduct.registration.suse.com",
//	            Version: "v1",
//	            Kind:    "ProductRegistration",
//	        },
//	        controller.HandlerConfig{
//	            SecretClient:           mgr.GetClient(),
//	            Scheme:                 mgr.GetScheme(),
//	            ProductIdentifier:      "myproduct",
//	            ProductVersion:         version.Version,
//	            CredentialsNamespace:   "myproduct-system",
//	            CredentialsSecretName:  "myproduct-scc-credentials",
//	            MetricsSecretNamespace: "myproduct-system",
//	            MetricsSecretName:      "myproduct-scc-metrics",
//	        },
//	    )
//
//	    ctrl.NewControllerManagedBy(mgr).
//	        For(&myproductv1.ProductRegistration{}).
//	        Complete(reconciler)
//
//	    mgr.Start(ctrl.SetupSignalHandler())
//	}
//
// # Packages
//
//   - Setup(): High-level one-liner setup (recommended)
//   - scclient: SCC API client implementations
//   - controller: Reconciler and handler implementations
//   - types: Interface definitions
//   - api/v1: Shared type implementations
//   - cmd/generate: Code generation tool (optional)
//
// # Examples
//
// See examples/rancher for a complete integration example.
package k8s
