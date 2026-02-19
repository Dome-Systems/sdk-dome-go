package dome

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

// agentCredentials holds Vault credentials issued to an agent.
// This is an internal type — consumers never see it. They pass the opaque
// base64 token from `dome agents register` and the SDK handles the rest.
type agentCredentials struct {
	APIURL       string `json:"api_url,omitempty"`
	VaultAddr    string `json:"vault_addr"`
	AuthMethod   string `json:"auth_method"`
	RoleID       string `json:"role_id,omitempty"`
	SecretID     string `json:"secret_id,omitempty"`
	KubeAuthRole string `json:"kube_auth_role,omitempty"`
	IAMAuthRole  string `json:"iam_auth_role,omitempty"`
	OIDCRoleName string `json:"oidc_role_name,omitempty"`
}

// decodeToken deserializes a base64-encoded JSON token to agentCredentials.
// Returns nil, nil if the token is empty or not valid base64 JSON (which
// indicates it might be a plain API key).
func decodeToken(token string) (*agentCredentials, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, nil
	}

	data, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		// Not a base64 blob — might be a plain API key. Return nil so the
		// caller falls through to bearer token auth.
		return nil, nil
	}

	var creds agentCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		// Valid base64 but not JSON — treat as plain token.
		return nil, nil
	}

	// If we decoded valid JSON but it doesn't look like credentials
	// (no api_url and no vault_addr), treat as a plain token.
	if creds.APIURL == "" && creds.VaultAddr == "" {
		return nil, nil
	}

	return &creds, nil
}
