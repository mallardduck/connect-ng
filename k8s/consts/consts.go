// Package consts defines constants for domain-specific strings used across the k8s library.
//
// This ensures consistent usage and enables code navigation - you can find all usages
// by jumping to references of a constant, rather than grepping for string literals.
package consts

// Kubernetes labels
const (
	LabelSecretRole   = "suse.com/secret-role"
	LabelObjectSalt   = "scc.suse.com/object-salt"
	LabelNameSuffix   = "scc.suse.com/name-suffix"
	LabelSccHash      = "scc.suse.com/scc-hash"
	LabelSccManagedBy = "scc.suse.com/managed-by"
	LabelK8sManagedBy = "app.kubernetes.io/managed-by"
)

// Kubernetes annotations
const (
	AnnotationLastProcessed = "scc.suse.com/last-processed"
)

// Finalizers
const (
	FinalizerRegistration        = "registration.suse.com/managed-registration"
	FinalizerSCCCredentials      = "registration.suse.com/managed-credentials"
	FinalizerOfflineRequest      = "registration.suse.com/managed-offline-request"
	FinalizerRegistrationCode    = "registration.suse.com/managed-registration-code"
	FinalizerOfflineCertificate  = "registration.suse.com/managed-offline-certificate"
	FinalizerRegistrationURLCert = "registration.suse.com/managed-registration-url-cert"
)

// Environment variable names
const (
	EnvPrimeSCCRegistrationHostURL = "PRIME_SCC_REGISTRATION_HOST_URL"
	EnvDevMode                     = "DEV_MODE"
)

// Metrics payload keys - part of the metrics secret contract
const (
	MetricsKeyHostname = "hostname"
	MetricsKeyArch     = "arch"
)

// Managed-by values
const (
	ManagedByValueSecretBroker = "secret-broker"
)
