package types

// Primitive types and constants that are shared across the library.
// These would otherwise need to be duplicated in every consuming repo.

// RegistrationMode defines the registration workflow (online or offline).
type RegistrationMode string

const (
	RegistrationModeOnline  RegistrationMode = "online"
	RegistrationModeOffline RegistrationMode = "offline"
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

// Well-known labels for secrets
const (
	// LabelRegistrationType identifies a secret as a SUSE registration
	LabelRegistrationType = "suse.com/type"
	// LabelRegistrationTypeValue is the value for registration secrets
	LabelRegistrationTypeValue = "registration"
	// LabelCredentialsType identifies a secret as SCC system credentials
	LabelCredentialsType = "suse.com/credentials"
)

// Secret data keys
const (
	// SecretKeyRegistrationCode is the key for the registration code in entrypoint secrets
	SecretKeyRegistrationCode = "registrationCode"
	// SecretKeyMode is the key for registration mode
	SecretKeyMode = "mode"
	// SecretKeyRegistrationURL is the key for custom SCC URL
	SecretKeyRegistrationURL = "registrationURL"
	// SecretKeyCertificate is the key for certificates (CA cert, offline cert)
	SecretKeyCertificate = "certificate"

	// SecretKeyLogin is the key for SCC login (in credentials secret created by library)
	SecretKeyLogin = "login"
	// SecretKeyPassword is the key for SCC password (in credentials secret created by library)
	SecretKeyPassword = "password"

	// SecretKeyOfflineRequest is the key for offline registration request XML
	SecretKeyOfflineRequest = "request.xml"
)
