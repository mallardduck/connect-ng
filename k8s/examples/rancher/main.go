package main

import (
	"flag"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	// Import the generated types - these were created by running:
	// go run github.com/SUSE/connect-ng/k8s/cmd/generate \
	//   --product rancher \
	//   --output apis/rancher.registration.suse.com/v1
	rancherv1 "github.com/SUSE/connect-ng/k8s/examples/rancher/apis/rancher.registration.suse.com/v1"

	// Import the library setup function
	k8s "github.com/SUSE/connect-ng/k8s"

	// Import our pure config (no dependencies on generated types)
	"github.com/SUSE/connect-ng/k8s/examples/rancher/pkg/scc"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	// Register the generated ProductRegistration types with the scheme
	// Consumers control when/how types are registered
	utilruntime.Must(rancherv1.SchemeBuilder.AddToScheme(scheme))
}

func main() {
	var enableLeaderElection bool
	var clusterID string
	var version string
	var devMode bool

	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager.")
	flag.StringVar(&clusterID, "cluster-id", "", "Cluster ID to use as hostname for SCC registration")
	flag.StringVar(&version, "version", "2.10.0", "Rancher version for SCC registration")
	flag.BoolVar(&devMode, "dev-mode", false,
		"Enable development mode (shorter keepalive intervals: 30m ± 10m instead of 20h ± 3h)")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	if clusterID == "" {
		setupLog.Error(nil, "cluster-id is required")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:           scheme,
		LeaderElection:   enableLeaderElection,
		LeaderElectionID: "rancher-registration.suse.com",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Use the pure config from pkg/scc and override version at runtime
	// This avoids chicken-and-egg: pkg/scc compiles without generated types,
	// generator creates types, main.go imports both
	productConfig := scc.Config
	productConfig.Version = version

	// Setup SCC registration - returns both reconcilers
	// Pass our typed CRD as the prototype - this gives us type safety!
	regReconciler, entrypointReconciler, err := k8s.Setup(
		mgr,
		productConfig,
		&rancherv1.ProductRegistration{}, // Typed prototype - reconciler uses this type internally!
	)
	if err != nil {
		setupLog.Error(err, "unable to setup SCC registration")
		os.Exit(1)
	}

	// Register ProductRegistration controller using our typed CRD
	// This gives us compile-time type safety
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&rancherv1.ProductRegistration{}).
		Complete(regReconciler); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ProductRegistration")
		os.Exit(1)
	}

	// Register entrypoint secret controller
	// Watches the rancher-scc-registration secret and creates ProductRegistration CRs
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Secret{}).
		Complete(entrypointReconciler); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EntrypointSecret")
		os.Exit(1)
	}

	// Start lifecycle manager for jitter-based keepalive scheduling
	// This runs in a background goroutine and sets spec.syncNow when keepalive is needed
	ctx := ctrl.SetupSignalHandler()
	go k8s.StartLifecycleManager(
		ctx,
		mgr,
		productConfig,
		&rancherv1.ProductRegistration{},
		devMode,
	)

	setupLog.Info("starting manager",
		"product", "rancher",
		"version", version,
		"clusterID", clusterID,
		"devMode", devMode)
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
