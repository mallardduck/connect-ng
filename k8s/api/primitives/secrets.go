package primitives

// Secret data keys - part of the API contract with users
const (
	// SecretKeyRegistrationCode is the key for the registration code in entrypoint secrets
	SecretKeyRegistrationCode = "registrationCode"
	// SecretKeyMode is the key for registration mode
	SecretKeyMode = "mode"
	// SecretKeyRegistrationURL is the key for custom SCC URL
	SecretKeyRegistrationURL = "registrationURL"
	// SecretKeyRegistrationURLCert is the key for custom SCC URL certificate
	SecretKeyRegistrationURLCert = "registrationURLCert"
	// SecretKeyCertificate is the key for certificates (CA cert, offline cert)
	SecretKeyCertificate = "certificate"
	// SecretKeyOfflineCertificate is the key for offline registration certificate
	SecretKeyOfflineCertificate = "offlineCertificate"

	// SecretKeyLogin is the key for SCC login (in credentials secret created by library)
	SecretKeyLogin = "login"
	// SecretKeyPassword is the key for SCC password (in credentials secret created by library)
	SecretKeyPassword = "password"
	// SecretKeySystemLogin is the key for SCC system login
	SecretKeySystemLogin = "systemLogin"
	// SecretKeySystemToken is the key for SCC system token
	SecretKeySystemToken = "systemToken"

	// SecretKeyRequestXML is the key for offline registration request XML
	SecretKeyRequestXML = "request.xml"
	// SecretKeyPayload is the key for metrics payload
	SecretKeyPayload = "payload"
	// SecretKeyRegistrationType is the key for registration type identifier
	SecretKeyRegistrationType = "registrationType"
)
