package vault_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/infraguard/drift-engine/pkg/vault"
)

func TestNewClient_InvalidAddress(t *testing.T) {
	// A completely unreachable address should still create
	// a client (connection is lazy) — error only on first call
	client, err := vault.NewClient("http://localhost:9999", "fake-token")
	require.NoError(t, err)
	assert.NotNil(t, client)
}

func TestGet_RealVault(t *testing.T) {
	// This test only runs when Vault is available locally
	// It reads the aws secret we stored in Phase 1 Part 3
	client, err := vault.NewClient("http://localhost:8200", "root")
	require.NoError(t, err)

	secrets, err := client.Get("aws")
	if err != nil {
		t.Skipf("Vault not available or secret missing: %v", err)
	}

	assert.Equal(t, "test", secrets["access_key"])
	assert.Equal(t, "test", secrets["secret_key"])
	assert.Equal(t, "us-east-1", secrets["region"])
	assert.Equal(t, "http://localhost:4566", secrets["endpoint"])
}
