package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/SUSE/connect-ng/k8s/api/primitives"
	"github.com/SUSE/connect-ng/k8s/contract"
	"github.com/SUSE/connect-ng/k8s/scc"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OfflineHandler implements RegistrationHandler for offline mode.
// Based on scc-operator's sccOfflineMode.
//
// In offline mode:
// - Registration: Generates XML request for user to upload
// - Activation: Validates user-uploaded certificate
// - Keepalive: No-op (offline systems don't phone home)
type OfflineHandler struct {
	ctx    context.Context
	obj    contract.ProductRegistrationObject
	config HandlerConfig
}

// NewOfflineHandler creates a new offline mode lifecycle.
// Handlers are created per-reconcile and are stateless.
func NewOfflineHandler(
	ctx context.Context,
	obj contract.ProductRegistrationObject,
	config HandlerConfig,
) *OfflineHandler {
	return &OfflineHandler{
		ctx:    ctx,
		obj:    obj,
		config: config,
	}
}

// Decision Methods

func (h *OfflineHandler) NeedsRegistration(ctx context.Context, obj contract.ProductRegistrationObject) bool {
	status := obj.GetStatus()

	// Never registered
	if status.GetRegistrationProcessedTS() == nil {
		return true
	}

	// Missing offline request condition
	if !status.IsConditionTrue(primitives.ConditionTypeOfflineRequestReady) {
		return true
	}

	return false
}

func (h *OfflineHandler) NeedsActivation(ctx context.Context, obj contract.ProductRegistrationObject) bool {
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

func (h *OfflineHandler) ReadyForActivation(ctx context.Context, obj contract.ProductRegistrationObject) bool {
	status := obj.GetStatus()
	spec := obj.GetSpec()

	// Offline request must be generated
	if !status.IsConditionTrue(primitives.ConditionTypeOfflineRequestReady) {
		return false
	}

	// User must have uploaded certificate
	certRef := spec.GetOfflineCertificateRef()
	if certRef == nil {
		return false
	}

	// Verify certificate secret exists
	secret := &corev1.Secret{}
	err := h.config.SecretClient.Get(ctx, certRef.Namespace, certRef.Name, secret)
	return err == nil
}

func (h *OfflineHandler) NeedsKeepalive(ctx context.Context, obj contract.ProductRegistrationObject) bool {
	// Offline systems don't need keepalive
	return false
}

// Preparation Methods

func (h *OfflineHandler) PrepareForRegister(ctx context.Context, obj contract.ProductRegistrationObject) (contract.ProductRegistrationObject, error) {
	// No preparation needed for offline registration
	return obj, nil
}

func (h *OfflineHandler) PrepareRegisteredForActivation(ctx context.Context, obj contract.ProductRegistrationObject) (contract.ProductRegistrationObject, error) {
	status := obj.GetStatus()

	// Set offline request ready condition
	status.SetCondition(primitives.ConditionTypeOfflineRequestReady, metav1.ConditionTrue, "Generated", "Offline registration request generated")

	return obj, nil
}

func (h *OfflineHandler) PrepareActivatedForKeepalive(ctx context.Context, obj contract.ProductRegistrationObject) (contract.ProductRegistrationObject, error) {
	status := obj.GetStatus()

	// Mark as activated
	status.SetActivated(true)

	// Set offline activation conditions
	status.SetCondition(primitives.ConditionTypeOfflineCertificateReady, metav1.ConditionTrue, "Validated", "Certificate validated")
	status.SetCondition(primitives.ConditionTypeOfflineActivationDone, metav1.ConditionTrue, "Complete", "Offline activation complete")

	// Set last validated timestamp (even though no keepalive)
	now := metav1.Now()
	status.SetLastValidatedTS(&now)

	return obj, nil
}

func (h *OfflineHandler) PrepareKeepaliveSucceeded(ctx context.Context, obj contract.ProductRegistrationObject) (contract.ProductRegistrationObject, error) {
	// No-op for offline mode
	return obj, nil
}

// Operation Methods

func (h *OfflineHandler) Register(ctx context.Context, obj contract.ProductRegistrationObject) (int, error) {
	spec := obj.GetSpec()

	// 1. Fetch system metrics from metrics secret
	// The metrics secret is externally populated by the product's telemetry system
	metricsData, err := h.fetchMetrics(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch metrics: %w", err)
	}

	// 2. Extract architecture from metrics (or use "unknown" if not present)
	arch := "unknown"
	if archValue, ok := metricsData[primitives.MetricsKeyArch]; ok {
		if archStr, ok := archValue.(string); ok {
			arch = archStr
		}
	}

	// 3. Determine SCC URL and create client
	sccURL := h.determineSCCURL(spec)
	sccClient := h.createSCCClient(ctx, sccURL)

	// 4. Generate offline registration request using SCC client
	requestData, err := sccClient.GenerateOfflineRequest(
		ctx,
		h.config.ProductIdentifier,
		h.config.ProductVersion,
		arch,
		metricsData,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to generate offline request: %w", err)
	}

	// 5. Store request in secret for user to download
	secretRef, err := h.createOfflineRequestSecret(ctx, obj, requestData)
	if err != nil {
		return 0, fmt.Errorf("failed to create offline request secret: %w", err)
	}

	status := obj.GetStatus()
	status.SetOfflineRegistrationRequestRef(secretRef)

	// 6. Return placeholder system ID (-1 for offline)
	placeholderID := -1
	status.SetSCCSystemID(&placeholderID)

	return placeholderID, nil
}

func (h *OfflineHandler) Activate(ctx context.Context, obj contract.ProductRegistrationObject) error {
	spec := obj.GetSpec()

	// 1. Fetch user-uploaded certificate
	certRef := spec.GetOfflineCertificateRef()
	if certRef == nil {
		return fmt.Errorf("offline certificate secret reference not set")
	}

	certData, err := h.fetchSecretData(ctx, certRef, primitives.SecretKeyCertificate)
	if err != nil {
		return fmt.Errorf("failed to fetch certificate: %w", err)
	}

	// 2. Determine SCC URL and create client
	sccURL := h.determineSCCURL(spec)
	sccClient := h.createSCCClient(ctx, sccURL)

	// 3. Validate certificate using SCC client
	err = sccClient.ValidateOfflineCertificate(ctx, certData)
	if err != nil {
		return fmt.Errorf("certificate validation failed: %w", err)
	}

	// 4. Certificate is valid - activation complete
	// TODO: Extract and store additional metadata from certificate if needed
	// (product name, expiration date, etc.)

	return nil
}

func (h *OfflineHandler) Keepalive(ctx context.Context, obj contract.ProductRegistrationObject) error {
	// No-op for offline mode
	return nil
}

func (h *OfflineHandler) Deregister(ctx context.Context, obj contract.ProductRegistrationObject) error {
	// Offline mode: just clean up local secrets
	status := obj.GetStatus()

	// Delete offline request secret if it exists
	requestRef := status.GetOfflineRegistrationRequestRef()
	if requestRef != nil {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      requestRef.Name,
				Namespace: requestRef.Namespace,
			},
		}
		_ = h.config.SecretClient.Delete(ctx, secret)
	}

	return nil
}

// Error Reconciliation Methods

func (h *OfflineHandler) ReconcileRegisterError(ctx context.Context, obj contract.ProductRegistrationObject, err error) contract.ProductRegistrationObject {
	status := obj.GetStatus()

	// Set failure condition
	status.SetCondition(primitives.ConditionTypeFailure, metav1.ConditionTrue, "OfflineRequestFailed", err.Error())
	status.SetCondition(primitives.ConditionTypeProgressing, metav1.ConditionFalse, "Failed", "Offline request generation failed")

	return obj
}

func (h *OfflineHandler) ReconcileActivateError(ctx context.Context, obj contract.ProductRegistrationObject, err error) contract.ProductRegistrationObject {
	status := obj.GetStatus()

	// Set failure condition
	status.SetCondition(primitives.ConditionTypeFailure, metav1.ConditionTrue, "CertificateValidationFailed", err.Error())
	status.SetCondition(primitives.ConditionTypeProgressing, metav1.ConditionFalse, "Failed", "Certificate validation failed")

	return obj
}

func (h *OfflineHandler) ReconcileKeepaliveError(ctx context.Context, obj contract.ProductRegistrationObject, err error) contract.ProductRegistrationObject {
	// No-op for offline mode (keepalive never called)
	return obj
}

// Preprocessing Methods

// NeedsPreprocessRegistration checks if the registration needs preprocessing.
// For offline mode, this happens when:
// 1. User removed offline certificate after activation (to retry with new cert)
// 2. User removed failed certificate (to fix and retry)
// Based on SCC Operator's offline lifecycle.
func (h *OfflineHandler) NeedsPreprocessRegistration(ctx context.Context, obj contract.ProductRegistrationObject) bool {
	spec := obj.GetSpec()
	status := obj.GetStatus()

	// Check if user removed cert after activation (wants to retry)
	activatedButMissingCert := status.GetActivated() && spec.GetOfflineCertificateRef() == nil

	// Check if user removed failed cert (wants to fix and retry)
	// Failure condition is true AND OfflineCertificateReady is false AND cert ref is nil
	failedCertRemoved := false
	if status.IsConditionTrue(primitives.ConditionTypeFailure) {
		// Check if OfflineCertificateReady condition exists and is false, and cert was removed
		if status.HasCondition(primitives.ConditionTypeOfflineCertificateReady) &&
			!status.IsConditionTrue(primitives.ConditionTypeOfflineCertificateReady) &&
			spec.GetOfflineCertificateRef() == nil {
			failedCertRemoved = true
		}
	}

	return activatedButMissingCert || failedCertRemoved
}

// PreprocessRegistration performs preprocessing on the registration.
// For offline mode, this resets the registration state to allow re-activation.
// Based on SCC Operator's offline lifecycle.
func (h *OfflineHandler) PreprocessRegistration(ctx context.Context, obj contract.ProductRegistrationObject) (contract.ProductRegistrationObject, error) {
	return h.ResetToReadyForActivation(ctx, obj)
}

// ResetToReadyForActivation resets the registration state to allow re-activation.
// Used when user removes offline certificate to retry, or when syncNow is triggered.
// Based on SCC Operator's offline lifecycle.
func (h *OfflineHandler) ResetToReadyForActivation(ctx context.Context, obj contract.ProductRegistrationObject) (contract.ProductRegistrationObject, error) {
	status := obj.GetStatus()

	// Clear activation status
	status.SetActivated(false)
	now := metav1.Now()
	status.SetLastValidatedTS(&now) // Reset to zero time

	// Remove existing conditions
	status.RemoveCondition(primitives.ConditionTypeActivated)
	status.RemoveCondition(primitives.ConditionTypeOfflineCertificateReady)
	status.RemoveCondition(primitives.ConditionTypeFailure)
	status.RemoveCondition(primitives.ConditionTypeReady)

	// Set Progressing condition
	status.SetCondition(primitives.ConditionTypeProgressing, metav1.ConditionTrue, "Resetting", "Resetting registration for re-activation")

	// Call PrepareRegisteredForActivation to set up proper state
	return h.PrepareRegisteredForActivation(ctx, obj)
}

// Helper Methods

// createOfflineRequestSecret creates a secret containing the offline registration request XML.
func (h *OfflineHandler) createOfflineRequestSecret(ctx context.Context, obj contract.ProductRegistrationObject, requestXML []byte) (*corev1.SecretReference, error) {
	namespace := obj.GetNamespace()
	if namespace == "" {
		namespace = "default" // TODO: Make configurable
	}

	secretName := primitives.OfflineRequestSecretName(obj.GetName())

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Labels: map[string]string{
				primitives.LabelSecretRole:       string(primitives.SecretRoleOfflineRequest),
				primitives.LabelRegistrationType: primitives.LabelRegistrationTypeValue,
				primitives.LabelCredentialsType:  h.config.ProductIdentifier,
			},
			Finalizers: []string{
				primitives.FinalizerOfflineRequest,
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			primitives.SecretKeyRequestXML: requestXML,
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
func (h *OfflineHandler) fetchSecretData(ctx context.Context, ref *corev1.SecretReference, key string) ([]byte, error) {
	if ref == nil {
		return nil, fmt.Errorf("secret reference is nil")
	}

	secret := &corev1.Secret{}

	if err := h.config.SecretClient.Get(ctx, ref.Namespace, ref.Name, secret); err != nil {
		return nil, fmt.Errorf("failed to get secret %s/%s: %w", ref.Namespace, ref.Name, err)
	}

	data, ok := secret.Data[key]
	if !ok {
		return nil, fmt.Errorf("key %s not found in secret %s/%s", key, ref.Namespace, ref.Name)
	}

	return data, nil
}

// fetchMetrics reads the metrics secret and returns the payload as map[string]any.
//
// Based on scc-operator's metrics secret pattern:
// - Secret is externally populated by the product's telemetry system
// - Contains a "payload" key with JSON-encoded system information
// - Data is passed directly to SCC (registration.SystemInformation)
//
// See handler_online.go for detailed documentation of the metrics secret format.
func (h *OfflineHandler) fetchMetrics(ctx context.Context) (map[string]any, error) {
	if h.config.MetricsSecretName == "" {
		return nil, fmt.Errorf("metrics secret name not configured")
	}
	if h.config.MetricsSecretNamespace == "" {
		return nil, fmt.Errorf("metrics secret namespace not configured")
	}

	// Fetch the metrics secret
	secret := &corev1.Secret{}

	if err := h.config.SecretClient.Get(ctx, h.config.MetricsSecretNamespace, h.config.MetricsSecretName, secret); err != nil {
		return nil, fmt.Errorf("failed to get metrics secret %s/%s: %w",
			h.config.MetricsSecretNamespace, h.config.MetricsSecretName, err)
	}

	// Extract the payload
	payloadBytes, ok := secret.Data[primitives.SecretKeyPayload]
	if !ok {
		return nil, fmt.Errorf("metrics secret %s/%s missing %q key",
			h.config.MetricsSecretNamespace, h.config.MetricsSecretName, primitives.SecretKeyPayload)
	}

	// Parse JSON payload
	var metricsData map[string]any
	if err := json.Unmarshal(payloadBytes, &metricsData); err != nil {
		return nil, fmt.Errorf("failed to parse metrics payload: %w", err)
	}

	return metricsData, nil
}

// determineSCCURL determines the SCC URL following scc-operator's precedence.
// Same logic as OnlineHandler.
func (h *OfflineHandler) determineSCCURL(spec contract.ProductRegistrationSpec) string {
	// 1. Check CR spec for custom URL (highest priority)
	if sccURL := spec.GetRegistrationURL(); sccURL != nil && *sccURL != "" {
		return *sccURL
	}

	// 2. Check PRIME_SCC_REGISTRATION_HOST_URL env var (global override)
	if primeURL := os.Getenv(primitives.EnvPrimeSCCRegistrationHostURL); primeURL != "" {
		return primeURL
	}

	// 3. Check DEV_MODE for staging SCC
	if devMode := os.Getenv(primitives.EnvDevMode); devMode == "true" || devMode == "1" {
		return "https://stgscc.suse.com"
	}

	// 4. Default to production SCC
	return "https://scc.suse.com"
}

// createSCCClient creates an SCC client with the determined URL.
func (h *OfflineHandler) createSCCClient(ctx context.Context, sccURL string) *scc.DefaultClient {
	return &scc.DefaultClient{
		BaseURL:               sccURL,
		K8sClient:             h.config.SecretClient,
		CredentialsNamespace:  h.config.CredentialsNamespace,
		CredentialsSecretName: h.config.CredentialsSecretName,
	}
}
