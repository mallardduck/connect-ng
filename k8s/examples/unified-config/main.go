package main

import (
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	// Import your SCC config package
	"github.com/SUSE/connect-ng/k8s/examples/unified-config/pkg/scc"

	"github.com/SUSE/connect-ng/k8s/reconciler"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// Example: Using unified config for SCC registration
//
// This shows how to use a single ProductConfig for both:
//   - Runtime SCC setup
//   - Code generation
//
// The config is defined once in pkg/scc/config.go and used here.

func main() {
	ctrl.SetLogger(zap.New())

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{})
	if err != nil {
		ctrl.Log.Error(err, "unable to create manager")
		os.Exit(1)
	}

	// ✨ Use the unified config
	// The config is pure data (no dependencies on generated types)
	// This allows it to be used by both runtime and codegen

	// Create unstructured prototype
	// For typed CRDs, generate them first with: go run github.com/SUSE/connect-ng/k8s/cmd/generate
	// Then pass &myproductv1.ProductRegistration{} instead
	prototype, err := reconciler.NewUnstructuredPrototype(scc.Config.Group)
	if err != nil {
		ctrl.Log.Error(err, "unable to create prototype")
		os.Exit(1)
	}

	regReconciler, entrypointReconciler, err := reconciler.Setup(mgr, scc.Config, prototype)
	if err != nil {
		ctrl.Log.Error(err, "unable to setup SCC registration")
		os.Exit(1)
	}

	// Register the ProductRegistration controller with an unstructured object
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   scc.Config.Group,
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
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Secret{}).
		Complete(entrypointReconciler); err != nil {
		ctrl.Log.Error(err, "unable to create entrypoint secret controller")
		os.Exit(1)
	}

	ctrl.Log.Info("SCC registration configured",
		"product", scc.Config.Product,
		"version", scc.Config.Version,
		"namespace", scc.Config.Namespace,
	)

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		ctrl.Log.Error(err, "unable to start manager")
		os.Exit(1)
	}
}
