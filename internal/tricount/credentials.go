package tricount

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Credentials identify this CLI as one Tricount device.
// The API stores the public key at registration and returns a session token.
// The private key is not kept, which matches the tricount-api Python client.
type Credentials struct {
	AppID        string `json:"app_id"`
	PublicKeyPEM string `json:"public_key_pem"`
}

// Generate creates a new app id and RSA public key.
func Generate() (Credentials, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return Credentials{}, fmt.Errorf("generate device key: %w", err)
	}
	pub := x509.MarshalPKCS1PublicKey(&key.PublicKey)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: pub})
	id, err := NewUUID()
	if err != nil {
		return Credentials{}, err
	}
	return Credentials{AppID: id, PublicKeyPEM: string(pemBytes)}, nil
}

// Save writes credentials so later commands reuse the same device identity.
func (c Credentials) Save(path string) error {
	if c.AppID == "" || c.PublicKeyPEM == "" {
		return fmt.Errorf("credentials are incomplete")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create credentials directory: %w", err)
	}
	body, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}
	return nil
}

// Load reads credentials written by this CLI or by the Python tricount-api client.
func Load(path string) (Credentials, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return Credentials{}, fmt.Errorf("read credentials: %w", err)
	}
	var c Credentials
	if err := json.Unmarshal(body, &c); err != nil {
		return Credentials{}, fmt.Errorf("credentials file %s is not JSON with app_id and public_key_pem: %w", path, err)
	}
	if c.AppID == "" || c.PublicKeyPEM == "" {
		return Credentials{}, fmt.Errorf("credentials file %s needs app_id and public_key_pem", path)
	}
	return c, nil
}

// LoadOrCreate loads credentials, or generates and saves them when the file is absent.
// The bool is true when a new file was written.
func LoadOrCreate(path string) (Credentials, bool, error) {
	if _, err := os.Stat(path); err == nil {
		c, loadErr := Load(path)
		return c, false, loadErr
	} else if !errors.Is(err, os.ErrNotExist) {
		return Credentials{}, false, err
	}
	c, err := Generate()
	if err != nil {
		return Credentials{}, false, err
	}
	if err := c.Save(path); err != nil {
		return Credentials{}, false, err
	}
	return c, true, nil
}

// Remove deletes a credentials file. A missing file is not an error.
func Remove(path string) (bool, error) {
	err := os.Remove(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}
