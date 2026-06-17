package k8s

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/SUSE/connect-ng/k8s/controller"
	"github.com/SUSE/connect-ng/k8s/productconfig"
	"github.com/SUSE/connect-ng/k8s/types"
)

// NewUnstructuredPrototype creates an unstructured prototype for products that don't generate typed CRDs.
// Use this when you want a quick integration without running the code generator.
//
// Example:
//
//	prototype := k8s.NewUnstructuredPrototype("myproduct.registration.suse.com")
//	regReconciler, entrypointReconciler, _ := k8s.Setup(mgr, config, prototype)
func NewUnstructuredPrototype(group string) (types.ProductRegistrationObject, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   group,
		Version: "v1",
		Kind:    "ProductRegistration",
	})

	// Wrap in adapter to implement our interfaces
	return controller.NewUnstructuredRegistrationObject(obj)
}

// Setup creates and configures both SCC registration reconcilers for a product.
//
// This is the main entry point for products to integrate SCC registration.
//
// Parameters:
//   - mgr: controller-runtime manager
//   - cfg: product configuration (use productconfig.New() to ensure defaults are applied)
//   - prototype: instance of your product's CRD type (e.g., &myproductv1.ProductRegistration{})
//     This allows the reconciler to use your typed CRD internally while remaining product-agnostic
//
// Returns three values:
//   - registrationReconciler: Handles ProductRegistration CRD reconciliation
//   - entrypointReconciler: Watches entrypoint secret and creates ProductRegistration CRs
//   - error: Any setup errors
//
// The prototype parameter closes the loop: products generate typed CRDs, pass them back to the library,
// and the library uses those types internally for type safety while working via interfaces.
//
// Recommended pattern:
//
//	// In pkg/scc/config.go (no dependencies on generated types)
//	var Config = productconfig.New("myproduct", "1.0.0", "myproduct-system",
//	    productconfig.WithMetricsSecret("myproduct-scc-metrics"),
//	)
//
//	// In main.go
//	cfg := scc.Config
//	cfg.Version = actualVersion  // Override version at runtime if needed
//	regReconciler, entrypointReconciler, _ := k8s.Setup(mgr, cfg, &myproductv1.ProductRegistration{})
//
// Example (with typed CRDs):
//
//	import k8s "github.com/SUSE/connect-ng/k8s"
//	import "myproduct/pkg/scc"  // Contains pure data config
//	import myproductv1 "myproduct/apis/myproduct.registration.suse.com/v1"  // Generated types
//	import corev1 "k8s.io/api/core/v1"
//
//	func main() {
//	    mgr, _ := ctrl.NewManager(...)
//
//	    regReconciler, entrypointReconciler, _ := k8s.Setup(
//	        mgr,
//	        scc.Config,                          // Pure data config (no generated type dependencies)
//	        &myproductv1.ProductRegistration{},  // Your generated typed CRD
//	    )
//
//	    // Register ProductRegistration controller - uses your typed CRD
//	    ctrl.NewControllerManagedBy(mgr).
//	        For(&myproductv1.ProductRegistration{}).
//	        Complete(regReconciler)
//
//	    // Register entrypoint secret controller
//	    ctrl.NewControllerManagedBy(mgr).
//	        For(&corev1.Secret{}).
//	        Complete(entrypointReconciler)
//
//	    mgr.Start(ctrl.SetupSignalHandler())
//	}
func Setup(mgr ctrl.Manager, cfg productconfig.ProductConfig, prototype types.ProductRegistrationObject) (*controller.RegistrationReconciler, *controller.EntrypointReconciler, error) {
	// Validate required fields
	if cfg.Product == "" {
		return nil, nil, fmt.Errorf("product is required")
	}
	if cfg.Version == "" {
		return nil, nil, fmt.Errorf("version is required")
	}
	if cfg.Namespace == "" {
		return nil, nil, fmt.Errorf("namespace is required")
	}
	if prototype == nil {
		return nil, nil, fmt.Errorf("prototype is required (pass an instance of your product's CRD type)")
	}

	// Apply defaults (idempotent - safe if already applied via productconfig.New())
	// Kept for backward compatibility with configs created via struct literals
	cfg.ApplyDefaults()

	// Create handler config
	// Note: SCC client is created per-reconcile in handlers to respect URL precedence
	// (CR spec > PRIME_SCC_REGISTRATION_HOST_URL env > DEV_MODE > default)
	credentialsSecretName := fmt.Sprintf("%s-scc-credentials", cfg.Product)
	handlerConfig := controller.HandlerConfig{
		SecretClient:           mgr.GetClient(),
		Scheme:                 mgr.GetScheme(),
		ProductIdentifier:      cfg.SCCProductIdentifier, // Use SCC-specific identifier for API calls
		ProductVersion:         cfg.Version,
		CredentialsNamespace:   cfg.Namespace,
		CredentialsSecretName:  credentialsSecretName,
		MetricsSecretNamespace: cfg.MetricsSecretNamespace,
		MetricsSecretName:      cfg.MetricsSecretName,
	}

	// Create GVK for the product's CRD
	gvk := schema.GroupVersionKind{
		Group:   cfg.Group,
		Version: "v1",
		Kind:    "ProductRegistration",
	}

	// Create registration reconciler with the typed prototype
	// This allows the reconciler to use your typed CRD internally
	registrationReconciler := controller.NewRegistrationReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		gvk,
		prototype,
		handlerConfig,
	)

	// Create entrypoint secret reconciler
	entrypointSecretName := fmt.Sprintf("%s-scc-registration", cfg.Product)
	entrypointReconciler := &controller.EntrypointReconciler{
		Client:               mgr.GetClient(),
		Scheme:               mgr.GetScheme(),
		ProductGroup:         cfg.Group,
		EntrypointSecretName: entrypointSecretName,
		EntrypointNamespace:  cfg.Namespace,
		ProductName:          cfg.Product,
	}

	// Return both reconcilers - product calls For() and Complete() themselves
	return registrationReconciler, entrypointReconciler, nil
}

// StartLifecycleManager starts the jitter-based keepalive lifecycle manager.
// This should be called AFTER Setup() and AFTER registering the controllers with the manager.
//
// The lifecycle manager runs in a background goroutine and handles scheduled keepalive checks
// using jitter to prevent thundering herd. It sets spec.syncNow when keepalive is needed,
// which triggers the reconciler to run keepalive.
//
// Based on scc-operator's RunLifecycleManager pattern.
//
// Parameters:
//   - ctx: Context for the lifecycle manager (typically ctrl.SetupSignalHandler())
//   - mgr: controller-runtime manager (same one passed to Setup)
//   - cfg: product configuration (same one passed to Setup)
//   - prototype: instance of your product's CRD type (same one passed to Setup)
//   - devMode: if true, uses shorter intervals (30m ± 10m) for development/testing
//     if false, uses production intervals (20h ± 3h)
//
// The lifecycle manager never returns - it runs until the context is canceled.
//
// Example usage:
//
//	func main() {
//	    mgr, _ := ctrl.NewManager(...)
//
//	    regReconciler, entrypointReconciler, _ := k8s.Setup(mgr, scc.Config, &myproductv1.ProductRegistration{})
//
//	    // Register controllers
//	    ctrl.NewControllerManagedBy(mgr).For(&myproductv1.ProductRegistration{}).Complete(regReconciler)
//	    ctrl.NewControllerManagedBy(mgr).For(&corev1.Secret{}).Complete(entrypointReconciler)
//
//	    // Start lifecycle manager in background
//	    ctx := ctrl.SetupSignalHandler()
//	    devMode := os.Getenv("DEV_MODE") == "true"
//	    go k8s.StartLifecycleManager(ctx, mgr, scc.Config, &myproductv1.ProductRegistration{}, devMode)
//
//	    // Start manager (blocks)
//	    mgr.Start(ctx)
//	}
func StartLifecycleManager(
	ctx context.Context,
	mgr ctrl.Manager,
	cfg productconfig.ProductConfig,
	prototype types.ProductRegistrationObject,
	devMode bool,
) {
	// Create handler config (same as Setup)
	credentialsSecretName := fmt.Sprintf("%s-scc-credentials", cfg.Product)
	handlerConfig := controller.HandlerConfig{
		SecretClient:           mgr.GetClient(),
		Scheme:                 mgr.GetScheme(),
		ProductIdentifier:      cfg.SCCProductIdentifier, // Use SCC-specific identifier for API calls
		ProductVersion:         cfg.Version,
		CredentialsNamespace:   cfg.Namespace,
		CredentialsSecretName:  credentialsSecretName,
		MetricsSecretNamespace: cfg.MetricsSecretNamespace,
		MetricsSecretName:      cfg.MetricsSecretName,
	}

	// Create lifecycle manager config
	lifecycleConfig := controller.LifecycleManagerConfig{
		Client:        mgr.GetClient(),
		Prototype:     prototype,
		Namespace:     cfg.Namespace,
		DevMode:       devMode,
		HandlerConfig: handlerConfig,
	}

	// Run lifecycle manager (blocks forever)
	controller.RunLifecycleManager(ctx, lifecycleConfig)
}
