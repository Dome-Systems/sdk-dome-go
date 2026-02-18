package dome

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestDecodeToken_ValidCredentials(t *testing.T) {
	creds := agentCredentials{
		VaultAddr:    "http://vault:8200",
		AuthMethod:   "approle",
		RoleID:       "role-123",
		SecretID:     "secret-456",
		OIDCRoleName: "dome-agent",
	}
	data, _ := json.Marshal(creds)
	token := base64.StdEncoding.EncodeToString(data)

	result, err := decodeToken(token)
	if err != nil {
		t.Fatalf("decodeToken error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.VaultAddr != "http://vault:8200" {
		t.Errorf("VaultAddr = %q, want %q", result.VaultAddr, "http://vault:8200")
	}
	if result.AuthMethod != "approle" {
		t.Errorf("AuthMethod = %q, want %q", result.AuthMethod, "approle")
	}
	if result.RoleID != "role-123" {
		t.Errorf("RoleID = %q, want %q", result.RoleID, "role-123")
	}
	if result.SecretID != "secret-456" {
		t.Errorf("SecretID = %q, want %q", result.SecretID, "secret-456")
	}
	if result.OIDCRoleName != "dome-agent" {
		t.Errorf("OIDCRoleName = %q, want %q", result.OIDCRoleName, "dome-agent")
	}
}

func TestDecodeToken_KubernetesCredentials(t *testing.T) {
	creds := agentCredentials{
		VaultAddr:    "http://vault:8200",
		AuthMethod:   "kubernetes",
		KubeAuthRole: "dome-k8s-role",
		OIDCRoleName: "dome-agent",
	}
	data, _ := json.Marshal(creds)
	token := base64.StdEncoding.EncodeToString(data)

	result, err := decodeToken(token)
	if err != nil {
		t.Fatalf("decodeToken error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.AuthMethod != "kubernetes" {
		t.Errorf("AuthMethod = %q, want %q", result.AuthMethod, "kubernetes")
	}
	if result.KubeAuthRole != "dome-k8s-role" {
		t.Errorf("KubeAuthRole = %q, want %q", result.KubeAuthRole, "dome-k8s-role")
	}
}

func TestDecodeToken_EmptyString(t *testing.T) {
	result, err := decodeToken("")
	if err != nil {
		t.Fatalf("decodeToken error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for empty token, got %+v", result)
	}
}

func TestDecodeToken_PlainAPIKey(t *testing.T) {
	// A plain API key is not valid base64 JSON — should return nil, nil.
	result, err := decodeToken("dome_sk_abc123def456")
	if err != nil {
		t.Fatalf("decodeToken error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for plain API key, got %+v", result)
	}
}

func TestDecodeToken_InvalidBase64(t *testing.T) {
	result, err := decodeToken("not-valid-base64!!!")
	if err != nil {
		t.Fatalf("decodeToken error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for invalid base64, got %+v", result)
	}
}

func TestDecodeToken_ValidBase64ButNotJSON(t *testing.T) {
	token := base64.StdEncoding.EncodeToString([]byte("just a plain string"))
	result, err := decodeToken(token)
	if err != nil {
		t.Fatalf("decodeToken error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for non-JSON base64, got %+v", result)
	}
}

func TestDecodeToken_ValidJSONButNoVaultAddr(t *testing.T) {
	// JSON without vault_addr — should fall through to plain token handling.
	data, _ := json.Marshal(map[string]string{"foo": "bar"})
	token := base64.StdEncoding.EncodeToString(data)

	result, err := decodeToken(token)
	if err != nil {
		t.Fatalf("decodeToken error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for JSON without vault_addr, got %+v", result)
	}
}

func TestDecodeToken_WhitespaceHandling(t *testing.T) {
	creds := agentCredentials{
		VaultAddr:    "http://vault:8200",
		AuthMethod:   "approle",
		RoleID:       "role-123",
		SecretID:     "secret-456",
		OIDCRoleName: "dome-agent",
	}
	data, _ := json.Marshal(creds)
	token := "  " + base64.StdEncoding.EncodeToString(data) + "  \n"

	result, err := decodeToken(token)
	if err != nil {
		t.Fatalf("decodeToken error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for whitespace-padded token")
	}
	if result.VaultAddr != "http://vault:8200" {
		t.Errorf("VaultAddr = %q, want %q", result.VaultAddr, "http://vault:8200")
	}
}
