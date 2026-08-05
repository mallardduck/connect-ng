package contract

import (
	"context"

	"github.com/SUSE/connect-ng/k8s/api/primitives"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// CRD-Based Interfaces for ProductRegistration
//
// These interfaces allow the library to work with any product-specific CRD
// (rancher.registration.suse.com, neuvector.registration.suse.com, etc.)
// as long as the CRD implements these contracts.
//
// Grounded in scc-operator's real implementation:
// - RegistrationSpec (pkg/apis/scc.cattle.io/v1/types.go:60-67)
// - RegistrationStatus (pkg/apis/scc.cattle.io/v1/types.go:85-110)
// - SCCHandler interface (pkg/controllers/controller.go:48-87)

// ProductRegistrationSpec defines methods to access registration configuration.
//
// Based on scc-operator's RegistrationSpec:
//   - Mode (online/offline)
//   - RegistrationRequest (registration code secret ref, URL, cert)
//   - OfflineRegistrationCertificateSecretRef
//   - SyncNow trigger
type ProductRegistrationSpec interface {
	// GetMode returns the registration mode (online or offline).
	GetMode() primitives.RegistrationMode

	// GetRegistrationCodeRef returns the secret reference containing the SCC registration code.
	// Corresponds to spec.registrationRequest.registrationCodeSecretRef in scc-operator.
	// May be nil for BAYG/RMT/offline scenarios.
	GetRegistrationCodeRef() *corev1.SecretReference

	// GetRegistrationURL returns the custom SCC URL if provided, otherwise nil.
	// Corresponds to spec.registrationRequest.registrationAPIUrl in scc-operator.
	GetRegistrationURL() *string

	// GetRegistrationURLCertRef returns the secret reference for custom SCC URL certificate.
	// Corresponds to spec.registrationRequest.registrationAPICertificateSecretRef in scc-operator.
	// May be nil if using default SCC.
	GetRegistrationURLCertRef() *corev1.SecretReference

	// GetOfflineCertificateRef returns the secret reference for uploaded offline certificate.
	// Corresponds to spec.offlineRegistrationCertificateSecretRef in scc-operator.
	// Only relevant in offline mode.
	GetOfflineCertificateRef() *corev1.SecretReference

	// GetSyncNow returns true if manual sync is requested.
	// Corresponds to spec.syncNow in scc-operator.
	GetSyncNow() bool

	// SetSyncNow sets the syncNow flag.
	// Used by lifecycle manager to trigger keepalive.
	SetSyncNow(bool)
}

// ProductRegistrationStatus defines methods to access and update registration status.
//
// Based on scc-operator's RegistrationStatus which includes:
//   - Conditions (genericcondition.GenericCondition from wrangler)
//   - CurrentCondition (reference to active condition)
//   - SCCSystemID
//   - RegisteredProduct
//   - RegistrationExpiresAt
//   - ActivationStatus (activated, lastValidatedTS, systemURL)
//   - SystemCredentialsSecretRef
//   - OfflineRegistrationRequest (secret ref for offline request XML)
type ProductRegistrationStatus interface {
	// Condition Management
	//
	// Based on wrangler's genericcondition pattern used in scc-operator.
	// Conditions include: Ready, Done, Progressing, Failure,
	// RegistrationAnnounced, RegistrationSccUrlReady, RegistrationActivated, RegistrationKeepalive,
	// OfflineRequestReady, OfflineCertificateReady, OfflineActivationDone

	// GetConditions returns all conditions.
	GetConditions() []metav1.Condition

	// SetCondition adds or updates a condition.
	SetCondition(condType string, status metav1.ConditionStatus, reason, message string)

	// HasCondition returns true if a condition of the given type exists.
	HasCondition(condType string) bool

	// IsConditionTrue returns true if the condition exists and status is True.
	IsConditionTrue(condType string) bool

	// RemoveCondition removes a condition by type.
	RemoveCondition(condType string)

	// Core Registration Fields

	// GetSCCSystemID returns the SCC system ID assigned after registration.
	// Corresponds to status.sccSystemID in scc-operator.
	// Nil if not yet registered.
	GetSCCSystemID() *int

	// SetSCCSystemID sets the SCC system ID.
	SetSCCSystemID(*int)

	// GetRegisteredProduct returns the product name from SCC.
	// Corresponds to status.registeredProduct in scc-operator.
	GetRegisteredProduct() *string

	// SetRegisteredProduct sets the registered product name.
	SetRegisteredProduct(*string)

	// GetRegistrationProcessedTS returns when registration was last processed.
	// Corresponds to status.registrationProcessedTS in scc-operator.
	GetRegistrationProcessedTS() *metav1.Time

	// SetRegistrationProcessedTS sets the registration processed timestamp.
	SetRegistrationProcessedTS(*metav1.Time)

	// GetRegistrationExpiresAt returns when the registration expires.
	// Corresponds to status.registrationExpiresAt in scc-operator.
	GetRegistrationExpiresAt() *metav1.Time

	// SetRegistrationExpiresAt sets the expiration timestamp.
	SetRegistrationExpiresAt(*metav1.Time)

	// Activation State
	//
	// Mirrors scc-operator's SystemActivationState:
	//   - Activated (bool)
	//   - LastValidatedTS (*metav1.Time)
	//   - SystemURL (*string)

	// GetActivated returns true if the system is activated.
	// Corresponds to status.activationStatus.activated in scc-operator.
	GetActivated() bool

	// SetActivated sets the activation state.
	SetActivated(bool)

	// GetLastValidatedTS returns the last successful keepalive/validation timestamp.
	// Corresponds to status.activationStatus.lastValidatedTS in scc-operator.
	GetLastValidatedTS() *metav1.Time

	// SetLastValidatedTS sets the last validated timestamp.
	SetLastValidatedTS(*metav1.Time)

	// GetSystemURL returns the SCC system URL for UI links.
	// Corresponds to status.activationStatus.systemURL in scc-operator.
	GetSystemURL() *string

	// SetSystemURL sets the system URL.
	SetSystemURL(*string)

	// Secret References

	// GetSystemCredentialsSecretRef returns the secret containing SCC system credentials.
	// Corresponds to status.systemCredentialsSecretRef in scc-operator.
	// Contains login/password returned from SCC after registration.
	GetSystemCredentialsSecretRef() *corev1.SecretReference

	// SetSystemCredentialsSecretRef sets the system credentials secret reference.
	SetSystemCredentialsSecretRef(*corev1.SecretReference)

	// GetOfflineRegistrationRequestRef returns the secret containing offline registration request XML.
	// Corresponds to status.offlineRegistrationRequest in scc-operator.
	// Only relevant in offline mode.
	GetOfflineRegistrationRequestRef() *corev1.SecretReference

	// SetOfflineRegistrationRequestRef sets the offline registration request reference.
	SetOfflineRegistrationRequestRef(*corev1.SecretReference)
}

// ProductRegistrationObject represents a product-specific ProductRegistration CRD.
//
// This is the top-level interface that products must implement.
// Products provide their own CRD type (e.g., rancher.registration.suse.com/v1.ProductRegistration)
// that implements this interface by delegating to embedded library types.
//
// Based on scc-operator's Registration type (pkg/apis/scc.cattle.io/v1/types.go:51-57)
type ProductRegistrationObject interface {
	// Kubernetes object metadata
	metav1.Object

	// Runtime object for controller-runtime client operations
	runtime.Object

	// Registration-specific accessors
	GetSpec() ProductRegistrationSpec
	GetStatus() ProductRegistrationStatus
}

// ProductRegistrationHandler defines the lifecycle operations for registration.
//
// This is the core abstraction that online and offline handlers implement.
// Based on scc-operator's SCCHandler interface (pkg/controllers/controller.go:48-87).
//
// State machine flow:
//
//	Online:  Registration → Activation → Keepalive (every ~20h)
//	Offline: Registration (generate XML) → Activation (validate cert uploaded by user)
//
// IMPORTANT: All methods modify the object in memory but do NOT save it.
// The caller (library controller) is responsible for persisting changes via client.Update().
type ProductRegistrationHandler interface {
	// Decision Methods
	//
	// These methods determine what actions are needed.
	// Based on scc-operator lifecycle deciders (pkg/controllers/lifecycle/deciders.go)

	// NeedsRegistration determines if the system requires initial SCC registration.
	// Returns true if:
	//   - status.registrationProcessedTS is zero (never registered)
	//   - Missing required conditions (RegistrationAnnounced, RegistrationSccUrlReady)
	NeedsRegistration(ctx context.Context, obj ProductRegistrationObject) bool

	// NeedsActivation checks if the system requires activation with SCC.
	// Returns true if:
	//   - status.registrationProcessedTS is zero, OR
	//   - status.activationStatus.activated is false
	NeedsActivation(ctx context.Context, obj ProductRegistrationObject) bool

	// ReadyForActivation checks if the system is ready for activation.
	// Returns true if registration succeeded and activation can proceed.
	ReadyForActivation(ctx context.Context, obj ProductRegistrationObject) bool

	// NeedsKeepalive checks if a keepalive heartbeat is needed (online mode only).
	// Returns true if enough time has passed since last validation.
	// Offline mode always returns false.
	NeedsKeepalive(ctx context.Context, obj ProductRegistrationObject) bool

	// Preparation Methods
	//
	// These methods set up state before operations.
	// Based on scc-operator's Prepare* methods in SCCHandler.

	// PrepareForRegister performs pre-registration setup.
	// Online mode: Creates system credentials secret if missing.
	// Offline mode: Ensures offline request secret is ready.
	// Returns updated object or error.
	PrepareForRegister(ctx context.Context, obj ProductRegistrationObject) (ProductRegistrationObject, error)

	// PrepareRegisteredForActivation updates the object after successful registration.
	// Sets conditions (RegistrationAnnounced, RegistrationSccUrlReady), saves system ID.
	// Returns updated object or error.
	PrepareRegisteredForActivation(ctx context.Context, obj ProductRegistrationObject) (ProductRegistrationObject, error)

	// PrepareActivatedForKeepalive updates the object after successful activation.
	// Sets RegistrationActivated condition, activation status.
	// Online mode: Schedules next keepalive (~20h).
	// Offline mode: Marks as complete (no keepalive needed).
	// Returns updated object or error.
	PrepareActivatedForKeepalive(ctx context.Context, obj ProductRegistrationObject) (ProductRegistrationObject, error)

	// PrepareKeepaliveSucceeded updates the object after successful keepalive.
	// Updates lastValidatedTS, schedules next keepalive.
	// Returns updated object or error.
	PrepareKeepaliveSucceeded(ctx context.Context, obj ProductRegistrationObject) (ProductRegistrationObject, error)

	// Operation Methods
	//
	// These methods perform actual SCC interactions.
	// Based on scc-operator's online.go and offline.go implementations.

	// Register performs the initial system registration with SCC.
	// Online: Calls SCC API, returns real system ID.
	// Offline: Generates registration request XML, stores in secret, returns placeholder ID (-1).
	Register(ctx context.Context, obj ProductRegistrationObject) (systemID int, err error)

	// Activate activates the system with SCC.
	// Online: Calls SCC API to activate products.
	// Offline: Validates that user has uploaded certificate to the referenced secret.
	Activate(ctx context.Context, obj ProductRegistrationObject) error

	// Keepalive sends a heartbeat to SCC and validates system status.
	// Online: Calls SCC API every ~20 hours to maintain registration.
	// Offline: No-op (returns nil immediately).
	Keepalive(ctx context.Context, obj ProductRegistrationObject) error

	// Deregister initiates the system's deregistration from SCC.
	// Online: Calls SCC API to remove system.
	// Offline: Local cleanup only (removes secrets).
	Deregister(ctx context.Context, obj ProductRegistrationObject) error

	// Error Reconciliation Methods
	//
	// These methods handle failures gracefully.
	// Based on scc-operator's Reconcile*Error methods in SCCHandler.

	// ReconcileRegisterError handles registration failures.
	// Sets Failure condition, determines if retry is appropriate based on error type.
	// Returns updated object.
	ReconcileRegisterError(ctx context.Context, obj ProductRegistrationObject, err error) ProductRegistrationObject

	// ReconcileActivateError handles activation failures.
	// Sets Failure condition, determines if retry is appropriate.
	// Returns updated object.
	ReconcileActivateError(ctx context.Context, obj ProductRegistrationObject, err error) ProductRegistrationObject

	// ReconcileKeepaliveError handles keepalive failures.
	// Sets Failure condition.
	// On persistent failure, may invalidate activation to trigger re-registration.
	// Returns updated object.
	ReconcileKeepaliveError(ctx context.Context, obj ProductRegistrationObject, err error) ProductRegistrationObject
}
