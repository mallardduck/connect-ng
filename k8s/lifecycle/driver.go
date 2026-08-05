package lifecycle

import (
	"context"
	"fmt"
	"time"

	"github.com/SUSE/connect-ng/k8s/api/primitives"
	k8sclient "github.com/SUSE/connect-ng/k8s/client"
	"github.com/SUSE/connect-ng/k8s/contract"
	"github.com/SUSE/connect-ng/k8s/productconfig"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// RegistrationDriver provides high-level registration workflows without requiring
// a full controller. This is useful for:
//
// 1. Products implementing custom controllers that need SCC logic
// 2. CLI tools that drive registration externally
// 3. External tools (like SUSEConnect) that manage registration and persist state to k8s
//
// The driver wraps a RegistrationHandler and provides simplified methods that:
// - Execute the correct sequence of lifecycle calls
// - Handle error reconciliation
// - Optionally save state to k8s (if client provided)
//
// Example (custom controller):
//
//	driver := controller.NewRegistrationDriver(lifecycle, k8sClient)
//	if err := driver.Register(ctx, obj); err != nil {
//	    return err  // State auto-saved to k8s
//	}
//
// Example (CLI tool):
//
//	driver := controller.NewRegistrationDriver(lifecycle, nil)  // No auto-save
//	if err := driver.Register(ctx, obj); err != nil {
//	    return err
//	}
//	// Manually save obj to k8s or local storage
type RegistrationDriver struct {
	handler RegistrationHandler
	client  k8sclient.Client // Optional - if nil, caller must save manually
}

// NewRegistrationDriver creates a new registration driver.
//
// Parameters:
//   - lifecycle: The registration lifecycle (online or offline)
//   - client: Optional k8s client for auto-saving state. If nil, caller must save manually.
//
// The lifecycle is typically created via NewOnlineHandler or NewOfflineHandler.
//
// For most use cases, prefer NewRegistrationDriverFromConfig which takes ProductConfig.
func NewRegistrationDriver(handler RegistrationHandler, client k8sclient.Client) *RegistrationDriver {
	return &RegistrationDriver{
		handler: handler,
		client:  client,
	}
}

// NewRegistrationDriverFromConfig creates a registration driver from ProductConfig.
// This is the recommended way to create drivers for custom controllers.
//
// Parameters:
//   - ctx: Context
//   - config: Product configuration (use productconfig.New() to ensure defaults)
//   - prototype: Instance of your product's CRD type
//   - client: Kubernetes client for auto-saving state
//   - scheme: Kubernetes scheme for type conversions
//
// The function:
//  1. Validates ProductConfig
//  2. Derives HandlerConfig from ProductConfig
//  3. Creates lifecycle based on prototype.GetSpec().GetMode() (or online if not set)
//  4. Wraps lifecycle in driver with client for auto-save
//
// Example:
//
//	driver, err := controller.NewRegistrationDriverFromConfig(
//	    ctx,
//	    productConfig,
//	    &myproductv1.ProductRegistration{},
//	    mgr.GetClient(),
//	    mgr.GetScheme(),
//	)
//	if err != nil {
//	    return err
//	}
//
//	reconciler := &MyCustomReconciler{
//	    Client: mgr.GetClient(),
//	    driver: driver,
//	}
func NewRegistrationDriverFromConfig(
	ctx context.Context,
	config productconfig.ProductConfig,
	prototype contract.ProductRegistrationObject,
	k8sClient k8sclient.Client,
	scheme *runtime.Scheme,
) (*RegistrationDriver, error) {
	// Validate config
	if config.Product == "" {
		return nil, fmt.Errorf("product is required")
	}
	if config.Version == "" {
		return nil, fmt.Errorf("version is required")
	}
	if config.Namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}

	// Apply defaults (idempotent)
	config.ApplyDefaults()

	// Derive HandlerConfig from ProductConfig
	credentialsSecretName := fmt.Sprintf("%s-scc-credentials", config.Product)
	handlerConfig := HandlerConfig{
		SecretClient:           k8sClient,
		Scheme:                 scheme,
		ProductIdentifier:      config.SCCProductIdentifier,
		ProductVersion:         config.Version,
		CredentialsNamespace:   config.Namespace,
		CredentialsSecretName:  credentialsSecretName,
		MetricsSecretNamespace: config.MetricsSecretNamespace,
		MetricsSecretName:      config.MetricsSecretName,
	}

	// Determine mode from prototype (if already populated) or default to online
	mode := primitives.RegistrationModeOnline
	if spec := prototype.GetSpec(); spec != nil {
		if specMode := spec.GetMode(); specMode != "" {
			mode = specMode
		}
	}

	// Create lifecycle
	handler := createHandler(ctx, prototype, mode, handlerConfig)

	// Wrap in driver
	return NewRegistrationDriver(handler, k8sClient), nil
}

// CreateHandler creates the appropriate lifecycle based on mode.
// This is a convenience method for creating handlers without needing to know the mode upfront.
//
// Example:
//
//	lifecycle := controller.CreateHandler(ctx, obj, spec.GetMode(), handlerConfig)
//	driver := controller.NewRegistrationDriver(lifecycle, client)
func CreateHandler(
	ctx context.Context,
	obj contract.ProductRegistrationObject,
	mode primitives.RegistrationMode,
	config HandlerConfig,
) RegistrationHandler {
	return createHandler(ctx, obj, mode, config)
}

// NeedsRegistration checks if registration is needed.
func (d *RegistrationDriver) NeedsRegistration(ctx context.Context, obj contract.ProductRegistrationObject) bool {
	return d.handler.NeedsRegistration(ctx, obj)
}

// NeedsActivation checks if activation is needed.
func (d *RegistrationDriver) NeedsActivation(ctx context.Context, obj contract.ProductRegistrationObject) bool {
	return d.handler.NeedsActivation(ctx, obj)
}

// ReadyForActivation checks if the system is ready for activation.
func (d *RegistrationDriver) ReadyForActivation(ctx context.Context, obj contract.ProductRegistrationObject) bool {
	return d.handler.ReadyForActivation(ctx, obj)
}

// NeedsKeepalive checks if keepalive is needed.
func (d *RegistrationDriver) NeedsKeepalive(ctx context.Context, obj contract.ProductRegistrationObject) bool {
	return d.handler.NeedsKeepalive(ctx, obj)
}

// NeedsPreprocessRegistration checks if preprocessing is needed.
func (d *RegistrationDriver) NeedsPreprocessRegistration(ctx context.Context, obj contract.ProductRegistrationObject) bool {
	return d.handler.NeedsPreprocessRegistration(ctx, obj)
}

// Register performs the full registration sequence:
//  1. PrepareForRegister (create secrets, set conditions)
//  2. Register (call SCC API or generate offline request)
//  3. Update status with system ID and timestamp
//  4. PrepareRegisteredForActivation (set conditions for next phase)
//  5. Save state (if client provided)
//
// Returns error if any step fails. On error, the object contains reconciled error state.
func (d *RegistrationDriver) Register(ctx context.Context, obj contract.ProductRegistrationObject) error {
	// 1. Prepare
	preparedObj, err := d.handler.PrepareForRegister(ctx, obj)
	if err != nil {
		obj = d.handler.ReconcileRegisterError(ctx, obj, err)
		if saveErr := d.saveStatus(ctx, obj); saveErr != nil {
			return fmt.Errorf("registration prepare failed: %w (save error: %v)", err, saveErr)
		}
		return fmt.Errorf("registration prepare failed: %w", err)
	}
	obj = preparedObj

	// Save preparation state
	if err := d.saveStatus(ctx, obj); err != nil {
		return fmt.Errorf("failed to save after prepare: %w", err)
	}

	// 2. Register
	systemID, err := d.handler.Register(ctx, obj)
	if err != nil {
		obj = d.handler.ReconcileRegisterError(ctx, obj, err)
		if saveErr := d.saveStatus(ctx, obj); saveErr != nil {
			return fmt.Errorf("registration failed: %w (save error: %v)", err, saveErr)
		}
		return fmt.Errorf("registration failed: %w", err)
	}

	// 3. Update status
	status := obj.GetStatus()
	status.SetSCCSystemID(&systemID)
	now := metav1.Now()
	status.SetRegistrationProcessedTS(&now)

	// 4. Prepare for activation
	preparedObj, err = d.handler.PrepareRegisteredForActivation(ctx, obj)
	if err != nil {
		return fmt.Errorf("failed to prepare for activation: %w", err)
	}
	obj = preparedObj

	// 5. Save
	if err := d.saveStatus(ctx, obj); err != nil {
		return fmt.Errorf("failed to save after registration: %w", err)
	}

	return nil
}

// Activate performs the full activation sequence:
//  1. Activate (call SCC API or validate offline certificate)
//  2. PrepareActivatedForKeepalive (set activation status, conditions)
//  3. Save state (if client provided)
//
// Returns error if any step fails. On error, the object contains reconciled error state.
func (d *RegistrationDriver) Activate(ctx context.Context, obj contract.ProductRegistrationObject) error {
	// 1. Activate
	err := d.handler.Activate(ctx, obj)
	if err != nil {
		obj = d.handler.ReconcileActivateError(ctx, obj, err)
		if saveErr := d.saveStatus(ctx, obj); saveErr != nil {
			return fmt.Errorf("activation failed: %w (save error: %v)", err, saveErr)
		}
		return fmt.Errorf("activation failed: %w", err)
	}

	// 2. Prepare for keepalive
	preparedObj, err := d.handler.PrepareActivatedForKeepalive(ctx, obj)
	if err != nil {
		return fmt.Errorf("failed to prepare for keepalive: %w", err)
	}
	obj = preparedObj

	// 3. Save
	if err := d.saveStatus(ctx, obj); err != nil {
		return fmt.Errorf("failed to save after activation: %w", err)
	}

	return nil
}

// Keepalive performs the full keepalive sequence:
//  1. Keepalive (send heartbeat to SCC)
//  2. PrepareKeepaliveSucceeded (update lastValidatedTS, schedule next)
//  3. Save state (if client provided)
//
// Returns error if any step fails. On error, the object contains reconciled error state.
// For keepalive failures, the system remains activated - this is retriable.
func (d *RegistrationDriver) Keepalive(ctx context.Context, obj contract.ProductRegistrationObject) error {
	// 1. Keepalive
	err := d.handler.Keepalive(ctx, obj)
	if err != nil {
		obj = d.handler.ReconcileKeepaliveError(ctx, obj, err)
		if saveErr := d.saveStatus(ctx, obj); saveErr != nil {
			return fmt.Errorf("keepalive failed: %w (save error: %v)", err, saveErr)
		}
		return fmt.Errorf("keepalive failed: %w", err)
	}

	// 2. Update after success
	preparedObj, err := d.handler.PrepareKeepaliveSucceeded(ctx, obj)
	if err != nil {
		return fmt.Errorf("failed to update after keepalive: %w", err)
	}
	obj = preparedObj

	// 3. Save
	if err := d.saveStatus(ctx, obj); err != nil {
		return fmt.Errorf("failed to save after keepalive: %w", err)
	}

	return nil
}

// Preprocess handles preprocessing (e.g., offline certificate removal, retry scenarios).
// Returns error if preprocessing fails.
func (d *RegistrationDriver) Preprocess(ctx context.Context, obj contract.ProductRegistrationObject) error {
	preparedObj, err := d.handler.PreprocessRegistration(ctx, obj)
	if err != nil {
		return fmt.Errorf("preprocessing failed: %w", err)
	}
	obj = preparedObj

	if err := d.saveStatus(ctx, obj); err != nil {
		return fmt.Errorf("failed to save after preprocessing: %w", err)
	}

	// Also update spec if needed (syncNow flag might have been cleared)
	if err := d.saveSpec(ctx, obj); err != nil {
		return fmt.Errorf("failed to update spec after preprocessing: %w", err)
	}

	return nil
}

// ResetToReadyForActivation resets the registration to allow re-activation.
// Used when syncNow is triggered on a failed activation, or when user removes failed offline cert.
func (d *RegistrationDriver) ResetToReadyForActivation(ctx context.Context, obj contract.ProductRegistrationObject) error {
	resetObj, err := d.handler.ResetToReadyForActivation(ctx, obj)
	if err != nil {
		return fmt.Errorf("failed to reset registration: %w", err)
	}
	obj = resetObj

	if err := d.saveStatus(ctx, obj); err != nil {
		return fmt.Errorf("failed to save after reset: %w", err)
	}

	if err := d.saveSpec(ctx, obj); err != nil {
		return fmt.Errorf("failed to update spec after reset: %w", err)
	}

	return nil
}

// Deregister performs system deregistration.
func (d *RegistrationDriver) Deregister(ctx context.Context, obj contract.ProductRegistrationObject) error {
	err := d.handler.Deregister(ctx, obj)
	if err != nil {
		return fmt.Errorf("deregistration failed: %w", err)
	}

	// Save updated state
	if err := d.saveStatus(ctx, obj); err != nil {
		return fmt.Errorf("failed to save after deregistration: %w", err)
	}

	return nil
}

// saveStatus saves the status subresource if client is configured.
// No-op if client is nil.
func (d *RegistrationDriver) saveStatus(ctx context.Context, obj contract.ProductRegistrationObject) error {
	if d.client == nil {
		return nil // Caller will save manually
	}

	if err := d.client.UpdateStatus(ctx, obj.(k8sclient.Object)); err != nil {
		return err
	}

	return nil
}

// saveSpec saves the spec (for syncNow flag, etc.) if client is configured.
// No-op if client is nil.
func (d *RegistrationDriver) saveSpec(ctx context.Context, obj contract.ProductRegistrationObject) error {
	if d.client == nil {
		return nil // Caller will save manually
	}

	if err := d.client.Update(ctx, obj.(k8sclient.Object)); err != nil {
		return err
	}

	return nil
}

// RunFullLifecycle is a convenience method that runs the full registration lifecycle:
//  1. Register (if needed)
//  2. Activate (if needed and ready)
//  3. Keepalive (if needed)
//
// This is useful for one-shot registration flows (e.g., CLI tools).
// Returns (requeue, interval, error):
//   - If requeue is true, caller should retry after interval
//   - If error is non-nil, a failure occurred
//
// Example:
//
//	requeue, interval, err := driver.RunFullLifecycle(ctx, obj)
//	if err != nil {
//	    return err
//	}
//	if requeue {
//	    time.Sleep(interval)
//	    // Try again
//	}
func (d *RegistrationDriver) RunFullLifecycle(
	ctx context.Context,
	obj contract.ProductRegistrationObject,
) (requeue bool, interval time.Duration, err error) {
	// Handle preprocessing
	if d.NeedsPreprocessRegistration(ctx, obj) {
		if err := d.Preprocess(ctx, obj); err != nil {
			return true, 1 * time.Minute, err
		}
		return true, 0, nil // Immediate requeue
	}

	// Handle syncNow
	spec := obj.GetSpec()
	status := obj.GetStatus()
	if spec.GetSyncNow() {
		spec.SetSyncNow(false)

		if status.GetActivated() {
			// Run keepalive
			if err := d.Keepalive(ctx, obj); err != nil {
				return true, 5 * time.Minute, err
			}
			return false, 0, nil
		}

		// Reset for re-activation
		if err := d.ResetToReadyForActivation(ctx, obj); err != nil {
			return true, 1 * time.Minute, err
		}
		return true, 0, nil // Immediate requeue
	}

	// Registration
	if d.NeedsRegistration(ctx, obj) {
		if err := d.Register(ctx, obj); err != nil {
			return true, 1 * time.Minute, err
		}
		return true, 0, nil // Immediate requeue for activation
	}

	// Activation
	if d.NeedsActivation(ctx, obj) {
		if !d.ReadyForActivation(ctx, obj) {
			return true, 1 * time.Minute, nil // Wait for user (e.g., offline cert upload)
		}

		if err := d.Activate(ctx, obj); err != nil {
			return true, 1 * time.Minute, err
		}
		return false, 0, nil // Complete
	}

	// Keepalive (immediate need, not scheduled)
	if d.NeedsKeepalive(ctx, obj) {
		if err := d.Keepalive(ctx, obj); err != nil {
			return true, 5 * time.Minute, err
		}
		return false, 0, nil // Complete
	}

	// All good
	return false, 0, nil
}
