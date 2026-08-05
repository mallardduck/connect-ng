package reconciler

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	k8sclient "github.com/SUSE/connect-ng/k8s/client"
	"github.com/SUSE/connect-ng/k8s/contract"
	"github.com/SUSE/connect-ng/k8s/lifecycle"
	"github.com/SUSE/connect-ng/k8s/productconfig"
)

// NewUnstructuredPrototype creates an unstructured prototype for products that don't generate typed CRDs.
// Use this when you want a quick integration without running the code generator.
//
// Example:
//
//	prototype := k8s.NewUnstructuredPrototype("myproduct.registration.suse.com")
//	regReconciler, entrypointReconciler, _ := k8s.Setup(mgr, config, prototype)
func NewUnstructuredPrototype(group string) (contract.ProductRegistrationObject, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   group,
		Version: "v1",
		Kind:    "ProductRegistration",
	})

	// Wrap in adapter to implement our interfaces
	return lifecycle.NewUnstructuredRegistrationObject(obj)
}

// Setup creates and configures both SCC registration reconcilers for a product.
//
// This is the main entry point for products to integrate SCC registration.
//
// Parameters:
//   - client: k8s client (products create an adapter from their controller-runtime client)
//   - scheme: runtime scheme for type resolution
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
//	import k8s "github.com/SUSE/connect-ng/k8s/reconciler"
//	import "myproduct/pkg/scc"  // Contains pure data config
//	import "myproduct/pkg/adapter"  // Your client adapter
//	import myproductv1 "myproduct/apis/myproduct.registration.suse.com/v1"
//	import corev1 "k8s.io/api/core/v1"
//	import ctrl "sigs.k8s.io/controller-runtime"
//
//	func main() {
//	    mgr, _ := ctrl.NewManager(...)
//
//	    // Create adapter from controller-runtime client to k8s client interface
//	    k8sClient := adapter.NewClient(mgr.GetClient())
//
//	    regReconciler, entrypointReconciler, _ := k8s.Setup(
//	        k8sClient,
//	        mgr.GetScheme(),
//	        scc.Config,
//	        &myproductv1.ProductRegistration{},
//	    )
//
//	    // Register ProductRegistration controller
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
func Setup(client k8sclient.Client, scheme *runtime.Scheme, cfg productconfig.ProductConfig, prototype contract.ProductRegistrationObject) (*RegistrationReconciler, *EntrypointReconciler, error) {
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

	// Create lifecycle config
	// Note: SCC client is created per-reconcile in handlers to respect URL precedence
	// (CR spec > PRIME_SCC_REGISTRATION_HOST_URL env > DEV_MODE > default)
	credentialsSecretName := fmt.Sprintf("%s-scc-credentials", cfg.Product)
	handlerConfig := lifecycle.HandlerConfig{
		SecretClient:           client,
		Scheme:                 scheme,
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
	registrationReconciler := NewRegistrationReconciler(
		client,
		scheme,
		gvk,
		prototype,
		handlerConfig,
	)

	// Create entrypoint secret reconciler
	entrypointSecretName := fmt.Sprintf("%s-scc-registration", cfg.Product)
	entrypointReconciler := &EntrypointReconciler{
		client:               client,
		Scheme:               scheme,
		ProductGroup:         cfg.Group,
		EntrypointSecretName: entrypointSecretName,
		EntrypointNamespace:  cfg.Namespace,
		ProductName:          cfg.Product,
		ProductIdentifier:    cfg.SCCProductIdentifier,
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
//   - client: k8s client (same adapter passed to Setup)
//   - scheme: runtime scheme (same one passed to Setup)
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
//	    k8sClient := adapter.NewClient(mgr.GetClient())
//
//	    regReconciler, entrypointReconciler, _ := k8s.Setup(k8sClient, mgr.GetScheme(), scc.Config, &myproductv1.ProductRegistration{})
//
//	    // Register controllers
//	    ctrl.NewControllerManagedBy(mgr).For(&myproductv1.ProductRegistration{}).Complete(regReconciler)
//	    ctrl.NewControllerManagedBy(mgr).For(&corev1.Secret{}).Complete(entrypointReconciler)
//
//	    // Start lifecycle manager in background
//	    ctx := ctrl.SetupSignalHandler()
//	    devMode := os.Getenv("DEV_MODE") == "true"
//	    go k8s.StartLifecycleManager(ctx, k8sClient, mgr.GetScheme(), scc.Config, &myproductv1.ProductRegistration{}, devMode)
//
//	    // Start manager (blocks)
//	    mgr.Start(ctx)
//	}
func StartLifecycleManager(
	ctx context.Context,
	client k8sclient.Client,
	scheme *runtime.Scheme,
	cfg productconfig.ProductConfig,
	prototype contract.ProductRegistrationObject,
	devMode bool,
) {
	// Create lifecycle config (same as Setup)
	credentialsSecretName := fmt.Sprintf("%s-scc-credentials", cfg.Product)
	handlerConfig := lifecycle.HandlerConfig{
		SecretClient:           client,
		Scheme:                 scheme,
		ProductIdentifier:      cfg.SCCProductIdentifier, // Use SCC-specific identifier for API calls
		ProductVersion:         cfg.Version,
		CredentialsNamespace:   cfg.Namespace,
		CredentialsSecretName:  credentialsSecretName,
		MetricsSecretNamespace: cfg.MetricsSecretNamespace,
		MetricsSecretName:      cfg.MetricsSecretName,
	}

	// Create lifecycle manager config
	lifecycleConfig := LifecycleManagerConfig{
		Client:        client,
		Prototype:     prototype,
		Namespace:     cfg.Namespace,
		DevMode:       devMode,
		HandlerConfig: handlerConfig,
	}

	// Run lifecycle manager (blocks forever)
	RunLifecycleManager(ctx, lifecycleConfig)
}
