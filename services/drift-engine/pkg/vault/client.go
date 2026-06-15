package vault

import (
	"fmt"

	vault "github.com/hashicorp/vault/api"
)

// Client wraps the HashiCorp Vault API client
type Client struct {
	logical *vault.Logical
}

// NewClient creates a new Vault client pointing at addr with the given token
func NewClient(addr, token string) (*Client, error) {
	cfg := vault.DefaultConfig()
	cfg.Address = addr

	v, err := vault.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("vault: create client: %w", err)
	}
	v.SetToken(token)

	return &Client{logical: v.Logical()}, nil
}

// Get returns all key-value pairs stored at the given KV v2 path.
// Example: Get("aws") reads from infraguard/data/aws
func (c *Client) Get(path string) (map[string]string, error) {
	secret, err := c.logical.Read("infraguard/data/" + path)
	if err != nil {
		return nil, fmt.Errorf("vault: read %s: %w", path, err)
	}
	if secret == nil {
		return nil, fmt.Errorf("vault: path infraguard/data/%s not found", path)
	}

	data, ok := secret.Data["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("vault: unexpected format at %s", path)
	}

	result := make(map[string]string, len(data))
	for k, v := range data {
		result[k] = fmt.Sprintf("%v", v)
	}
	return result, nil
}
