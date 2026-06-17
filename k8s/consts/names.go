package consts

// Secret name prefixes
const (
	SCCCredentialsSecretNamePrefix             = "scc-system-credentials-"
	RegistrationCodeSecretNamePrefix           = "registration-code-"
	OfflineRequestSecretNamePrefix             = "offline-request-"
	OfflineCertificateSecretNamePrefix         = "offline-certificate-"
	RegistrationURLCertificateSecretNamePrefix = "registration-url-cert-"
)

// RegistrationName constructs the name for a ProductRegistration CR from a hash.
// Based on SCC Operator's pattern: registration-{hash}
func RegistrationName(nameHash string) string {
	return "registration-" + nameHash
}

// SCCCredentialsSecretName constructs the name for SCC credentials secret
func SCCCredentialsSecretName(namePart string) string {
	return SCCCredentialsSecretNamePrefix + namePart
}

// RegistrationCodeSecretName constructs the name for registration code secret
func RegistrationCodeSecretName(namePart string) string {
	return RegistrationCodeSecretNamePrefix + namePart
}

// OfflineRequestSecretName constructs the name for offline request secret
func OfflineRequestSecretName(namePart string) string {
	return OfflineRequestSecretNamePrefix + namePart
}

// OfflineCertificateSecretName constructs the name for offline certificate secret
func OfflineCertificateSecretName(namePart string) string {
	return OfflineCertificateSecretNamePrefix + namePart
}

// RegistrationURLCertificateSecretName constructs the name for registration URL certificate secret
func RegistrationURLCertificateSecretName(namePart string) string {
	return RegistrationURLCertificateSecretNamePrefix + namePart
}
