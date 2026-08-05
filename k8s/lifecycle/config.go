package lifecycle

import (
	"context"

	"github.com/SUSE/connect-ng/k8s/api/primitives"
	k8sclient "github.com/SUSE/connect-ng/k8s/client"
	"github.com/SUSE/connect-ng/k8s/contract"
	"k8s.io/apimachinery/pkg/runtime"
)

// HandlerConfig contains configuration for creating handlers.
type HandlerConfig struct {
	// SecretClient is used to read/write secrets
	SecretClient k8sclient.Client

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

// createHandler creates the appropriate lifecycle based on mode.
// This is the internal factory used by CreateHandler (in driver.go) and reconcilers.
func createHandler(
	ctx context.Context,
	obj contract.ProductRegistrationObject,
	mode primitives.RegistrationMode,
	config HandlerConfig,
) RegistrationHandler {
	if mode == primitives.RegistrationModeOffline {
		return NewOfflineHandler(ctx, obj, config)
	}
	return NewOnlineHandler(ctx, obj, config)
}
