package controller

import (
	"context"

	"github.com/SUSE/connect-ng/k8s/types"
)

// RegistrationHandler defines the interface for registration operations.
// Works with ProductRegistrationObject (CRDs) via interfaces.
//
// Implementations: OnlineHandler, OfflineHandler
// Based on scc-operator's SCCHandler interface.
//
// IMPORTANT: All methods modify the object in memory but do NOT save it.
// The caller (reconciler) is responsible for persisting changes via client.Update().
type RegistrationHandler interface {
	// Decision Methods - determine what actions are needed

	NeedsRegistration(ctx context.Context, obj types.ProductRegistrationObject) bool
	NeedsActivation(ctx context.Context, obj types.ProductRegistrationObject) bool
	ReadyForActivation(ctx context.Context, obj types.ProductRegistrationObject) bool
	NeedsKeepalive(ctx context.Context, obj types.ProductRegistrationObject) bool

	// Preparation Methods - set up state before operations

	PrepareForRegister(ctx context.Context, obj types.ProductRegistrationObject) (types.ProductRegistrationObject, error)
	PrepareRegisteredForActivation(ctx context.Context, obj types.ProductRegistrationObject) (types.ProductRegistrationObject, error)
	PrepareActivatedForKeepalive(ctx context.Context, obj types.ProductRegistrationObject) (types.ProductRegistrationObject, error)
	PrepareKeepaliveSucceeded(ctx context.Context, obj types.ProductRegistrationObject) (types.ProductRegistrationObject, error)

	// Operation Methods - perform SCC interactions

	// Register performs the initial system registration with SCC.
	// Online: Calls SCC API, returns real system ID.
	// Offline: Generates registration request XML, stores in secret, returns placeholder ID (-1).
	Register(ctx context.Context, obj types.ProductRegistrationObject) (systemID int, err error)

	// Activate activates the system with SCC.
	// Online: Calls SCC API to activate products.
	// Offline: Validates that user has uploaded certificate to the referenced secret.
	Activate(ctx context.Context, obj types.ProductRegistrationObject) error

	// Keepalive sends a heartbeat to SCC and validates system status.
	// Online: Calls SCC API every ~20 hours to maintain registration.
	// Offline: No-op (returns nil immediately).
	Keepalive(ctx context.Context, obj types.ProductRegistrationObject) error

	// Deregister initiates the system's deregistration from SCC.
	Deregister(ctx context.Context, obj types.ProductRegistrationObject) error

	// Error Reconciliation Methods - handle failures gracefully

	ReconcileRegisterError(ctx context.Context, obj types.ProductRegistrationObject, err error) types.ProductRegistrationObject
	ReconcileActivateError(ctx context.Context, obj types.ProductRegistrationObject, err error) types.ProductRegistrationObject
	ReconcileKeepaliveError(ctx context.Context, obj types.ProductRegistrationObject, err error) types.ProductRegistrationObject
}
