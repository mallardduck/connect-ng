package reconciler

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/SUSE/connect-ng/k8s/api/primitives"
	k8sclient "github.com/SUSE/connect-ng/k8s/client"
	"github.com/SUSE/connect-ng/k8s/contract"
	"github.com/SUSE/connect-ng/k8s/lifecycle"
)

// Note: Keepalive scheduling is now handled by the lifecycle manager in lifecycle.go
// using jitterbug with production intervals of 20h ± 3h and dev intervals of 30m ± 10m.
// The lifecycle manager sets spec.syncNow when keepalive is needed.

// RegistrationReconciler reconciles ProductRegistration objects.
// Works with any product's CRD via interfaces.
//
// Based on scc-operator's RegistrationReconciler.
// Key design: Creates lifecycle per-reconcile based on spec.mode.
type RegistrationReconciler struct {
	client k8sclient.Client
	Scheme *runtime.Scheme

	// GVK identifies the product-specific CRD kind
	GVK schema.GroupVersionKind

	// Prototype is a template instance of the product's CRD type.
	// Used to create new instances via DeepCopyObject() in Reconcile().
	// This allows the reconciler to use typed CRDs internally while remaining product-agnostic.
	Prototype contract.ProductRegistrationObject

	// HandlerConfig is used to create handlers
	handlerConfig lifecycle.HandlerConfig
}

// NewRegistrationReconciler creates a new registration reconciler.
func NewRegistrationReconciler(
	client k8sclient.Client,
	scheme *runtime.Scheme,
	gvk schema.GroupVersionKind,
	prototype contract.ProductRegistrationObject,
	handlerConfig lifecycle.HandlerConfig,
) *RegistrationReconciler {
	return &RegistrationReconciler{
		client:        client,
		Scheme:        scheme,
		GVK:           gvk,
		Prototype:     prototype,
		handlerConfig: handlerConfig,
	}
}

// Reconcile handles ProductRegistration objects.
func (r *RegistrationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)

	// Create a new instance of the product's CRD type using the prototype
	// This gives us type safety while remaining product-agnostic
	obj := r.Prototype.DeepCopyObject().(contract.ProductRegistrationObject)

	if err := r.client.Get(ctx, req.Namespace, req.Name, obj); err != nil {
		if errors.IsNotFound(err) {
			// Object deleted, nothing to do
			return ctrl.Result{}, nil
		}
		log.Error(err, "failed to get ProductRegistration")
		return ctrl.Result{}, err
	}

	// Handle deletion with finalizer
	if !obj.GetDeletionTimestamp().IsZero() {
		if containsString(obj.GetFinalizers(), primitives.FinalizerRegistration) {
			// Cleanup secrets created by this ProductRegistration
			log.Info("ProductRegistration being deleted, cleaning up secrets")
			if err := r.cleanupSecrets(ctx, obj); err != nil {
				log.Error(err, "failed to cleanup secrets")
				return ctrl.Result{}, err
			}

			// Remove finalizer
			obj.SetFinalizers(removeString(obj.GetFinalizers(), primitives.FinalizerRegistration))
			if err := r.client.Update(ctx, obj); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !containsString(obj.GetFinalizers(), primitives.FinalizerRegistration) {
		obj.SetFinalizers(append(obj.GetFinalizers(), primitives.FinalizerRegistration))
		if err := r.client.Update(ctx, obj); err != nil {
			return ctrl.Result{}, err
		}
	}

	// obj already implements contract.ProductRegistrationObject - use it directly!
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
	regObj contract.ProductRegistrationObject,
) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)

	spec := regObj.GetSpec()
	status := regObj.GetStatus()

	// Create lifecycle based on current mode (per-reconcile pattern)
	h := lifecycle.CreateHandler(ctx, regObj, spec.GetMode(), r.handlerConfig)

	// Wrap lifecycle in driver with client for auto-save
	// The driver encapsulates the registration sequences (register, activate, keepalive)
	driver := lifecycle.NewRegistrationDriver(h, r.client)

	// Handle preprocessing before main state machine
	// This handles edge cases like user removing offline certificate to retry activation
	if driver.NeedsPreprocessRegistration(ctx, regObj) {
		log.Info("Preprocessing registration (user removed certificate or resetting state)")

		if err := driver.Preprocess(ctx, regObj); err != nil {
			log.Error(err, "failed to preprocess registration")
			return ctrl.Result{}, err
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
			if err := r.client.Update(ctx, regObj); err != nil {
				log.Error(err, "failed to clear syncNow flag")
				return ctrl.Result{}, err
			}

			if err := driver.Keepalive(ctx, regObj); err != nil {
				log.Error(err, "keepalive failed")
				return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
			}

			log.Info("Keepalive successful")
			return ctrl.Result{}, nil
		}

		// If not activated, reset to ReadyForActivation for retry
		// This allows user to trigger re-activation via syncNow
		log.Info("syncNow triggered on non-activated registration, resetting to ReadyForActivation")
		if err := driver.ResetToReadyForActivation(ctx, regObj); err != nil {
			log.Error(err, "failed to reset registration")
			return ctrl.Result{}, err
		}

		// Requeue to process activation
		return ctrl.Result{Requeue: true}, nil
	}

	// State machine: NeedsRegistration → Register → NeedsActivation → Activate → NeedsKeepalive → Keepalive

	// 1. Registration phase
	if driver.NeedsRegistration(ctx, regObj) {
		log.Info("System needs registration", "mode", spec.GetMode())

		if err := driver.Register(ctx, regObj); err != nil {
			log.Error(err, "registration failed")
			return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
		}

		log.Info("Registration successful", "systemID", regObj.GetStatus().GetSCCSystemID())
		// Requeue immediately to proceed to activation
		return ctrl.Result{Requeue: true}, nil
	}

	// 2. Activation phase
	if driver.NeedsActivation(ctx, regObj) {
		if !driver.ReadyForActivation(ctx, regObj) {
			// Not ready yet (e.g., waiting for user to upload offline certificate)
			log.Info("System not ready for activation", "mode", spec.GetMode())
			return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
		}

		log.Info("System needs activation", "mode", spec.GetMode())

		if err := driver.Activate(ctx, regObj); err != nil {
			log.Error(err, "activation failed")
			return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
		}

		log.Info("Activation successful")
		// No requeue needed - lifecycle manager will trigger keepalives via spec.syncNow
		return ctrl.Result{}, nil
	}

	// 3. Keepalive phase (immediate need, not scheduled)
	if driver.NeedsKeepalive(ctx, regObj) {
		log.Info("System needs keepalive", "mode", spec.GetMode())

		if err := driver.Keepalive(ctx, regObj); err != nil {
			log.Error(err, "keepalive failed")
			// Retry sooner on failure
			return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
		}

		log.Info("Keepalive successful")
		// No requeue needed - lifecycle manager will trigger next keepalive via spec.syncNow
		return ctrl.Result{}, nil
	}

	// 4. All good - no requeue needed, lifecycle manager handles scheduled keepalives
	log.V(1).Info("System is healthy", "systemID", status.GetSCCSystemID(), "activated", status.GetActivated())
	return ctrl.Result{}, nil
}

// cleanupSecrets removes secrets created by this ProductRegistration
func (r *RegistrationReconciler) cleanupSecrets(ctx context.Context, obj contract.ProductRegistrationObject) error {
	log := logr.FromContextOrDiscard(ctx)
	status := obj.GetStatus()

	// Delete SCC credentials secret if it exists
	if credRef := status.GetSystemCredentialsSecretRef(); credRef != nil {
		secret := &corev1.Secret{}
		err := r.client.Get(ctx, credRef.Namespace, credRef.Name, secret)

		if err == nil {
			// Remove finalizer if present
			if containsString(secret.Finalizers, primitives.FinalizerSCCCredentials) {
				secret.Finalizers = removeString(secret.Finalizers, primitives.FinalizerSCCCredentials)
				if err := r.client.Update(ctx, secret); err != nil {
					return fmt.Errorf("failed to remove finalizer from credentials secret: %w", err)
				}
			}

			// Now delete the secret
			if err := r.client.Delete(ctx, secret); err != nil && !errors.IsNotFound(err) {
				return fmt.Errorf("failed to delete credentials secret: %w", err)
			}
			log.Info("deleted SCC credentials secret", "name", credRef.Name, "namespace", credRef.Namespace)
		} else if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get credentials secret: %w", err)
		}
	}

	// Delete offline request secret if it exists
	if offlineRef := status.GetOfflineRegistrationRequestRef(); offlineRef != nil {
		secret := &corev1.Secret{}
		err := r.client.Get(ctx, offlineRef.Namespace, offlineRef.Name, secret)

		if err == nil {
			// Remove finalizer if present
			if containsString(secret.Finalizers, primitives.FinalizerOfflineRequest) {
				secret.Finalizers = removeString(secret.Finalizers, primitives.FinalizerOfflineRequest)
				if err := r.client.Update(ctx, secret); err != nil {
					return fmt.Errorf("failed to remove finalizer from offline request secret: %w", err)
				}
			}

			// Now delete the secret
			if err := r.client.Delete(ctx, secret); err != nil && !errors.IsNotFound(err) {
				return fmt.Errorf("failed to delete offline request secret: %w", err)
			}
			log.Info("deleted offline request secret", "name", offlineRef.Name, "namespace", offlineRef.Namespace)
		} else if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get offline request secret: %w", err)
		}
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
