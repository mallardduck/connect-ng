package controller

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/SUSE/connect-ng/k8s/types"
	"github.com/SUSE/connect-ng/k8s/util/jitterbug"
)

const (
	// Production keepalive: 20 hours ± 3 hours (17-23 hours)
	prodBaseCheckin = 20 * time.Hour
	// Dev keepalive: 30 minutes ± 10 minutes (20-40 minutes)
	devBaseCheckin = 30 * time.Minute
)

// LifecycleManagerConfig contains configuration for the lifecycle manager.
type LifecycleManagerConfig struct {
	// Client is used to update ProductRegistration objects
	Client client.Client

	// Prototype is a template instance of the product's CRD type.
	// Used to create proper typed objects.
	Prototype types.ProductRegistrationObject

	// Namespace where ProductRegistration objects are located.
	// If empty, watches all namespaces.
	Namespace string

	// DevMode enables shorter intervals for development.
	// Production: 20h ± 3h, Dev: 30m ± 10m
	DevMode bool

	// HandlerConfig is used to create handlers for mode detection
	HandlerConfig HandlerConfig
}

// setupJitterConfig creates jitterbug config based on dev/prod mode.
// Production: 20h ± 3h, poll every 9 minutes
// Dev: 30m ± 10m, poll every 9 seconds
func setupJitterConfig(devMode bool) *jitterbug.Config {
	if devMode {
		return &jitterbug.Config{
			BaseInterval:    devBaseCheckin,
			JitterMax:       10,
			JitterMaxScale:  time.Minute, // ± 10 minutes
			PollingInterval: 9 * time.Second,
		}
	}
	return &jitterbug.Config{
		BaseInterval:    prodBaseCheckin,
		JitterMax:       3,
		JitterMaxScale:  time.Hour, // ± 3 hours
		PollingInterval: 9 * time.Minute,
	}
}

// RunLifecycleManager starts the jitter-based keepalive lifecycle manager.
// This runs in a background goroutine and never returns.
// It polls all ProductRegistration objects and sets spec.syncNow when keepalive is needed.
//
// Based on scc-operator's RunLifecycleManager in dispatcher.go.
func RunLifecycleManager(ctx context.Context, cfg LifecycleManagerConfig) {
	logger := log.FromContext(ctx).WithName("lifecycle-manager")

	jitterConfig := setupJitterConfig(cfg.DevMode)
	logger.Info("Starting lifecycle manager",
		"baseInterval", jitterConfig.BaseInterval,
		"jitterMax", jitterConfig.JitterMaxDuration(),
		"pollingInterval", jitterConfig.PollingInterval,
		"devMode", cfg.DevMode)

	jitterChecker := jitterbug.NewJitterChecker(
		jitterConfig,
		func(nextTrigger, strictDeadline time.Duration) (bool, error) {
			checkInTriggered := false

			// List all ProductRegistration objects using unstructured
			gvk := cfg.Prototype.GetObjectKind().GroupVersionKind()
			list := &unstructured.UnstructuredList{}
			list.SetGroupVersionKind(gvk)

			listOpts := &client.ListOptions{}
			if cfg.Namespace != "" {
				listOpts.Namespace = cfg.Namespace
			}

			if err := cfg.Client.List(ctx, list, listOpts); err != nil {
				logger.Error(err, "Failed to list ProductRegistration objects")
				return false, err
			}

			logger.V(1).Info("Checking registrations for keepalive",
				"count", len(list.Items),
				"nextTrigger", nextTrigger,
				"strictDeadline", strictDeadline)

			// Check each registration
			for _, item := range list.Items {
				// Convert unstructured to our ProductRegistrationObject
				regObj, err := NewUnstructuredRegistrationObject(&item)
				if err != nil {
					logger.Error(err, "Failed to convert unstructured object",
						"name", item.GetName(),
						"namespace", item.GetNamespace())
					continue
				}

				spec := regObj.GetSpec()
				status := regObj.GetStatus()

				// Create handler to check mode
				handler := createHandler(ctx, regObj, spec.GetMode(), cfg.HandlerConfig)

				// Skip offline mode (no keepalive needed)
				// Skip registrations that haven't progressed to activation
				// Skip registrations that have never been validated
				if spec.GetMode() == types.RegistrationModeOffline ||
					handler.NeedsRegistration(ctx, regObj) ||
					status.GetLastValidatedTS() == nil {
					continue
				}

				lastValidated := status.GetLastValidatedTS()
				timeSinceLastValidation := time.Since(lastValidated.Time)

				// If the time since last validation exceeds the trigger interval,
				// or exceeds the strict deadline, trigger keepalive
				if timeSinceLastValidation >= nextTrigger || timeSinceLastValidation >= strictDeadline {
					checkInTriggered = true

					logger.Info("Triggering keepalive for registration",
						"name", regObj.GetName(),
						"namespace", regObj.GetNamespace(),
						"timeSinceLastValidation", timeSinceLastValidation,
						"nextTrigger", nextTrigger)

					// Set spec.syncNow = true
					// Create a deep copy of the item to update
					itemCopy := item.DeepCopy()
					if err := unstructured.SetNestedField(itemCopy.Object, true, "spec", "syncNow"); err != nil {
						logger.Error(err, "Failed to set spec.syncNow",
							"name", itemCopy.GetName(),
							"namespace", itemCopy.GetNamespace())
						continue
					}

					if err := cfg.Client.Update(ctx, itemCopy); err != nil {
						logger.Error(err, "Failed to update object with syncNow",
							"name", itemCopy.GetName(),
							"namespace", itemCopy.GetNamespace())
						return true, err
					}
				}
			}

			return checkInTriggered, nil
		},
	)

	jitterChecker.Start()
	jitterChecker.Run() // Blocks forever
}

// createHandler creates the appropriate handler based on mode.
// Extracted from RegistrationReconciler to avoid circular dependencies.
func createHandler(
	ctx context.Context,
	obj types.ProductRegistrationObject,
	mode types.RegistrationMode,
	handlerConfig HandlerConfig,
) RegistrationHandler {
	if mode == types.RegistrationModeOffline {
		return NewOfflineHandler(ctx, obj, handlerConfig)
	}
	return NewOnlineHandler(ctx, obj, handlerConfig)
}
