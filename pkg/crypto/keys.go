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
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/curve25519"
)

const (
	// KeySize is the size of X25519 keys in bytes
	KeySize = 32
)

// KeyPair represents an X25519 key pair for ECDH
type KeyPair struct {
	PrivateKey [KeySize]byte
	PublicKey  [KeySize]byte
}

// EphemeralKeyPair represents a temporary key pair for single message encryption
type EphemeralKeyPair struct {
	PrivateKey [KeySize]byte
	PublicKey  [KeySize]byte
}

// GenerateKeyPair generates a new X25519 key pair
func GenerateKeyPair() (*KeyPair, error) {
	var kp KeyPair

	// Generate random private key
	_, err := rand.Read(kp.PrivateKey[:])
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Clamp the private key according to X25519 spec
	kp.PrivateKey[0] &= 248
	kp.PrivateKey[31] &= 127
	kp.PrivateKey[31] |= 64

	// Generate public key from private key using X25519
	pub, err := curve25519.X25519(kp.PrivateKey[:], curve25519.Basepoint)
	if err != nil {
		return nil, fmt.Errorf("failed to generate public key: %w", err)
	}
	copy(kp.PublicKey[:], pub)

	return &kp, nil
}

// GenerateEphemeralKeyPair generates a new ephemeral X25519 key pair for single message use
func GenerateEphemeralKeyPair() (*EphemeralKeyPair, error) {
	var ekp EphemeralKeyPair

	// Generate random private key
	_, err := rand.Read(ekp.PrivateKey[:])
	if err != nil {
		return nil, fmt.Errorf("failed to generate ephemeral private key: %w", err)
	}

	// Clamp the private key according to X25519 spec
	ekp.PrivateKey[0] &= 248
	ekp.PrivateKey[31] &= 127
	ekp.PrivateKey[31] |= 64

	// Generate public key from private key using X25519
	pub, err := curve25519.X25519(ekp.PrivateKey[:], curve25519.Basepoint)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ephemeral public key: %w", err)
	}
	copy(ekp.PublicKey[:], pub)

	return &ekp, nil
}

// ECDH performs Elliptic Curve Diffie-Hellman key exchange
// Returns the shared secret computed from private key and other's public key
func ECDH(privateKey, publicKey *[KeySize]byte) ([KeySize]byte, error) {
	var sharedSecret [KeySize]byte

	shared, err := curve25519.X25519(privateKey[:], publicKey[:])
	if err != nil {
		return sharedSecret, fmt.Errorf("ECDH failed: %w", err)
	}

	copy(sharedSecret[:], shared)
	return sharedSecret, nil
}

// EncodePublicKey encodes a public key to base64 string
func EncodePublicKey(publicKey *[KeySize]byte) string {
	return base64.StdEncoding.EncodeToString(publicKey[:])
}

// DecodePublicKey decodes a base64 string to public key
func DecodePublicKey(encoded string) ([KeySize]byte, error) {
	var publicKey [KeySize]byte

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return publicKey, fmt.Errorf("failed to decode public key: %w", err)
	}

	if len(decoded) != KeySize {
		return publicKey, fmt.Errorf("invalid public key size: expected %d, got %d", KeySize, len(decoded))
	}

	copy(publicKey[:], decoded)
	return publicKey, nil
}

// EncodePrivateKey encodes a private key to base64 string
func EncodePrivateKey(privateKey *[KeySize]byte) string {
	return base64.StdEncoding.EncodeToString(privateKey[:])
}

// DecodePrivateKey decodes a base64 string to private key
func DecodePrivateKey(encoded string) ([KeySize]byte, error) {
	var privateKey [KeySize]byte

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return privateKey, fmt.Errorf("failed to decode private key: %w", err)
	}

	if len(decoded) != KeySize {
		return privateKey, fmt.Errorf("invalid private key size: expected %d, got %d", KeySize, len(decoded))
	}

	copy(privateKey[:], decoded)
	return privateKey, nil
}

// GetPublicKeyFromPrivate derives the public key from a private key
func GetPublicKeyFromPrivate(privateKey *[KeySize]byte) [KeySize]byte {
	var publicKey [KeySize]byte
	// Use X25519 to derive public key from private key
	pub, _ := curve25519.X25519(privateKey[:], curve25519.Basepoint)
	copy(publicKey[:], pub)
	return publicKey
}
