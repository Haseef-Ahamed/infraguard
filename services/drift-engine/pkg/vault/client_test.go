package vault_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/infraguard/drift-engine/pkg/vault"
)

func TestNewClient_InvalidAddress(t *testing.T) {
	// Connection is lazy — creation succeeds even for unreachable address
	client, err := vault.NewClient("http://localhost:9999", "fake-token")
	require.NoError(t, err)
	assert.NotNil(t, client)
}

func TestGet_RealVault(t *testing.T) {
	client, err := vault.NewClient("http://localhost:8200", "root")
	require.NoError(t, err)

	secrets, err := client.Get("aws")
	if err != nil {
		t.Skipf("Vault not available or secret missing: %v", err)
	}

	// Verify required keys exist with correct values
	assert.Equal(t, "test", secrets["access_key"])
	assert.Equal(t, "test", secrets["secret_key"])
	assert.Equal(t, "us-east-1", secrets["region"])

	// Endpoint can be localhost OR host IP depending on environment
	// Just verify it is a non-empty URL pointing to port 4566
	assert.NotEmpty(t, secrets["endpoint"])
	assert.Contains(t, secrets["endpoint"], ":4566")
}
