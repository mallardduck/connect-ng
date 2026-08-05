package lifecycle

import (
	"context"
	"fmt"

	"github.com/SUSE/connect-ng/k8s/api/primitives"
	k8sclient "github.com/SUSE/connect-ng/k8s/client"
	"github.com/SUSE/connect-ng/k8s/contract"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RegistrationState represents the external registration state.
// This is used by external tools (like SUSEConnect) to publish registration state to k8s.
type RegistrationState struct {
	// Mode is the registration mode (online or offline)
	Mode primitives.RegistrationMode

	// SystemID is the SCC system ID (-1 for offline, positive for online)
	SystemID int

	// Activated indicates if the system is activated
	Activated bool

	// RegisteredProduct is the product name from SCC
	RegisteredProduct string

	// RegistrationExpiresAt is when the registration expires (optional)
	RegistrationExpiresAt *metav1.Time

	// LastValidatedTS is when keepalive last succeeded (optional, online only)
	LastValidatedTS *metav1.Time

	// SystemURL is the SCC URL for this system (optional)
	SystemURL *string

	// SystemCredentialsSecretRef points to the credentials secret (optional)
	// External tool should create this secret before publishing state
	SystemCredentialsSecretRef *corev1.SecretReference

	// OfflineRegistrationRequestRef points to offline request secret (optional, offline only)
	OfflineRegistrationRequestRef *corev1.SecretReference

	// Error message if registration failed (optional)
	ErrorMessage string

	// ErrorReason is the condition reason for the error (optional)
	ErrorReason string
}

// StatePublisher provides a simplified API for external tools to publish
// registration state to Kubernetes CRDs.
//
// Use cases:
//  1. SUSEConnect k3s addon - SLES registers k3s, publishes state to cluster
//  2. External registration CLI - drives registration outside k8s, publishes results
//  3. Hybrid scenarios - OS-level tool manages registration, k8s just reads state
//
// The StatePublisher:
//   - Creates or updates ProductRegistration CRs
//   - Sets appropriate conditions based on state
//   - Does NOT call SCC API (caller has already done that)
//   - Does NOT run reconciliation loops (just writes state)
//
// Example (SUSEConnect k3s addon):
//
//	publisher := controller.NewStatePublisher(k8sClient, &v1.ProductRegistration{})
//
//	// After external registration succeeds
//	state := controller.RegistrationState{
//	    Mode:              primitives.RegistrationModeOnline,
//	    SystemID:          12345,
//	    Activated:         true,
//	    RegisteredProduct: "k3s",
//	    SystemURL:         &systemURL,
//	}
//	err := publisher.PublishState(ctx, "k3s-registration", "kube-system", state)
type StatePublisher struct {
	client    k8sclient.Client
	prototype contract.ProductRegistrationObject
}

// NewStatePublisher creates a new state publisher.
//
// Parameters:
//   - client: Kubernetes client for creating/updating CRs
//   - prototype: Instance of the product's CRD type (e.g., &v1.ProductRegistration{})
//     Used to create new CRs with the correct type
func NewStatePublisher(client k8sclient.Client, prototype contract.ProductRegistrationObject) *StatePublisher {
	return &StatePublisher{
		client:    client,
		prototype: prototype,
	}
}

// PublishState creates or updates a ProductRegistration CR to reflect external registration state.
//
// Parameters:
//   - ctx: Context
//   - name: Name of the ProductRegistration CR
//   - namespace: Namespace of the ProductRegistration CR
//   - state: External registration state to publish
//
// The method:
//  1. Gets existing CR or creates new one
//  2. Updates spec with mode
//  3. Updates status with all state fields
//  4. Sets appropriate conditions based on state
//  5. Saves to k8s
//
// Returns error if any k8s operation fails.
func (p *StatePublisher) PublishState(
	ctx context.Context,
	name, namespace string,
	state RegistrationState,
) error {
	// Get existing CR or create new one
	obj := p.prototype.DeepCopyObject().(contract.ProductRegistrationObject)

	err := p.client.Get(ctx, namespace, name, obj.(k8sclient.Object))
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get existing CR: %w", err)
		}

		// Create new CR
		obj.(metav1.Object).SetName(name)
		obj.(metav1.Object).SetNamespace(namespace)

		// Note: Can't set spec.mode via interface - would need type assertion
		// External tools should create CR with correct mode first, or use typed CRD

		// Create
		if err := p.client.Create(ctx, obj.(k8sclient.Object)); err != nil {
			return fmt.Errorf("failed to create CR: %w", err)
		}

		// Re-fetch to get server-populated fields
		if err := p.client.Get(ctx, namespace, name, obj.(k8sclient.Object)); err != nil {
			return fmt.Errorf("failed to re-fetch created CR: %w", err)
		}
	}

	// Update status with external state
	status := obj.GetStatus()

	// Core fields
	status.SetSCCSystemID(&state.SystemID)
	status.SetActivated(state.Activated)

	if state.RegisteredProduct != "" {
		status.SetRegisteredProduct(&state.RegisteredProduct)
	}

	now := metav1.Now()
	status.SetRegistrationProcessedTS(&now)

	if state.RegistrationExpiresAt != nil {
		status.SetRegistrationExpiresAt(state.RegistrationExpiresAt)
	}

	if state.LastValidatedTS != nil {
		status.SetLastValidatedTS(state.LastValidatedTS)
	}

	if state.SystemURL != nil {
		status.SetSystemURL(state.SystemURL)
	}

	// Secret references
	if state.SystemCredentialsSecretRef != nil {
		status.SetSystemCredentialsSecretRef(state.SystemCredentialsSecretRef)
	}

	if state.OfflineRegistrationRequestRef != nil {
		status.SetOfflineRegistrationRequestRef(state.OfflineRegistrationRequestRef)
	}

	// Set conditions based on state
	if state.ErrorMessage != "" {
		// Registration failed
		reason := state.ErrorReason
		if reason == "" {
			reason = "ExternalRegistrationFailed"
		}
		status.SetCondition(primitives.ConditionTypeFailure, metav1.ConditionTrue, reason, state.ErrorMessage)
		status.SetCondition(primitives.ConditionTypeReady, metav1.ConditionFalse, reason, state.ErrorMessage)
	} else if state.Activated {
		// Fully activated
		status.SetCondition(primitives.ConditionTypeAnnounced, metav1.ConditionTrue, "ExternalRegistration", "Registered by external tool")
		status.SetCondition(primitives.ConditionTypeURLReady, metav1.ConditionTrue, "ExternalRegistration", "Registered by external tool")
		status.SetCondition(primitives.ConditionTypeActivated, metav1.ConditionTrue, "ExternalActivation", "Activated by external tool")
		status.SetCondition(primitives.ConditionTypeReady, metav1.ConditionTrue, "ExternalActivation", "System activated by external tool")
		status.SetCondition(primitives.ConditionTypeDone, metav1.ConditionTrue, "ExternalActivation", "Registration complete")

		// Remove failure conditions if any
		status.RemoveCondition(primitives.ConditionTypeFailure)
	} else if state.SystemID > 0 || state.SystemID == -1 {
		// Registered but not activated
		status.SetCondition(primitives.ConditionTypeAnnounced, metav1.ConditionTrue, "ExternalRegistration", "Registered by external tool")
		status.SetCondition(primitives.ConditionTypeURLReady, metav1.ConditionTrue, "ExternalRegistration", "Registered by external tool")
		status.SetCondition(primitives.ConditionTypeProgressing, metav1.ConditionTrue, "AwaitingActivation", "Waiting for activation")

		if state.Mode == primitives.RegistrationModeOffline {
			// Offline mode specific
			status.SetCondition(primitives.ConditionTypeOfflineRequestReady, metav1.ConditionTrue, "ExternalRegistration", "Offline request generated by external tool")

			if state.OfflineRegistrationRequestRef != nil {
				// Waiting for user to upload cert
				status.SetCondition(primitives.ConditionTypeProgressing, metav1.ConditionTrue, "AwaitingCertificate", "Waiting for offline certificate upload")
			}
		}

		// Remove failure conditions if any
		status.RemoveCondition(primitives.ConditionTypeFailure)
	}

	// Save status
	if err := p.client.UpdateStatus(ctx, obj.(k8sclient.Object)); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	return nil
}

// PublishError publishes an error state to a ProductRegistration CR.
// This is a convenience method for reporting external registration failures.
//
// Example:
//
//	err := externalTool.Register()
//	if err != nil {
//	    publisher.PublishError(ctx, "k3s-registration", "kube-system",
//	        "RegistrationFailed", err.Error())
//	}
func (p *StatePublisher) PublishError(
	ctx context.Context,
	name, namespace string,
	reason, message string,
) error {
	return p.PublishState(ctx, name, namespace, RegistrationState{
		ErrorReason:  reason,
		ErrorMessage: message,
	})
}

// GetState retrieves the current registration state from a ProductRegistration CR.
// This is useful for external tools that need to read state before updating.
//
// Returns (state, exists, error):
//   - state: Current registration state
//   - exists: true if CR exists, false if not found
//   - error: any error other than NotFound
func (p *StatePublisher) GetState(
	ctx context.Context,
	name, namespace string,
) (RegistrationState, bool, error) {
	obj := p.prototype.DeepCopyObject().(contract.ProductRegistrationObject)

	err := p.client.Get(ctx, namespace, name, obj.(k8sclient.Object))
	if err != nil {
		if errors.IsNotFound(err) {
			return RegistrationState{}, false, nil
		}
		return RegistrationState{}, false, err
	}

	// Extract state
	spec := obj.GetSpec()
	status := obj.GetStatus()

	state := RegistrationState{
		Mode:                          spec.GetMode(),
		Activated:                     status.GetActivated(),
		LastValidatedTS:               status.GetLastValidatedTS(),
		SystemURL:                     status.GetSystemURL(),
		RegistrationExpiresAt:         status.GetRegistrationExpiresAt(),
		SystemCredentialsSecretRef:    status.GetSystemCredentialsSecretRef(),
		OfflineRegistrationRequestRef: status.GetOfflineRegistrationRequestRef(),
	}

	if systemID := status.GetSCCSystemID(); systemID != nil {
		state.SystemID = *systemID
	}

	if product := status.GetRegisteredProduct(); product != nil {
		state.RegisteredProduct = *product
	}

	// Check for failure condition
	if status.IsConditionTrue(primitives.ConditionTypeFailure) {
		// Get condition manually since interface might not expose GetCondition
		for _, cond := range status.GetConditions() {
			if cond.Type == primitives.ConditionTypeFailure {
				state.ErrorReason = cond.Reason
				state.ErrorMessage = cond.Message
				break
			}
		}
	}

	return state, true, nil
}

// EnsureCredentialsSecret creates or updates the system credentials secret.
// This is a helper for external tools that need to create the credentials secret
// before publishing state.
//
// Parameters:
//   - ctx: Context
//   - namespace: Namespace for the secret
//   - secretName: Name of the secret
//   - username: SCC username (system login)
//   - password: SCC password
//
// Returns the secret reference that can be set in RegistrationState.SystemCredentialsSecretRef
func (p *StatePublisher) EnsureCredentialsSecret(
	ctx context.Context,
	namespace, secretName string,
	username, password string,
) (*corev1.SecretReference, error) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Labels: map[string]string{
				primitives.LabelSecretRole: string(primitives.SecretRoleSCCCredentials),
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			primitives.SecretKeyLogin:    []byte(username),
			primitives.SecretKeyPassword: []byte(password),
		},
	}

	// Try to create
	err := p.client.Create(ctx, secret)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return nil, fmt.Errorf("failed to create credentials secret: %w", err)
		}

		// Update existing
		existing := &corev1.Secret{}
		if err := p.client.Get(ctx, namespace, secretName, existing); err != nil {
			return nil, fmt.Errorf("failed to get existing secret: %w", err)
		}

		existing.Data = secret.Data
		if err := p.client.Update(ctx, existing); err != nil {
			return nil, fmt.Errorf("failed to update credentials secret: %w", err)
		}
	}

	return &corev1.SecretReference{
		Name:      secretName,
		Namespace: namespace,
	}, nil
}
