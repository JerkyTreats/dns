package certificate

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-acme/lego/v4/registration"
)

// User implements acme.User interface for ACME registration
type User struct {
	Email        string
	Registration *registration.Resource
	key          crypto.PrivateKey
}

func (u *User) GetEmail() string                        { return u.Email }
func (u *User) GetRegistration() *registration.Resource { return u.Registration }
func (u *User) GetPrivateKey() crypto.PrivateKey        { return u.key }

// saveUser persists the ACME user registration and private key to disk
func saveUser(user *User, userFile, keyFile string) error {
	if err := os.MkdirAll(filepath.Dir(userFile), 0755); err != nil {
		return fmt.Errorf("could not create user directory: %w", err)
	}

	userData, err := json.Marshal(user.Registration)
	if err != nil {
		return fmt.Errorf("could not marshal user registration: %w", err)
	}

	if err := os.WriteFile(userFile, userData, 0644); err != nil {
		return fmt.Errorf("could not write user file: %w", err)
	}

	keyBytes, err := x509.MarshalECPrivateKey(user.key.(*ecdsa.PrivateKey))
	if err != nil {
		return fmt.Errorf("could not marshal private key: %w", err)
	}

	keyData := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	})

	if err := os.WriteFile(keyFile, keyData, 0600); err != nil {
		return fmt.Errorf("could not write key file: %w", err)
	}

	return nil
}

// loadUser loads an existing ACME user from disk
func loadUser(userFile, keyFile string) (*User, error) {
	userData, err := os.ReadFile(userFile)
	if err != nil {
		return nil, err
	}

	keyData, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(keyData)
	if block == nil {
		return nil, fmt.Errorf("could not decode private key")
	}

	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("could not parse private key: %w", err)
	}

	var reg registration.Resource
	if err := json.Unmarshal(userData, &reg); err != nil {
		return nil, fmt.Errorf("could not unmarshal user registration: %w", err)
	}

	return &User{
		Registration: &reg,
		key:          key,
	}, nil
}
