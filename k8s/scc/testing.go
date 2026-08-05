package scc

import "github.com/SUSE/connect-ng/pkg/connection"

// memoryCredentials is a simple in-memory implementation of connection.Credentials.
// FOR TESTING ONLY - does not persist credentials, so token updates are lost.
//
// Production code must use SecretBackedCredentials to ensure SCC token rotation
// is persisted to Kubernetes secrets.
type memoryCredentials struct {
	login       string
	password    string
	systemToken string
}

// NewMemoryCredentials creates in-memory credentials for testing.
// DO NOT USE IN PRODUCTION - token updates will be lost.
func NewMemoryCredentials(login, password string) connection.Credentials {
	return &memoryCredentials{
		login:    login,
		password: password,
	}
}

func (m *memoryCredentials) HasAuthentication() bool {
	return m.login != "" && m.password != ""
}

func (m *memoryCredentials) Token() (string, error) {
	return m.systemToken, nil
}

func (m *memoryCredentials) UpdateToken(token string) error {
	m.systemToken = token
	return nil
}

func (m *memoryCredentials) Login() (string, string, error) {
	return m.login, m.password, nil
}

func (m *memoryCredentials) SetLogin(login, password string) error {
	m.login = login
	m.password = password
	return nil
}
