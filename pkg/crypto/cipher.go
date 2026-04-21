// Copyright © 2023 OpenIM. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/sha3"
)

const (
	// AESKeySize is the size of AES-256 key in bytes
	AESKeySize = 32
	// NonceSize is the size of GCM nonce in bytes
	NonceSize = 12
	// SaltSize is the size of salt for HKDF
	SaltSize = 32
	// Info string for HKDF key derivation
	HKDFInfo = "OpenIM-E2EE-v1"
)

// EncryptedMessage represents an encrypted message with all necessary metadata
type EncryptedMessage struct {
	Ciphertext      string `json:"ciphertext"`        // Base64 encoded encrypted content
	Nonce           string `json:"nonce"`             // Base64 encoded nonce
	Salt            string `json:"salt"`              // Base64 encoded salt for HKDF
	EphemeralPubKey string `json:"ephemeral_pub_key"` // Base64 encoded ephemeral public key
	Version         int    `json:"version"`           // Encryption version
}

// Cipher provides AES-256-GCM encryption/decryption functionality
type Cipher struct{}

// NewCipher creates a new Cipher instance
func NewCipher() *Cipher {
	return &Cipher{}
}

// deriveKey derives an AES-256 key from shared secret using HKDF-SHA3-256
func (c *Cipher) deriveKey(sharedSecret, salt []byte) ([]byte, error) {
	hkdfReader := hkdf.New(sha3.New256, sharedSecret, salt, []byte(HKDFInfo))

	key := make([]byte, AESKeySize)
	_, err := io.ReadFull(hkdfReader, key)
	if err != nil {
		return nil, fmt.Errorf("failed to derive key: %w", err)
	}

	return key, nil
}

// Encrypt encrypts plaintext using AES-256-GCM with a key derived from shared secret
// Each call generates a new ephemeral key pair and random nonce/salt
func (c *Cipher) Encrypt(plaintext []byte, recipientPublicKey *[KeySize]byte) (*EncryptedMessage, error) {
	// Generate ephemeral key pair for this message
	ephemeralKP, err := GenerateEphemeralKeyPair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate ephemeral key: %w", err)
	}

	// Perform ECDH to get shared secret
	sharedSecret, err := ECDH(&ephemeralKP.PrivateKey, recipientPublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to compute shared secret: %w", err)
	}

	// Generate random salt
	salt := make([]byte, SaltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	// Derive encryption key from shared secret
	key, err := c.deriveKey(sharedSecret[:], salt)
	if err != nil {
		return nil, err
	}

	// Create AES-GCM cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate random nonce
	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt the plaintext
	ciphertext := aesGCM.Seal(nil, nonce, plaintext, nil)

	return &EncryptedMessage{
		Ciphertext:      base64.StdEncoding.EncodeToString(ciphertext),
		Nonce:           base64.StdEncoding.EncodeToString(nonce),
		Salt:            base64.StdEncoding.EncodeToString(salt),
		EphemeralPubKey: EncodePublicKey(&ephemeralKP.PublicKey),
		Version:         1,
	}, nil
}

// Decrypt decrypts an EncryptedMessage using the recipient's private key
func (c *Cipher) Decrypt(em *EncryptedMessage, recipientPrivateKey *[KeySize]byte) ([]byte, error) {
	// Decode ephemeral public key
	ephemeralPubKey, err := DecodePublicKey(em.EphemeralPubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ephemeral public key: %w", err)
	}

	// Perform ECDH to get shared secret
	sharedSecret, err := ECDH(recipientPrivateKey, &ephemeralPubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to compute shared secret: %w", err)
	}

	// Decode salt
	salt, err := base64.StdEncoding.DecodeString(em.Salt)
	if err != nil {
		return nil, fmt.Errorf("failed to decode salt: %w", err)
	}

	// Derive encryption key from shared secret
	key, err := c.deriveKey(sharedSecret[:], salt)
	if err != nil {
		return nil, err
	}

	// Create AES-GCM cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Decode nonce
	nonce, err := base64.StdEncoding.DecodeString(em.Nonce)
	if err != nil {
		return nil, fmt.Errorf("failed to decode nonce: %w", err)
	}

	// Decode ciphertext
	ciphertext, err := base64.StdEncoding.DecodeString(em.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	// Decrypt
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}

// EncryptWithSharedSecret encrypts plaintext with a pre-computed shared secret
// This is useful when the shared secret is already established (e.g., from a previous key exchange)
func (c *Cipher) EncryptWithSharedSecret(plaintext []byte, sharedSecret []byte) (*EncryptedMessage, error) {
	// Generate random salt
	salt := make([]byte, SaltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	// Derive encryption key from shared secret
	key, err := c.deriveKey(sharedSecret, salt)
	if err != nil {
		return nil, err
	}

	// Create AES-GCM cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate random nonce
	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt the plaintext
	ciphertext := aesGCM.Seal(nil, nonce, plaintext, nil)

	return &EncryptedMessage{
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Salt:       base64.StdEncoding.EncodeToString(salt),
		Version:    1,
	}, nil
}

// DecryptWithSharedSecret decrypts an EncryptedMessage with a pre-computed shared secret
func (c *Cipher) DecryptWithSharedSecret(em *EncryptedMessage, sharedSecret []byte) ([]byte, error) {
	// Decode salt
	salt, err := base64.StdEncoding.DecodeString(em.Salt)
	if err != nil {
		return nil, fmt.Errorf("failed to decode salt: %w", err)
	}

	// Derive encryption key from shared secret
	key, err := c.deriveKey(sharedSecret, salt)
	if err != nil {
		return nil, err
	}

	// Create AES-GCM cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Decode nonce
	nonce, err := base64.StdEncoding.DecodeString(em.Nonce)
	if err != nil {
		return nil, fmt.Errorf("failed to decode nonce: %w", err)
	}

	// Decode ciphertext
	ciphertext, err := base64.StdEncoding.DecodeString(em.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	// Decrypt
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}
