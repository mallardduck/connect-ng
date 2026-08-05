// Package v1 contains common API types for SUSE product registration.
//
// These types are imported and reused by all products (Rancher, NeuVector, etc.)
// to ensure consistency across implementations. Only the outer wrapper type
// and API group are product-specific.
//
// The types in this package implement the interfaces defined in github.com/SUSE/connect-ng/k8s/types
// so that library controllers can work with any product's CRD through a common interface.
package v1

import (
	"github.com/SUSE/connect-ng/k8s/api/primitives"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RegistrationRequest contains references to secrets with registration data.
// Based on scc-operator's RegistrationRequest.
type RegistrationRequest struct {
	// RegistrationCodeSecretRef points to the Secret containing the SCC registration code.
	// The secret should have a key "registrationCode" with the code value.
	// May be nil for BAYG/RMT scenarios or offline mode.
	// +optional
	RegistrationCodeSecretRef *corev1.SecretReference `json:"registrationCodeSecretRef,omitempty"`

	// RegistrationAPIUrl is a custom SCC URL (e.g., for RMT proxy).
	// If nil, uses default SCC.
	// +optional
	RegistrationAPIUrl *string `json:"registrationAPIUrl,omitempty"`

	// RegistrationAPICertificateSecretRef points to a Secret containing a custom CA certificate.
	// Used when RegistrationAPIUrl points to a server with custom TLS certificate.
	// The secret should have a key "ca.crt" with the PEM-encoded certificate.
	// +optional
	RegistrationAPICertificateSecretRef *corev1.SecretReference `json:"registrationAPICertificateSecretRef,omitempty"`
}

// ProductRegistrationSpec defines the desired state of ProductRegistration.
// This spec is common across all products using the registration library.
// Based on scc-operator's RegistrationSpec.
type ProductRegistrationSpec struct {
	// Mode determines the registration workflow (online or offline).
	// +kubebuilder:default="online"
	// +kubebuilder:validation:Enum=online;offline
	// +optional
	Mode primitives.RegistrationMode `json:"mode,omitempty"`

	// RegistrationRequest contains references to secrets with registration data.
	// +optional
	RegistrationRequest *RegistrationRequest `json:"registrationRequest,omitempty"`

	// OfflineRegistrationCertificateSecretRef points to the Secret containing
	// the certificate uploaded by the user in offline mode.
	// Only relevant when Mode is "offline".
	// +optional
	OfflineRegistrationCertificateSecretRef *corev1.SecretReference `json:"offlineRegistrationCertificateSecretRef,omitempty"`

	// SyncNow triggers an immediate sync when set to true.
	// The controller will reset this to false after processing.
	// +optional
	SyncNow *bool `json:"syncNow,omitempty"`
}

// Interface implementation for types.ProductRegistrationSpec

func (s *ProductRegistrationSpec) GetMode() primitives.RegistrationMode {
	if s.Mode == "" {
		return primitives.RegistrationModeOnline // default
	}
	return s.Mode
}

func (s *ProductRegistrationSpec) GetRegistrationCodeRef() *corev1.SecretReference {
	if s.RegistrationRequest == nil {
		return nil
	}
	return s.RegistrationRequest.RegistrationCodeSecretRef
}

func (s *ProductRegistrationSpec) GetRegistrationURL() *string {
	if s.RegistrationRequest == nil {
		return nil
	}
	return s.RegistrationRequest.RegistrationAPIUrl
}

func (s *ProductRegistrationSpec) GetRegistrationURLCertRef() *corev1.SecretReference {
	if s.RegistrationRequest == nil {
		return nil
	}
	return s.RegistrationRequest.RegistrationAPICertificateSecretRef
}

func (s *ProductRegistrationSpec) GetOfflineCertificateRef() *corev1.SecretReference {
	return s.OfflineRegistrationCertificateSecretRef
}

func (s *ProductRegistrationSpec) GetSyncNow() bool {
	return s.SyncNow != nil && *s.SyncNow
}

func (s *ProductRegistrationSpec) SetSyncNow(syncNow bool) {
	s.SyncNow = &syncNow
}

// SystemActivationState represents the activation state of the system.
// Based on scc-operator's SystemActivationState.
type SystemActivationState struct {
	// Activated indicates if the system is activated with SCC.
	// +kubebuilder:default=false
	Activated bool `json:"activated"`

	// LastValidatedTS is when the system was last validated (keepalive).
	// +optional
	LastValidatedTS *metav1.Time `json:"lastValidatedTS,omitempty"`

	// SystemURL is the SCC URL for this system (for UI links).
	// Example: https://scc.suse.com/systems/12345
	// +optional
	SystemURL *string `json:"systemURL,omitempty"`
}

// SubscriptionMetadata contains information about the subscription from SCC.
// Maps to the SubscriptionInfo returned by /connect/subscriptions/info.
type SubscriptionMetadata struct {
	// Kind is the subscription type (e.g., "full", "trial")
	Kind string `json:"kind"`

	// Name is the subscription name
	Name string `json:"name"`

	// StartsAt is when the subscription becomes active
	// +optional
	StartsAt *metav1.Time `json:"startsAt,omitempty"`

	// ExpiresAt is when the subscription expires
	// +optional
	ExpiresAt *metav1.Time `json:"expiresAt,omitempty"`

	// ProductClasses lists the product classes covered by this subscription
	ProductClasses []ProductClassInfo `json:"productClasses,omitempty"`
}

// ProductClassInfo describes a product class available under the subscription.
type ProductClassInfo struct {
	// Name is the product class identifier (e.g., "RANCHER-X86", "SLES-X86")
	Name string `json:"name"`

	// Description is a human-readable description
	// +optional
	Description string `json:"description,omitempty"`
}

// ProductRegistrationStatus defines the observed state of ProductRegistration.
// This status is common across all products using the registration library.
// Based on scc-operator's RegistrationStatus.
type ProductRegistrationStatus struct {
	// Conditions represent the latest available observations of the registration state.
	// Standard conditions: Ready, Done, Progressing, Failure
	// Registration conditions: RegistrationAnnounced, RegistrationSccUrlReady, RegistrationActivated, RegistrationKeepalive
	// Offline conditions: OfflineRequestReady, OfflineCertificateReady, OfflineActivationDone
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// RegistrationProcessedTS is when the registration was last processed.
	// +optional
	RegistrationProcessedTS *metav1.Time `json:"registrationProcessedTS,omitempty"`

	// SCCSystemID is the ID assigned by SCC after registration.
	// Nil if not yet registered. -1 for offline mode.
	// +optional
	SCCSystemID *int `json:"sccSystemID,omitempty"`

	// RegisteredProduct is the product name from SCC.
	// +optional
	RegisteredProduct *string `json:"registeredProduct,omitempty"`

	// RegistrationExpiresAt is when the registration expires.
	// +optional
	RegistrationExpiresAt *metav1.Time `json:"registrationExpiresAt,omitempty"`

	// ActivationStatus contains activation-related state.
	// +optional
	ActivationStatus SystemActivationState `json:"activationStatus,omitempty"`

	// SystemCredentialsSecretRef points to the Secret containing SCC system credentials.
	// Created by the controller after successful registration.
	// Contains login/password for SCC API access.
	// +optional
	SystemCredentialsSecretRef *corev1.SecretReference `json:"systemCredentialsSecretRef,omitempty"`

	// OfflineRegistrationRequest points to the Secret containing the offline registration request XML.
	// Only populated in offline mode.
	// +optional
	OfflineRegistrationRequest *corev1.SecretReference `json:"offlineRegistrationRequest,omitempty"`

	// SubscriptionInfo contains metadata about the subscription from SCC.
	// Populated by querying /connect/subscriptions/info during registration.
	// Updated when syncNow is triggered.
	// +optional
	SubscriptionInfo *SubscriptionMetadata `json:"subscriptionInfo,omitempty"`
}

// Interface implementation for contract.ProductRegistrationStatus

// Condition Management

func (s *ProductRegistrationStatus) GetConditions() []metav1.Condition {
	return s.Conditions
}

func (s *ProductRegistrationStatus) HasCondition(condType string) bool {
	for _, c := range s.Conditions {
		if c.Type == condType {
			return true
		}
	}
	return false
}

func (s *ProductRegistrationStatus) IsConditionTrue(condType string) bool {
	for _, c := range s.Conditions {
		if c.Type == condType {
			return c.Status == metav1.ConditionTrue
		}
	}
	return false
}

func (s *ProductRegistrationStatus) GetCondition(condType string) *metav1.Condition {
	for i := range s.Conditions {
		if s.Conditions[i].Type == condType {
			return &s.Conditions[i]
		}
	}
	return nil
}

func (s *ProductRegistrationStatus) SetCondition(condType string, status metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()

	for i := range s.Conditions {
		if s.Conditions[i].Type == condType {
			s.Conditions[i].Status = status
			s.Conditions[i].Reason = reason
			s.Conditions[i].Message = message
			s.Conditions[i].LastTransitionTime = now
			return
		}
	}

	s.Conditions = append(s.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
	})
}

func (s *ProductRegistrationStatus) RemoveCondition(condType string) {
	var newConditions []metav1.Condition
	for _, cond := range s.Conditions {
		if cond.Type != condType {
			newConditions = append(newConditions, cond)
		}
	}
	s.Conditions = newConditions
}

// Core Registration Fields

func (s *ProductRegistrationStatus) GetSCCSystemID() *int {
	return s.SCCSystemID
}

func (s *ProductRegistrationStatus) SetSCCSystemID(id *int) {
	s.SCCSystemID = id
}

func (s *ProductRegistrationStatus) GetRegisteredProduct() *string {
	return s.RegisteredProduct
}

func (s *ProductRegistrationStatus) SetRegisteredProduct(product *string) {
	s.RegisteredProduct = product
}

func (s *ProductRegistrationStatus) GetRegistrationProcessedTS() *metav1.Time {
	return s.RegistrationProcessedTS
}

func (s *ProductRegistrationStatus) SetRegistrationProcessedTS(ts *metav1.Time) {
	s.RegistrationProcessedTS = ts
}

func (s *ProductRegistrationStatus) GetRegistrationExpiresAt() *metav1.Time {
	return s.RegistrationExpiresAt
}

func (s *ProductRegistrationStatus) SetRegistrationExpiresAt(ts *metav1.Time) {
	s.RegistrationExpiresAt = ts
}

func (s *ProductRegistrationStatus) GetSubscriptionInfo() *SubscriptionMetadata {
	return s.SubscriptionInfo
}

func (s *ProductRegistrationStatus) SetSubscriptionInfo(info *SubscriptionMetadata) {
	s.SubscriptionInfo = info
}

// Activation State

func (s *ProductRegistrationStatus) GetActivated() bool {
	return s.ActivationStatus.Activated
}

func (s *ProductRegistrationStatus) SetActivated(activated bool) {
	s.ActivationStatus.Activated = activated
}

func (s *ProductRegistrationStatus) GetLastValidatedTS() *metav1.Time {
	return s.ActivationStatus.LastValidatedTS
}

func (s *ProductRegistrationStatus) SetLastValidatedTS(ts *metav1.Time) {
	s.ActivationStatus.LastValidatedTS = ts
}

func (s *ProductRegistrationStatus) GetSystemURL() *string {
	return s.ActivationStatus.SystemURL
}

func (s *ProductRegistrationStatus) SetSystemURL(url *string) {
	s.ActivationStatus.SystemURL = url
}

// Secret References

func (s *ProductRegistrationStatus) GetSystemCredentialsSecretRef() *corev1.SecretReference {
	return s.SystemCredentialsSecretRef
}

func (s *ProductRegistrationStatus) SetSystemCredentialsSecretRef(ref *corev1.SecretReference) {
	s.SystemCredentialsSecretRef = ref
}

func (s *ProductRegistrationStatus) GetOfflineRegistrationRequestRef() *corev1.SecretReference {
	return s.OfflineRegistrationRequest
}

func (s *ProductRegistrationStatus) SetOfflineRegistrationRequestRef(ref *corev1.SecretReference) {
	s.OfflineRegistrationRequest = ref
}
