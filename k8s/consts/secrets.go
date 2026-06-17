package consts

// Secret data keys - these are part of the API contract with users
const (
	SecretKeySystemLogin         = "systemLogin"
	SecretKeyPassword            = "password"
	SecretKeySystemToken         = "systemToken"
	SecretKeyLogin               = "login"
	SecretKeyRegistrationCode    = "registrationCode"
	SecretKeyPayload             = "payload"
	SecretKeyCertificate         = "certificate"
	SecretKeyRequestXML          = "request.xml"
	SecretKeyMode                = "mode"
	SecretKeyRegistrationURL     = "registrationURL"
	SecretKeyRegistrationURLCert = "registrationURLCert"
	SecretKeyOfflineCertificate  = "offlineCertificate"
	SecretKeyRegistrationType    = "registrationType"
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
