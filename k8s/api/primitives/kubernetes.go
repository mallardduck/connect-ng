package primitives

// Kubernetes labels
const (
	// Secret identification labels
	LabelSecretRole            = "suse.com/secret-role"
	LabelRegistrationType      = "suse.com/type"
	LabelRegistrationTypeValue = "registration"
	LabelCredentialsType       = "suse.com/credentials"

	// SCC operator labels (hash-based resource management)
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

// Managed-by values
const (
	ManagedByValueSecretBroker = "secret-broker"
)

// Condition type constants - standard across all products.
// These match scc-operator's condition types.
const (
	// General conditions
	ConditionTypeReady       = "Ready"
	ConditionTypeDone        = "Done"
	ConditionTypeProgressing = "Progressing"
	ConditionTypeFailure     = "Failure"

	// Online registration conditions
	ConditionTypeAnnounced = "RegistrationAnnounced"
	ConditionTypeURLReady  = "RegistrationSccUrlReady"
	ConditionTypeActivated = "RegistrationActivated"
	ConditionTypeKeepalive = "RegistrationKeepalive"

	// Offline registration conditions
	ConditionTypeOfflineRequestReady     = "OfflineRequestReady"
	ConditionTypeOfflineCertificateReady = "OfflineCertificateReady"
	ConditionTypeOfflineActivationDone   = "OfflineActivationDone"
)
