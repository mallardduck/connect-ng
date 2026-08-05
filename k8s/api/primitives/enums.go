package primitives

// RegistrationMode defines the registration workflow (online or offline).
type RegistrationMode string

const (
	RegistrationModeOnline  RegistrationMode = "online"
	RegistrationModeOffline RegistrationMode = "offline"
)

// SecretRole identifies the purpose of a secret
type SecretRole string

const (
	SecretRoleSCCCredentials      SecretRole = "scc-credentials"
	SecretRoleOfflineRequest      SecretRole = "offline-request"
	SecretRoleOfflineCert         SecretRole = "offline-certificate"
	SecretRoleRegistrationCode    SecretRole = "registration-code"
	SecretRoleRegistrationURLCert SecretRole = "registration-url-cert"
)
