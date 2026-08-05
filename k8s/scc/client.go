package scc

import (
	"bytes"
	"context"
	"io"

	k8sclient "github.com/SUSE/connect-ng/k8s/client"
	"github.com/SUSE/connect-ng/pkg/connection"
	"github.com/SUSE/connect-ng/pkg/registration"
)

// Client defines the interface for interacting with SUSE Customer Center (SCC).
// Implementations handle the actual network calls to SCC.
//
// This interface abstracts the underlying connect-ng library to make handlers
// testable and to allow consumers to customize SCC interaction behavior.
type Client interface {
	// Register announces the system to SCC and returns the system ID and credentials.
	// regCode: Registration code from secret
	// hostname: System hostname
	// sysInfo: System information (maps directly to registration.SystemInformation)
	// extraData: Optional extra data (instance data, etc.)
	Register(ctx context.Context, regCode, hostname string, sysInfo map[string]any, extraData map[string]interface{}) (systemID int, login, password string, err error)

	// Activate activates products with SCC.
	// identifier: Product identifier (e.g., "sle-module-containers")
	// version: Product version (e.g., "15.5")
	// arch: Architecture (e.g., "x86_64")
	// regCode: Registration code
	// login, password: System credentials from Register()
	Activate(ctx context.Context, identifier, version, arch, regCode, login, password string) error

	// Keepalive performs a status check with SCC to maintain registration.
	// hostname: System hostname
	// sysInfo: System information (maps directly to registration.SystemInformation)
	// login, password: System credentials
	Keepalive(ctx context.Context, hostname string, sysInfo map[string]any, login, password string) error

	// Deregister removes the system from SCC.
	// login, password: System credentials
	Deregister(ctx context.Context, login, password string) error

	// GenerateOfflineRequest creates a base64-encoded offline registration request.
	// Returns the request data that users upload to SCC portal.
	// sysInfo: System information (maps directly to registration.SystemInformation)
	GenerateOfflineRequest(ctx context.Context, identifier, version, arch string, sysInfo map[string]any) ([]byte, error)

	// ValidateOfflineCertificate validates a certificate uploaded by the user.
	// certificateData: Raw certificate data from secret
	ValidateOfflineCertificate(ctx context.Context, certificateData []byte) error
}

// DefaultClient implements Client using connect-ng library.
type DefaultClient struct {
	// BaseURL is the SCC API endpoint (e.g., "https://scc.suse.com")
	BaseURL string

	// Insecure disables TLS verification (for testing only)
	Insecure bool

	// CustomCA is an optional custom CA certificate for the SCC URL
	CustomCA []byte

	// K8sClient is the Kubernetes client for secret access (required for secret-backed credentials)
	K8sClient k8sclient.Client

	// CredentialsNamespace is the namespace where credentials secret is stored
	CredentialsNamespace string

	// CredentialsSecretName is the name of the credentials secret
	CredentialsSecretName string
}

// NewDefaultClient creates a new SCC client with default configuration.
func NewDefaultClient(baseURL string) *DefaultClient {
	if baseURL == "" {
		baseURL = "https://scc.suse.com"
	}
	return &DefaultClient{
		BaseURL: baseURL,
	}
}

// Register implements Client.Register using connect-ng.
//
// IMPORTANT: Uses secret-backed credentials to ensure the system token
// returned by SCC is immediately persisted.
func (c *DefaultClient) Register(ctx context.Context, regCode, hostname string, sysInfo map[string]any, extraData map[string]interface{}) (int, string, string, error) {
	// Create connection without credentials (will be set during registration)
	conn, creds, err := c.createConnectionWithSecretCreds(ctx, c.K8sClient, c.CredentialsNamespace, c.CredentialsSecretName)
	if err != nil {
		return 0, "", "", err
	}

	systemID, err := registration.Register(
		conn,
		regCode,
		hostname,
		registration.SystemInformation(sysInfo),
		extraData,
	)
	if err != nil {
		return 0, "", "", err
	}

	// Credentials were automatically saved by SecretBackedCredentials.SetLogin()
	// Just retrieve them for return
	login, password, err := creds.Login()
	if err != nil {
		return 0, "", "", err
	}

	return systemID, login, password, nil
}

// Activate implements Client.Activate using connect-ng.
//
// IMPORTANT: Uses secret-backed credentials to ensure the system token
// is persisted after the activation API call.
//
// The arch parameter comes from the product's metrics/telemetry system,
// NOT from static configuration.
func (c *DefaultClient) Activate(ctx context.Context, identifier, version, arch, regCode, login, password string) error {
	conn, _, err := c.createConnectionWithSecretCreds(ctx, c.K8sClient, c.CredentialsNamespace, c.CredentialsSecretName)
	if err != nil {
		return err
	}

	_, _, err = registration.Activate(conn, identifier, version, arch, regCode)
	// Token automatically saved by SecretBackedCredentials.UpdateToken()
	return err
}

// Keepalive implements Client.Keepalive using connect-ng.
//
// IMPORTANT: Uses secret-backed credentials to ensure the system token
// is persisted after the status API call.
func (c *DefaultClient) Keepalive(ctx context.Context, hostname string, sysInfo map[string]any, login, password string) error {
	conn, _, err := c.createConnectionWithSecretCreds(ctx, c.K8sClient, c.CredentialsNamespace, c.CredentialsSecretName)
	if err != nil {
		return err
	}

	_, err = registration.Status(
		conn,
		hostname,
		registration.SystemInformation(sysInfo),
		nil, // profiles
		nil, // extraData
	)
	// Token automatically saved by SecretBackedCredentials.UpdateToken()
	return err
}

// Deregister implements Client.Deregister using connect-ng.
//
// Uses secret-backed credentials for consistency, though token updates
// don't matter since we're destroying the registration.
func (c *DefaultClient) Deregister(ctx context.Context, login, password string) error {
	conn, _, err := c.createConnectionWithSecretCreds(ctx, c.K8sClient, c.CredentialsNamespace, c.CredentialsSecretName)
	if err != nil {
		return err
	}

	return registration.Deregister(conn)
}

// GenerateOfflineRequest implements Client.GenerateOfflineRequest using connect-ng.
//
// The arch parameter comes from the product's metrics/telemetry system,
// NOT from static configuration.
func (c *DefaultClient) GenerateOfflineRequest(ctx context.Context, identifier, version, arch string, sysInfo map[string]any) ([]byte, error) {
	req := registration.BuildOfflineRequest(
		identifier,
		version,
		arch,
		registration.SystemInformation(sysInfo),
	)

	reader, err := req.Base64Encoded()
	if err != nil {
		return nil, err
	}

	return io.ReadAll(reader)
}

// ValidateOfflineCertificate implements Client.ValidateOfflineCertificate using connect-ng.
func (c *DefaultClient) ValidateOfflineCertificate(ctx context.Context, certificateData []byte) error {
	// Parse the certificate
	_, err := registration.OfflineCertificateFrom(bytes.NewReader(certificateData), true)
	if err != nil {
		return err
	}

	// Certificate parsed successfully - validation complete
	// TODO: Add more thorough validation using offlinevalidator if needed
	return nil
}

// createConnectionWithSecretCreds creates a connection with secret-backed credentials.
//
// CRITICAL: Always use this for production. SCC rotates tokens after every non-read
// request, and SecretBackedCredentials ensures these updates are persisted to etcd.
func (c *DefaultClient) createConnectionWithSecretCreds(
	ctx context.Context,
	k8sClient k8sclient.Client,
	namespace, secretName string,
) (*connection.ApiConnection, *SecretBackedCredentials, error) {
	opts := connection.Options{
		URL:    c.BaseURL,
		Secure: !c.Insecure,
	}

	// TODO: Add custom CA support if needed
	// if len(c.CustomCA) > 0 {
	//     opts.CACert = c.CustomCA
	// }

	creds, err := NewSecretBackedCredentials(ctx, k8sClient, namespace, secretName)
	if err != nil {
		return nil, nil, err
	}

	return connection.New(opts, creds), creds, nil
}
