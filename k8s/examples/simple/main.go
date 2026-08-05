package main

import (
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/SUSE/connect-ng/k8s/productconfig"
	"github.com/SUSE/connect-ng/k8s/reconciler"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// Example: Minimal integration of SCC registration into a Kubernetes operator
//
// This shows the simplest possible integration - just call k8s.Setup()
// during your controller manager setup.
//
// The library will:
//   - Watch for ProductRegistration CRDs in <product>.registration.suse.com/v1
//   - Handle online and offline registration modes
//   - Manage secrets for credentials and offline requests
//   - Perform keepalive heartbeats every ~20 hours

func main() {
	// Standard controller-runtime setup
	ctrl.SetLogger(zap.New())

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{})
	if err != nil {
		ctrl.Log.Error(err, "unable to create manager")
		os.Exit(1)
	}

	// ✨ Setup SCC registration - returns two reconcilers
	// For production, define config once in pkg/scc/config.go (see examples/unified-config)

	// Create unstructured prototype for the CRD type
	// This example doesn't generate typed CRDs - uses unstructured wrapped in adapter
	prototype, err := reconciler.NewUnstructuredPrototype("myproduct.registration.suse.com")
	if err != nil {
		ctrl.Log.Error(err, "unable to create prototype")
		os.Exit(1)
	}

	regReconciler, entrypointReconciler, err := reconciler.Setup(
		mgr,
		productconfig.ProductConfig{
			Product:           "myproduct",             // Your product name
			Version:           "1.0.0",                 // Your product version
			Namespace:         "myproduct-system",      // Where secrets will be created
			MetricsSecretName: "myproduct-scc-metrics", // Externally populated by telemetry
		},
		prototype, // Pass the unstructured prototype
	)
	if err != nil {
		ctrl.Log.Error(err, "unable to setup SCC registration")
		os.Exit(1)
	}

	// Register the ProductRegistration controller
	// Uses the same unstructured object for the For() call
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "myproduct.registration.suse.com",
		Version: "v1",
		Kind:    "ProductRegistration",
	})

	if err := ctrl.NewControllerManagedBy(mgr).
		For(obj).
		Complete(regReconciler); err != nil {
		ctrl.Log.Error(err, "unable to create ProductRegistration controller")
		os.Exit(1)
	}

	// Register the entrypoint secret controller
	// Watches the myproduct-scc-registration secret and creates ProductRegistration CRs
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Secret{}).
		Complete(entrypointReconciler); err != nil {
		ctrl.Log.Error(err, "unable to create entrypoint secret controller")
		os.Exit(1)
	}

	// Start the controller manager
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		ctrl.Log.Error(err, "unable to start manager")
		os.Exit(1)
	}
}
