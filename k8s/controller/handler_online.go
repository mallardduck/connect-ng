package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/SUSE/connect-ng/k8s/consts"
	"github.com/SUSE/connect-ng/k8s/scclient"
	"github.com/SUSE/connect-ng/k8s/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// OnlineHandler implements RegistrationHandler for online mode.
// Based on scc-operator's sccOnlineMode.
//
// In online mode:
// - Registration: Calls SCC API to announce system
// - Activation: Calls SCC API to activate products
// - Keepalive: Periodic heartbeat to SCC (every ~20 hours)
type OnlineHandler struct {
	ctx    context.Context
	obj    types.ProductRegistrationObject
	config HandlerConfig
}

// NewOnlineHandler creates a new online mode handler.
// Handlers are created per-reconcile and are stateless.
func NewOnlineHandler(
	ctx context.Context,
	obj types.ProductRegistrationObject,
	config HandlerConfig,
) *OnlineHandler {
	return &OnlineHandler{
		ctx:    ctx,
		obj:    obj,
		config: config,
	}
}

// Decision Methods

func (h *OnlineHandler) NeedsRegistration(ctx context.Context, obj types.ProductRegistrationObject) bool {
	status := obj.GetStatus()

	// Never registered
	if status.GetRegistrationProcessedTS() == nil {
		return true
	}

	// Missing required conditions
	if !status.IsConditionTrue("RegistrationAnnounced") {
		return true
	}
	if !status.IsConditionTrue("RegistrationSccUrlReady") {
		return true
	}

	return false
}

func (h *OnlineHandler) NeedsActivation(ctx context.Context, obj types.ProductRegistrationObject) bool {
	status := obj.GetStatus()

	// Never registered
	if status.GetRegistrationProcessedTS() == nil {
		return true
	}

	// Not activated
	if !status.GetActivated() {
		return true
	}

	return false
}

func (h *OnlineHandler) ReadyForActivation(ctx context.Context, obj types.ProductRegistrationObject) bool {
	status := obj.GetStatus()

	// Registration must have succeeded
	return status.IsConditionTrue("RegistrationAnnounced") &&
		status.IsConditionTrue("RegistrationSccUrlReady")
}

func (h *OnlineHandler) NeedsKeepalive(ctx context.Context, obj types.ProductRegistrationObject) bool {
	status := obj.GetStatus()

	// Not activated yet
	if !status.GetActivated() {
		return false
	}

	// Check if enough time has passed
	lastValidated := status.GetLastValidatedTS()
	if lastValidated == nil {
		return true
	}

	// Keepalive every 20 hours, but check a bit earlier (17 hours)
	nextKeepalive := lastValidated.Add(17 * time.Hour)
	return time.Now().After(nextKeepalive)
}

// Preparation Methods

func (h *OnlineHandler) PrepareForRegister(ctx context.Context, obj types.ProductRegistrationObject) (types.ProductRegistrationObject, error) {
	status := obj.GetStatus()

	// Create system credentials secret if it doesn't exist
	if status.GetSystemCredentialsSecretRef() == nil {
		// Create an empty credentials secret (will be populated after registration)
		secretRef, err := h.createCredentialsSecret(ctx, obj)
		if err != nil {
			return obj, fmt.Errorf("failed to create credentials secret: %w", err)
		}
		status.SetSystemCredentialsSecretRef(secretRef)
	}

	return obj, nil
}

func (h *OnlineHandler) PrepareRegisteredForActivation(ctx context.Context, obj types.ProductRegistrationObject) (types.ProductRegistrationObject, error) {
	status := obj.GetStatus()

	// Set conditions
	status.SetCondition("RegistrationAnnounced", metav1.ConditionTrue, "Announced", "System announced to SCC")
	status.SetCondition("RegistrationSccUrlReady", metav1.ConditionTrue, "Ready", "SCC URL ready")

	return obj, nil
}

func (h *OnlineHandler) PrepareActivatedForKeepalive(ctx context.Context, obj types.ProductRegistrationObject) (types.ProductRegistrationObject, error) {
	status := obj.GetStatus()

	// Mark as activated
	status.SetActivated(true)

	// Set activation condition
	status.SetCondition("RegistrationActivated", metav1.ConditionTrue, "Activated", "Products activated with SCC")

	// Set last validated timestamp
	now := metav1.Now()
	status.SetLastValidatedTS(&now)

	return obj, nil
}

func (h *OnlineHandler) PrepareKeepaliveSucceeded(ctx context.Context, obj types.ProductRegistrationObject) (types.ProductRegistrationObject, error) {
	status := obj.GetStatus()

	// Update keepalive condition
	status.SetCondition("RegistrationKeepalive", metav1.ConditionTrue, "Success", "Keepalive successful")

	return obj, nil
}

// Operation Methods

func (h *OnlineHandler) Register(ctx context.Context, obj types.ProductRegistrationObject) (int, error) {
	spec := obj.GetSpec()

	// 1. Fetch registration code from secret
	regCodeRef := spec.GetRegistrationCodeRef()
	if regCodeRef == nil {
		return 0, fmt.Errorf("registration code secret reference is required for online mode")
	}

	regCode, err := h.fetchSecretData(ctx, regCodeRef, consts.SecretKeyRegistrationCode)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch registration code: %w", err)
	}

	// 2. Fetch system metrics from metrics secret
	// The metrics secret is externally populated by the product's telemetry system
	metricsData, err := h.fetchMetrics(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch metrics: %w", err)
	}

	// 3. Extract hostname from metrics (or use os.Hostname() as fallback)
	hostname, err := h.extractHostname(metricsData)
	if err != nil {
		return 0, fmt.Errorf("failed to determine hostname: %w", err)
	}

	// 4. Determine SCC URL and create client
	sccURL := h.determineSCCURL(spec)
	sccClient := h.createSCCClient(ctx, sccURL)

	// 5. Call SCC API to register
	systemID, login, password, err := sccClient.Register(
		ctx,
		regCode,
		hostname,
		metricsData,
		nil, // extraData
	)
	if err != nil {
		return 0, fmt.Errorf("SCC registration failed: %w", err)
	}

	// 6. Save system ID to status
	status := obj.GetStatus()
	status.SetSCCSystemID(&systemID)

	// 7. Save credentials to secret
	credRef := status.GetSystemCredentialsSecretRef()
	if credRef != nil {
		if err := h.updateCredentialsSecret(ctx, credRef, login, password); err != nil {
			return 0, fmt.Errorf("failed to save credentials: %w", err)
		}
	}

	return systemID, nil
}

func (h *OnlineHandler) Activate(ctx context.Context, obj types.ProductRegistrationObject) error {
	spec := obj.GetSpec()
	status := obj.GetStatus()

	// 1. Fetch system credentials
	credRef := status.GetSystemCredentialsSecretRef()
	if credRef == nil {
		return fmt.Errorf("system credentials secret reference not found")
	}

	login, password, err := h.fetchCredentials(ctx, credRef)
	if err != nil {
		return fmt.Errorf("failed to fetch credentials: %w", err)
	}

	// 2. Fetch registration code (needed for activation)
	regCodeRef := spec.GetRegistrationCodeRef()
	if regCodeRef == nil {
		return fmt.Errorf("registration code secret reference required for activation")
	}

	regCode, err := h.fetchSecretData(ctx, regCodeRef, consts.SecretKeyRegistrationCode)
	if err != nil {
		return fmt.Errorf("failed to fetch registration code: %w", err)
	}

	// 3. Fetch metrics to extract architecture
	metricsData, err := h.fetchMetrics(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch metrics: %w", err)
	}

	// Extract architecture from metrics (or use "unknown" if not present)
	arch := "unknown"
	if archValue, ok := metricsData[consts.MetricsKeyArch]; ok {
		if archStr, ok := archValue.(string); ok {
			arch = archStr
		}
	}

	// 4. Determine SCC URL and create client
	sccURL := h.determineSCCURL(spec)
	sccClient := h.createSCCClient(ctx, sccURL)

	// 5. Call SCC API to activate products
	err = sccClient.Activate(
		ctx,
		h.config.ProductIdentifier,
		h.config.ProductVersion,
		arch,
		regCode,
		login,
		password,
	)
	if err != nil {
		return fmt.Errorf("SCC activation failed: %w", err)
	}

	// 6. Save system URL
	systemID := status.GetSCCSystemID()
	if systemID != nil {
		systemURL := fmt.Sprintf("https://scc.suse.com/systems/%d", *systemID)
		status.SetSystemURL(&systemURL)
	}

	return nil
}

func (h *OnlineHandler) Keepalive(ctx context.Context, obj types.ProductRegistrationObject) error {
	spec := obj.GetSpec()
	status := obj.GetStatus()

	// 1. Fetch system credentials
	credRef := status.GetSystemCredentialsSecretRef()
	if credRef == nil {
		return fmt.Errorf("system credentials secret reference not found")
	}

	login, password, err := h.fetchCredentials(ctx, credRef)
	if err != nil {
		return fmt.Errorf("failed to fetch credentials: %w", err)
	}

	// 2. Fetch system metrics from metrics secret
	// The metrics secret is externally populated by the product's telemetry system
	metricsData, err := h.fetchMetrics(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch metrics: %w", err)
	}

	// 3. Extract hostname from metrics (or use os.Hostname() as fallback)
	hostname, err := h.extractHostname(metricsData)
	if err != nil {
		return fmt.Errorf("failed to determine hostname: %w", err)
	}

	// 4. Determine SCC URL and create client
	sccURL := h.determineSCCURL(spec)
	sccClient := h.createSCCClient(ctx, sccURL)

	// 5. Call SCC API to perform keepalive
	err = sccClient.Keepalive(
		ctx,
		hostname,
		metricsData,
		login,
		password,
	)
	if err != nil {
		return fmt.Errorf("SCC keepalive failed: %w", err)
	}

	// 6. Update last validated timestamp
	now := metav1.Now()
	status.SetLastValidatedTS(&now)

	return nil
}

func (h *OnlineHandler) Deregister(ctx context.Context, obj types.ProductRegistrationObject) error {
	spec := obj.GetSpec()
	status := obj.GetStatus()

	// 1. Fetch system credentials
	credRef := status.GetSystemCredentialsSecretRef()
	if credRef == nil {
		// Nothing to deregister
		return nil
	}

	login, password, err := h.fetchCredentials(ctx, credRef)
	if err != nil {
		// If we can't fetch credentials, we can't deregister
		// but we should still clean up local state
		return nil
	}

	// 2. Determine SCC URL and create client
	sccURL := h.determineSCCURL(spec)
	sccClient := h.createSCCClient(ctx, sccURL)

	// 3. Call SCC API to deregister
	err = sccClient.Deregister(ctx, login, password)
	if err != nil {
		return fmt.Errorf("SCC deregister failed: %w", err)
	}

	// 4. Clear activation state
	status.SetActivated(false)
	status.SetSCCSystemID(nil)
	status.SetSystemURL(nil)

	return nil
}

// Error Reconciliation Methods

func (h *OnlineHandler) ReconcileRegisterError(ctx context.Context, obj types.ProductRegistrationObject, err error) types.ProductRegistrationObject {
	status := obj.GetStatus()

	// Set failure condition
	status.SetCondition("Failure", metav1.ConditionTrue, "RegistrationFailed", err.Error())
	status.SetCondition("Progressing", metav1.ConditionFalse, "Failed", "Registration failed")

	return obj
}

func (h *OnlineHandler) ReconcileActivateError(ctx context.Context, obj types.ProductRegistrationObject, err error) types.ProductRegistrationObject {
	status := obj.GetStatus()

	// Set failure condition
	status.SetCondition("Failure", metav1.ConditionTrue, "ActivationFailed", err.Error())
	status.SetCondition("Progressing", metav1.ConditionFalse, "Failed", "Activation failed")

	return obj
}

func (h *OnlineHandler) ReconcileKeepaliveError(ctx context.Context, obj types.ProductRegistrationObject, err error) types.ProductRegistrationObject {
	status := obj.GetStatus()

	// Set failure condition (but don't invalidate activation)
	status.SetCondition("Failure", metav1.ConditionTrue, "KeepaliveFailed", err.Error())
	status.SetCondition("RegistrationKeepalive", metav1.ConditionFalse, "Failed", "Keepalive failed")

	// TODO: On persistent failure, may need to invalidate activation

	return obj
}

// Preprocessing Methods

// NeedsPreprocessRegistration checks if the registration needs preprocessing.
// For online mode, preprocessing is not currently needed.
// Based on SCC Operator's online handler (always returns false).
func (h *OnlineHandler) NeedsPreprocessRegistration(ctx context.Context, obj types.ProductRegistrationObject) bool {
	// TODO: online implementation of NeedsPreprocessRegistration if needed
	return false
}

// PreprocessRegistration performs preprocessing on the registration.
// For online mode, this is currently a no-op.
// Based on SCC Operator's online handler.
func (h *OnlineHandler) PreprocessRegistration(ctx context.Context, obj types.ProductRegistrationObject) (types.ProductRegistrationObject, error) {
	// TODO: online implementation of PreprocessRegistration if needed
	return obj, nil
}

// ResetToReadyForActivation resets the registration state to allow re-activation.
// Used when syncNow is triggered on a failed activation.
// Based on SCC Operator's online handler.
func (h *OnlineHandler) ResetToReadyForActivation(ctx context.Context, obj types.ProductRegistrationObject) (types.ProductRegistrationObject, error) {
	status := obj.GetStatus()

	// Clear activation status
	status.SetActivated(false)
	now := metav1.Now()
	status.SetLastValidatedTS(&now) // Set to zero time

	// Set conditions
	status.SetCondition("Progressing", metav1.ConditionTrue, "Resetting", "Resetting registration for re-activation")
	status.SetCondition("Ready", metav1.ConditionFalse, "NotReady", "Registration reset")
	status.SetCondition("Activated", metav1.ConditionFalse, "NotActivated", "Activation cleared")

	// Clear failure condition if present
	status.RemoveCondition("Failure")

	return obj, nil
}

// Helper Methods

// createCredentialsSecret creates an empty credentials secret.
func (h *OnlineHandler) createCredentialsSecret(ctx context.Context, obj types.ProductRegistrationObject) (*corev1.SecretReference, error) {
	// Create secret in the same namespace as the CRD
	namespace := obj.GetNamespace()
	if namespace == "" {
		// Cluster-scoped CRD - use a default namespace
		namespace = "default" // TODO: Make configurable
	}

	secretName := consts.SCCCredentialsSecretName(obj.GetName())

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Labels: map[string]string{
				consts.LabelSecretRole: string(consts.SecretRoleSCCCredentials),
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			// Will be populated after registration
		},
	}

	if err := h.config.SecretClient.Create(ctx, secret); err != nil {
		return nil, err
	}

	return &corev1.SecretReference{
		Name:      secretName,
		Namespace: namespace,
	}, nil
}

// fetchSecretData fetches a specific key from a secret.
func (h *OnlineHandler) fetchSecretData(ctx context.Context, ref *corev1.SecretReference, key string) (string, error) {
	if ref == nil {
		return "", fmt.Errorf("secret reference is nil")
	}

	secret := &corev1.Secret{}
	secretKey := client.ObjectKey{
		Name:      ref.Name,
		Namespace: ref.Namespace,
	}

	if err := h.config.SecretClient.Get(ctx, secretKey, secret); err != nil {
		return "", fmt.Errorf("failed to get secret %s/%s: %w", ref.Namespace, ref.Name, err)
	}

	data, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("key %s not found in secret %s/%s", key, ref.Namespace, ref.Name)
	}

	return string(data), nil
}

// fetchCredentials fetches login and password from credentials secret.
func (h *OnlineHandler) fetchCredentials(ctx context.Context, ref *corev1.SecretReference) (login, password string, err error) {
	login, err = h.fetchSecretData(ctx, ref, consts.SecretKeyLogin)
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch login: %w", err)
	}

	password, err = h.fetchSecretData(ctx, ref, consts.SecretKeyPassword)
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch password: %w", err)
	}

	return login, password, nil
}

// updateCredentialsSecret updates the credentials secret with login/password.
func (h *OnlineHandler) updateCredentialsSecret(ctx context.Context, ref *corev1.SecretReference, login, password string) error {
	secret := &corev1.Secret{}
	secretKey := client.ObjectKey{
		Name:      ref.Name,
		Namespace: ref.Namespace,
	}

	if err := h.config.SecretClient.Get(ctx, secretKey, secret); err != nil {
		return fmt.Errorf("failed to get credentials secret: %w", err)
	}

	// Update secret data
	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}
	secret.Data[consts.SecretKeyLogin] = []byte(login)
	secret.Data[consts.SecretKeyPassword] = []byte(password)

	if err := h.config.SecretClient.Update(ctx, secret); err != nil {
		return fmt.Errorf("failed to update credentials secret: %w", err)
	}

	return nil
}

// fetchMetrics reads the metrics secret and returns the payload as map[string]any.
//
// Based on scc-operator's metrics secret pattern:
// - Secret is externally populated by the product's telemetry system
// - Contains a "payload" key with JSON-encoded system information
// - Data is passed directly to SCC (registration.SystemInformation)
//
// Example metrics secret (rancher):
//
//	apiVersion: v1
//	kind: Secret
//	metadata:
//	  name: rancher-scc-metrics
//	data:
//	  payload: eyJ2ZXJzaW9uIjogIjIuMTAuMCIsICJzdWJzY3JpcHRpb24iOiB7Li4ufX0=
//
// The payload structure is product-specific. For Rancher:
//
//	{
//	  "version": "2.10.0",
//	  "hostname": "rancher-cluster-01",
//	  "subscription": {
//	    "installuuid": "...",
//	    "product": "rancher",
//	    "version": "2.10.0",
//	    "arch": "x86_64",
//	    "git": "..."
//	  }
//	}
func (h *OnlineHandler) fetchMetrics(ctx context.Context) (map[string]any, error) {
	if h.config.MetricsSecretName == "" {
		return nil, fmt.Errorf("metrics secret name not configured")
	}
	if h.config.MetricsSecretNamespace == "" {
		return nil, fmt.Errorf("metrics secret namespace not configured")
	}

	// Fetch the metrics secret
	secret := &corev1.Secret{}
	secretKey := client.ObjectKey{
		Name:      h.config.MetricsSecretName,
		Namespace: h.config.MetricsSecretNamespace,
	}

	if err := h.config.SecretClient.Get(ctx, secretKey, secret); err != nil {
		return nil, fmt.Errorf("failed to get metrics secret %s/%s: %w",
			h.config.MetricsSecretNamespace, h.config.MetricsSecretName, err)
	}

	// Extract the payload
	payloadBytes, ok := secret.Data[consts.SecretKeyPayload]
	if !ok {
		return nil, fmt.Errorf("metrics secret %s/%s missing %q key",
			h.config.MetricsSecretNamespace, h.config.MetricsSecretName, consts.SecretKeyPayload)
	}

	// Parse JSON payload
	var metricsData map[string]any
	if err := json.Unmarshal(payloadBytes, &metricsData); err != nil {
		return nil, fmt.Errorf("failed to parse metrics payload: %w", err)
	}

	return metricsData, nil
}

// extractHostname extracts hostname from metrics data, falling back to os.Hostname().
//
// Hostname is runtime information that should come from the product's telemetry system.
// If not present in metrics, we fall back to the container's hostname as a last resort.
func (h *OnlineHandler) extractHostname(metricsData map[string]any) (string, error) {
	// Try to extract from metrics first
	if hostnameValue, ok := metricsData[consts.MetricsKeyHostname]; ok {
		if hostname, ok := hostnameValue.(string); ok && hostname != "" {
			return hostname, nil
		}
	}

	// Fallback to os.Hostname()
	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("hostname not in metrics and os.Hostname() failed: %w", err)
	}

	return hostname, nil
}

// determineSCCURL determines the SCC URL following scc-operator's precedence:
// 1. ProductRegistration CR spec field (per-registration custom URL)
// 2. PRIME_SCC_REGISTRATION_HOST_URL environment variable (global override)
// 3. DEV_MODE environment variable (uses staging SCC)
// 4. Default to production SCC (https://scc.suse.com)
//
// Based on scc-operator's getCurrentRegURL() pattern.
func (h *OnlineHandler) determineSCCURL(spec types.ProductRegistrationSpec) string {
	// 1. Check CR spec for custom URL (highest priority)
	if sccURL := spec.GetRegistrationURL(); sccURL != nil && *sccURL != "" {
		return *sccURL
	}

	// 2. Check PRIME_SCC_REGISTRATION_HOST_URL env var (global override)
	if primeURL := os.Getenv(consts.EnvPrimeSCCRegistrationHostURL); primeURL != "" {
		return primeURL
	}

	// 3. Check DEV_MODE for staging SCC
	if devMode := os.Getenv(consts.EnvDevMode); devMode == "true" || devMode == "1" {
		return "https://stgscc.suse.com"
	}

	// 4. Default to production SCC
	return "https://scc.suse.com"
}

// createSCCClient creates an SCC client with the determined URL.
// This is called per-reconcile to respect URL precedence.
func (h *OnlineHandler) createSCCClient(ctx context.Context, sccURL string) *scclient.DefaultClient {
	return &scclient.DefaultClient{
		BaseURL:               sccURL,
		K8sClient:             h.config.SecretClient,
		CredentialsNamespace:  h.config.CredentialsNamespace,
		CredentialsSecretName: h.config.CredentialsSecretName,
	}
}
