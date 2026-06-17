package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/SUSE/connect-ng/k8s/consts"
	"github.com/SUSE/connect-ng/k8s/types"
)

// Note: Keepalive scheduling is now handled by the lifecycle manager in lifecycle.go
// using jitterbug with production intervals of 20h ± 3h and dev intervals of 30m ± 10m.
// The lifecycle manager sets spec.syncNow when keepalive is needed.

// HandlerConfig contains configuration for creating handlers.
type HandlerConfig struct {
	// SecretClient is used to read/write secrets
	SecretClient client.Client

	// Scheme is used for type conversions
	Scheme *runtime.Scheme

	// ProductIdentifier is the SCC product identifier (e.g., "rancher")
	ProductIdentifier string

	// ProductVersion is the product version (e.g., "2.10.0")
	ProductVersion string

	// CredentialsNamespace is the namespace where SCC credentials secret is stored
	CredentialsNamespace string

	// CredentialsSecretName is the name of the SCC credentials secret
	CredentialsSecretName string

	// MetricsSecretNamespace is the namespace containing the metrics secret.
	// The metrics secret is externally populated by the product's telemetry system.
	// The payload MUST include runtime information (hostname, architecture, etc.).
	MetricsSecretNamespace string

	// MetricsSecretName is the name of the metrics secret.
	// The secret must contain a "payload" key with JSON-encoded map[string]any.
	// This data is passed directly to SCC as system information.
	// The payload MUST include runtime information (hostname, architecture, etc.).
	//
	// Example secret:
	//   apiVersion: v1
	//   kind: Secret
	//   metadata:
	//     name: rancher-scc-metrics
	//     namespace: cattle-system
	//   data:
	//     payload: <base64-encoded JSON with hostname, arch, etc.>
	//
	// Based on scc-operator's metrics secret pattern.
	MetricsSecretName string
}

// RegistrationReconciler reconciles ProductRegistration objects.
// Works with any product's CRD via interfaces.
//
// Based on scc-operator's RegistrationReconciler.
// Key design: Creates handler per-reconcile based on spec.mode.
type RegistrationReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// GVK identifies the product-specific CRD kind
	GVK schema.GroupVersionKind

	// Prototype is a template instance of the product's CRD type.
	// Used to create new instances via DeepCopyObject() in Reconcile().
	// This allows the reconciler to use typed CRDs internally while remaining product-agnostic.
	Prototype types.ProductRegistrationObject

	// HandlerConfig is used to create handlers
	handlerConfig HandlerConfig
}

// NewRegistrationReconciler creates a new registration reconciler.
func NewRegistrationReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	gvk schema.GroupVersionKind,
	prototype types.ProductRegistrationObject,
	handlerConfig HandlerConfig,
) *RegistrationReconciler {
	return &RegistrationReconciler{
		Client:        client,
		Scheme:        scheme,
		GVK:           gvk,
		Prototype:     prototype,
		handlerConfig: handlerConfig,
	}
}

// Reconcile handles ProductRegistration objects.
func (r *RegistrationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Create a new instance of the product's CRD type using the prototype
	// This gives us type safety while remaining product-agnostic
	obj := r.Prototype.DeepCopyObject().(types.ProductRegistrationObject)

	if err := r.Get(ctx, req.NamespacedName, obj.(client.Object)); err != nil {
		if errors.IsNotFound(err) {
			// Object deleted, nothing to do
			return ctrl.Result{}, nil
		}
		log.Error(err, "failed to get ProductRegistration")
		return ctrl.Result{}, err
	}

	// Handle deletion with finalizer
	if !obj.GetDeletionTimestamp().IsZero() {
		if containsString(obj.GetFinalizers(), consts.FinalizerRegistration) {
			// Cleanup secrets created by this ProductRegistration
			log.Info("ProductRegistration being deleted, cleaning up secrets")
			if err := r.cleanupSecrets(ctx, obj); err != nil {
				log.Error(err, "failed to cleanup secrets")
				return ctrl.Result{}, err
			}

			// Remove finalizer
			obj.SetFinalizers(removeString(obj.GetFinalizers(), consts.FinalizerRegistration))
			if err := r.Update(ctx, obj.(client.Object)); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !containsString(obj.GetFinalizers(), consts.FinalizerRegistration) {
		obj.SetFinalizers(append(obj.GetFinalizers(), consts.FinalizerRegistration))
		if err := r.Update(ctx, obj.(client.Object)); err != nil {
			return ctrl.Result{}, err
		}
	}

	// obj already implements types.ProductRegistrationObject - use it directly!
	// No wrapper needed - this is the beauty of the prototype pattern
	result, err := r.reconcileRegistration(ctx, obj)
	if err != nil {
		log.Error(err, "failed to reconcile registration")
		return result, err
	}

	return result, nil
}

// reconcileRegistration performs the actual reconciliation logic.
func (r *RegistrationReconciler) reconcileRegistration(
	ctx context.Context,
	regObj types.ProductRegistrationObject,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	spec := regObj.GetSpec()
	status := regObj.GetStatus()

	// Create handler based on current mode (per-reconcile pattern)
	handler := createHandler(ctx, regObj, spec.GetMode(), r.handlerConfig)

	// Handle preprocessing before main state machine
	// This handles edge cases like user removing offline certificate to retry activation
	if handler.NeedsPreprocessRegistration(ctx, regObj) {
		log.Info("Preprocessing registration (user removed certificate or resetting state)")

		preparedObj, err := handler.PreprocessRegistration(ctx, regObj)
		if err != nil {
			log.Error(err, "failed to preprocess registration")
			return ctrl.Result{}, err
		}
		regObj = preparedObj

		// Save preprocessing changes
		if err := r.updateStatus(ctx, regObj); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update status after preprocessing: %w", err)
		}

		// Also update spec if needed (syncNow flag might have been cleared)
		if err := r.Update(ctx, regObj.(client.Object)); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update after preprocessing: %w", err)
		}

		// Requeue to process the new state
		return ctrl.Result{Requeue: true}, nil
	}

	// Check for syncNow trigger from lifecycle manager
	// This is how the jitter-based keepalive system triggers daily checkins
	// Also used to reset/retry failed activations
	if spec.GetSyncNow() {
		log.Info("syncNow triggered")

		// Clear the syncNow flag immediately
		spec.SetSyncNow(false)

		// If system is activated, run keepalive
		if status.GetActivated() {
			log.Info("Running keepalive due to syncNow trigger")
			if err := r.Update(ctx, regObj.(client.Object)); err != nil {
				log.Error(err, "failed to clear syncNow flag")
				return ctrl.Result{}, err
			}
			return r.doKeepalive(ctx, regObj, handler)
		}

		// If not activated, reset to ReadyForActivation for retry
		// This allows user to trigger re-activation via syncNow
		log.Info("syncNow triggered on non-activated registration, resetting to ReadyForActivation")
		resetObj, err := handler.ResetToReadyForActivation(ctx, regObj)
		if err != nil {
			log.Error(err, "failed to reset registration")
			return ctrl.Result{}, err
		}
		regObj = resetObj

		// Save changes
		if err := r.updateStatus(ctx, regObj); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update status after reset: %w", err)
		}
		if err := r.Update(ctx, regObj.(client.Object)); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update after reset: %w", err)
		}

		// Requeue to process activation
		return ctrl.Result{Requeue: true}, nil
	}

	// State machine: NeedsRegistration → Register → NeedsActivation → Activate → NeedsKeepalive → Keepalive

	// 1. Registration phase
	if handler.NeedsRegistration(ctx, regObj) {
		log.Info("System needs registration", "mode", spec.GetMode())
		return r.doRegistration(ctx, regObj, handler)
	}

	// 2. Activation phase
	if handler.NeedsActivation(ctx, regObj) {
		if !handler.ReadyForActivation(ctx, regObj) {
			// Not ready yet (e.g., waiting for user to upload offline certificate)
			log.Info("System not ready for activation", "mode", spec.GetMode())
			return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
		}

		log.Info("System needs activation", "mode", spec.GetMode())
		return r.doActivation(ctx, regObj, handler)
	}

	// 3. Keepalive phase (immediate need, not scheduled)
	if handler.NeedsKeepalive(ctx, regObj) {
		log.Info("System needs keepalive", "mode", spec.GetMode())
		return r.doKeepalive(ctx, regObj, handler)
	}

	// 4. All good - no requeue needed, lifecycle manager handles scheduled keepalives
	log.V(1).Info("System is healthy", "systemID", status.GetSCCSystemID(), "activated", status.GetActivated())
	return ctrl.Result{}, nil
}

// doRegistration performs the registration operation.
func (r *RegistrationReconciler) doRegistration(
	ctx context.Context,
	regObj types.ProductRegistrationObject,
	handler RegistrationHandler,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// 1. Prepare for registration
	preparedObj, err := handler.PrepareForRegister(ctx, regObj)
	if err != nil {
		log.Error(err, "failed to prepare for registration")
		// Update status with error
		regObj = handler.ReconcileRegisterError(ctx, regObj, err)
		if err := r.updateStatus(ctx, regObj); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	}
	regObj = preparedObj

	// Save preparation changes
	if err := r.updateStatus(ctx, regObj); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status after prepare: %w", err)
	}

	// 2. Perform registration
	systemID, err := handler.Register(ctx, regObj)
	if err != nil {
		log.Error(err, "registration failed")
		// Update status with error
		regObj = handler.ReconcileRegisterError(ctx, regObj, err)
		if err := r.updateStatus(ctx, regObj); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	}

	// 3. Update status with registration result
	status := regObj.GetStatus()
	status.SetSCCSystemID(&systemID)
	now := metav1.Now()
	status.SetRegistrationProcessedTS(&now)

	// 4. Prepare for activation
	preparedObj, err = handler.PrepareRegisteredForActivation(ctx, regObj)
	if err != nil {
		log.Error(err, "failed to prepare for activation")
		return ctrl.Result{}, err
	}
	regObj = preparedObj

	// Save status
	if err := r.updateStatus(ctx, regObj); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status after registration: %w", err)
	}

	log.Info("Registration successful", "systemID", systemID)

	// Requeue immediately to proceed to activation
	return ctrl.Result{Requeue: true}, nil
}

// doActivation performs the activation operation.
func (r *RegistrationReconciler) doActivation(
	ctx context.Context,
	regObj types.ProductRegistrationObject,
	handler RegistrationHandler,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// 1. Perform activation
	err := handler.Activate(ctx, regObj)
	if err != nil {
		log.Error(err, "activation failed")
		// Update status with error
		regObj = handler.ReconcileActivateError(ctx, regObj, err)
		if err := r.updateStatus(ctx, regObj); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	}

	// 2. Prepare for keepalive
	preparedObj, err := handler.PrepareActivatedForKeepalive(ctx, regObj)
	if err != nil {
		log.Error(err, "failed to prepare for keepalive")
		return ctrl.Result{}, err
	}
	regObj = preparedObj

	// Save status
	if err := r.updateStatus(ctx, regObj); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status after activation: %w", err)
	}

	log.Info("Activation successful")

	// No requeue needed - lifecycle manager will trigger keepalives via spec.syncNow
	return ctrl.Result{}, nil
}

// doKeepalive performs the keepalive operation.
func (r *RegistrationReconciler) doKeepalive(
	ctx context.Context,
	regObj types.ProductRegistrationObject,
	handler RegistrationHandler,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// 1. Perform keepalive
	err := handler.Keepalive(ctx, regObj)
	if err != nil {
		log.Error(err, "keepalive failed")
		// Update status with error (but don't fail fast - keepalive can be retried)
		regObj = handler.ReconcileKeepaliveError(ctx, regObj, err)
		if err := r.updateStatus(ctx, regObj); err != nil {
			return ctrl.Result{}, err
		}
		// Retry sooner on failure
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	// 2. Update status after successful keepalive
	preparedObj, err := handler.PrepareKeepaliveSucceeded(ctx, regObj)
	if err != nil {
		log.Error(err, "failed to update after keepalive")
		return ctrl.Result{}, err
	}
	regObj = preparedObj

	// Save status
	if err := r.updateStatus(ctx, regObj); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status after keepalive: %w", err)
	}

	log.Info("Keepalive successful")

	// No requeue needed - lifecycle manager will trigger next keepalive via spec.syncNow
	return ctrl.Result{}, nil
}

// updateStatus updates the status subresource of the object.
func (r *RegistrationReconciler) updateStatus(
	ctx context.Context,
	regObj types.ProductRegistrationObject,
) error {
	// regObj is the typed ProductRegistration CR (e.g., &rancherv1.ProductRegistration{})
	// Status changes via regObj.GetStatus() are reflected in the object itself.
	// We just need to persist it via the status subresource.

	// Update the status subresource
	if err := r.Status().Update(ctx, regObj.(client.Object)); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	return nil
}

// cleanupSecrets removes secrets created by this ProductRegistration
func (r *RegistrationReconciler) cleanupSecrets(ctx context.Context, obj types.ProductRegistrationObject) error {
	log := log.FromContext(ctx)
	status := obj.GetStatus()

	// Delete SCC credentials secret if it exists
	if credRef := status.GetSystemCredentialsSecretRef(); credRef != nil {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      credRef.Name,
				Namespace: credRef.Namespace,
			},
		}
		if err := r.Delete(ctx, secret); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete credentials secret: %w", err)
		}
		log.Info("deleted SCC credentials secret", "name", credRef.Name, "namespace", credRef.Namespace)
	}

	// Delete offline request secret if it exists
	if offlineRef := status.GetOfflineRegistrationRequestRef(); offlineRef != nil {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      offlineRef.Name,
				Namespace: offlineRef.Namespace,
			},
		}
		if err := r.Delete(ctx, secret); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete offline request secret: %w", err)
		}
		log.Info("deleted offline request secret", "name", offlineRef.Name, "namespace", offlineRef.Namespace)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RegistrationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Create an unstructured object of the right GVK to watch
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(r.GVK)

	return ctrl.NewControllerManagedBy(mgr).
		For(obj).
		Complete(r)
}
