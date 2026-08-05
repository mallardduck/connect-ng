package scc

import (
	"context"
	"fmt"
	"sync"

	"github.com/SUSE/connect-ng/k8s/api/primitives"
	k8sclient "github.com/SUSE/connect-ng/k8s/client"
	"github.com/SUSE/connect-ng/pkg/connection"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SecretBackedCredentials implements connection.Credentials with thread-safe secret persistence.
//
// CRITICAL: SCC rotates the system token after EVERY non-read request.
// The token MUST be saved back to the secret immediately after each update.
// Multiple concurrent SCC requests could race and corrupt the token if not properly locked.
//
// This implementation uses a mutex to ensure:
//   - Only one goroutine can update credentials at a time
//   - Reads and writes to the secret are atomic
//   - Token updates are persisted immediately and in order
//
// Based on scc-operator's CredentialSecretsAdapter pattern (battle-tested in production).
type SecretBackedCredentials struct {
	// mu protects all credential fields and secret access
	mu sync.Mutex

	// In-memory cache of credentials
	systemLogin string
	password    string
	systemToken string

	// Secret details
	secretNamespace string
	secretName      string

	// Kubernetes client for secret access
	client k8sclient.Client
	ctx    context.Context
}

// NewSecretBackedCredentials creates credentials backed by a Kubernetes secret.
//
// The secret will be created if it doesn't exist.
// All credential updates are immediately persisted to the secret.
func NewSecretBackedCredentials(
	ctx context.Context,
	client k8sclient.Client,
	namespace, name string,
) (*SecretBackedCredentials, error) {
	creds := &SecretBackedCredentials{
		secretNamespace: namespace,
		secretName:      name,
		client:          client,
		ctx:             ctx,
	}

	// Load existing credentials if secret exists
	if err := creds.Refresh(); err != nil && !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to load credentials: %w", err)
	}

	return creds, nil
}

// Refresh reloads credentials from the secret.
// Thread-safe: can be called concurrently.
func (c *SecretBackedCredentials) Refresh() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.loadFromSecret()
}

// loadFromSecret loads credentials from secret (caller must hold lock).
func (c *SecretBackedCredentials) loadFromSecret() error {
	secret := &corev1.Secret{}
	err := c.client.Get(c.ctx, c.secretNamespace, c.secretName, secret)

	if err != nil {
		return err
	}

	// Update in-memory cache
	if login, ok := secret.Data[primitives.SecretKeySystemLogin]; ok {
		c.systemLogin = string(login)
	}
	if password, ok := secret.Data[primitives.SecretKeyPassword]; ok {
		c.password = string(password)
	}
	if token, ok := secret.Data[primitives.SecretKeySystemToken]; ok {
		c.systemToken = string(token)
	}

	return nil
}

// saveToSecret persists credentials to secret (caller must hold lock).
func (c *SecretBackedCredentials) saveToSecret() error {
	secret := &corev1.Secret{}
	err := c.client.Get(c.ctx, c.secretNamespace, c.secretName, secret)

	create := false
	if apierrors.IsNotFound(err) {
		create = true
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      c.secretName,
				Namespace: c.secretNamespace,
				Labels: map[string]string{
					primitives.LabelSecretRole: string(primitives.SecretRoleSCCCredentials),
				},
			},
			Data: make(map[string][]byte),
		}
	} else if err != nil {
		return err
	}

	// Ensure data map exists
	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}

	// Update secret data from in-memory cache
	secret.Data[primitives.SecretKeySystemLogin] = []byte(c.systemLogin)
	secret.Data[primitives.SecretKeyPassword] = []byte(c.password)
	secret.Data[primitives.SecretKeySystemToken] = []byte(c.systemToken)

	// Create or update
	if create {
		return c.client.Create(c.ctx, secret)
	}
	return c.client.Update(c.ctx, secret)
}

// HasAuthentication implements connection.Credentials.
// Thread-safe: uses mutex for read consistency.
func (c *SecretBackedCredentials) HasAuthentication() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.systemLogin != "" && c.password != "" && c.systemToken != ""
}

// Token implements connection.Credentials.
// Thread-safe: uses mutex for read consistency.
func (c *SecretBackedCredentials) Token() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.systemToken, nil
}

// UpdateToken implements connection.Credentials.
//
// CRITICAL: This is called by connect-ng after EVERY non-read SCC API request.
// The token MUST be saved immediately to prevent losing state.
//
// Thread-safe: uses mutex to prevent concurrent updates from corrupting the secret.
func (c *SecretBackedCredentials) UpdateToken(newToken string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Skip empty token updates (common during deregistration)
	if newToken == "" && c.systemToken == "" {
		return nil
	}

	// Update in-memory cache
	c.systemToken = newToken

	// IMMEDIATELY save to secret
	// This ensures the token survives restarts and is available to other reconcilers
	return c.saveToSecret()
}

// Login implements connection.Credentials.
// Thread-safe: uses mutex for read consistency.
func (c *SecretBackedCredentials) Login() (string, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.systemLogin == "" || c.password == "" {
		return "", "", fmt.Errorf("credentials not configured")
	}

	return c.systemLogin, c.password, nil
}

// SetLogin implements connection.Credentials.
//
// Called after successful registration to store the system credentials returned by SCC.
//
// Thread-safe: uses mutex to prevent concurrent updates from corrupting the secret.
func (c *SecretBackedCredentials) SetLogin(newLogin, newPassword string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Update in-memory cache
	c.systemLogin = newLogin
	c.password = newPassword

	// IMMEDIATELY save to secret
	return c.saveToSecret()
}

// Ensure we implement the interface
var _ connection.Credentials = (*SecretBackedCredentials)(nil)
