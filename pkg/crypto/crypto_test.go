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
	"bytes"
	"testing"
)

func TestGenerateKeyPair(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	// Check that private key is not all zeros
	isZero := true
	for _, b := range kp.PrivateKey {
		if b != 0 {
			isZero = false
			break
		}
	}
	if isZero {
		t.Error("Private key should not be all zeros")
	}

	// Check that public key is not all zeros
	isZero = true
	for _, b := range kp.PublicKey {
		if b != 0 {
			isZero = false
			break
		}
	}
	if isZero {
		t.Error("Public key should not be all zeros")
	}
}

func TestECDH(t *testing.T) {
	// Generate two key pairs
	kp1, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate first key pair: %v", err)
	}

	kp2, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate second key pair: %v", err)
	}

	// Compute shared secrets
	shared1, err := ECDH(&kp1.PrivateKey, &kp2.PublicKey)
	if err != nil {
		t.Fatalf("Failed to compute first shared secret: %v", err)
	}

	shared2, err := ECDH(&kp2.PrivateKey, &kp1.PublicKey)
	if err != nil {
		t.Fatalf("Failed to compute second shared secret: %v", err)
	}

	// Shared secrets should be equal
	if !bytes.Equal(shared1[:], shared2[:]) {
		t.Error("Shared secrets should be equal")
	}
}

func TestCipherEncryptDecrypt(t *testing.T) {
	cipher := NewCipher()

	// Generate key pair for recipient
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	// Test message
	plaintext := []byte("Hello, this is a secret message!")

	// Encrypt
	encrypted, err := cipher.Encrypt(plaintext, &kp.PublicKey)
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}

	// Verify encrypted message fields
	if encrypted.Ciphertext == "" {
		t.Error("Ciphertext should not be empty")
	}
	if encrypted.Nonce == "" {
		t.Error("Nonce should not be empty")
	}
	if encrypted.Salt == "" {
		t.Error("Salt should not be empty")
	}
	if encrypted.EphemeralPubKey == "" {
		t.Error("Ephemeral public key should not be empty")
	}
	if encrypted.Version != 1 {
		t.Errorf("Version should be 1, got %d", encrypted.Version)
	}

	// Decrypt
	decrypted, err := cipher.Decrypt(encrypted, &kp.PrivateKey)
	if err != nil {
		t.Fatalf("Failed to decrypt: %v", err)
	}

	// Verify decrypted message
	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("Decrypted message doesn't match original: got %s, want %s", decrypted, plaintext)
	}
}

func TestCipherEncryptDecryptWithSharedSecret(t *testing.T) {
	cipher := NewCipher()

	// Generate key pairs and compute shared secret
	kp1, _ := GenerateKeyPair()
	kp2, _ := GenerateKeyPair()
	sharedSecret, _ := ECDH(&kp1.PrivateKey, &kp2.PublicKey)

	// Test message
	plaintext := []byte("Hello, this is another secret message!")

	// Encrypt with shared secret
	encrypted, err := cipher.EncryptWithSharedSecret(plaintext, sharedSecret[:])
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}

	// Decrypt with shared secret
	decrypted, err := cipher.DecryptWithSharedSecret(encrypted, sharedSecret[:])
	if err != nil {
		t.Fatalf("Failed to decrypt: %v", err)
	}

	// Verify decrypted message
	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("Decrypted message doesn't match original: got %s, want %s", decrypted, plaintext)
	}
}

func TestDoubleRatchet(t *testing.T) {
	// Create session store and ratchet
	store := NewSessionStore()
	ratchet := NewDoubleRatchet(store)

	// Generate identity keys for both parties
	aliceIdentity, _ := GenerateKeyPair()
	bobIdentity, _ := GenerateKeyPair()

	// Alice initiates session
	sessionID := "test-session"
	_, err := ratchet.InitiateSession(sessionID, aliceIdentity, &bobIdentity.PublicKey)
	if err != nil {
		t.Fatalf("Failed to initiate session: %v", err)
	}

	// Bob responds to session
	// Get Alice's ephemeral public key
	session, _ := store.GetSession(sessionID)
	aliceEphemeralPub, _ := DecodePublicKey(session.LocalIdentityPublic)
	_, err = ratchet.RespondToSession(sessionID+"-bob", bobIdentity, &aliceEphemeralPub)
	if err != nil {
		t.Fatalf("Failed to respond to session: %v", err)
	}
}

func TestE2EEManager(t *testing.T) {
	manager := NewE2EEManager()

	// Generate identity keys
	aliceIdentity, _ := GenerateKeyPair()
	bobIdentity, _ := GenerateKeyPair()

	// Initiate session
	sessionID := "test-e2ee-session"
	err := manager.InitiateSession(sessionID, aliceIdentity, &bobIdentity.PublicKey)
	if err != nil {
		t.Fatalf("Failed to initiate session: %v", err)
	}

	// Check if E2EE is enabled
	if !manager.IsE2EEnabled(sessionID) {
		t.Error("E2EE should be enabled for the session")
	}

	// Encrypt a message
	plaintext := []byte("Secret message from Alice to Bob")
	e2eMsg, encrypted, err := manager.EncryptMessage(sessionID, plaintext)
	if err != nil {
		t.Fatalf("Failed to encrypt message: %v", err)
	}
	if !encrypted {
		t.Error("Message should be encrypted")
	}

	// For testing decryption, we need to set up Bob's side
	// In real scenario, this would be done through proper key exchange
	_ = e2eMsg
}

func TestGenerateSessionID(t *testing.T) {
	// Test single chat session ID generation
	sessionID1 := GenerateSessionID(1, "userA", "userB")
	sessionID2 := GenerateSessionID(1, "userB", "userA")
	if sessionID1 != sessionID2 {
		t.Errorf("Session IDs should be the same regardless of order: %s vs %s", sessionID1, sessionID2)
	}

	// Test group chat session ID generation
	groupSessionID := GenerateSessionID(2, "group123", "")
	expected := "e2e:group:group123"
	if groupSessionID != expected {
		t.Errorf("Group session ID incorrect: got %s, want %s", groupSessionID, expected)
	}
}

func BenchmarkEncryptDecrypt(b *testing.B) {
	cipher := NewCipher()
	kp, _ := GenerateKeyPair()
	plaintext := []byte("Benchmark message for encryption and decryption")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encrypted, err := cipher.Encrypt(plaintext, &kp.PublicKey)
		if err != nil {
			b.Fatal(err)
		}
		_, err = cipher.Decrypt(encrypted, &kp.PrivateKey)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkECDH(b *testing.B) {
	kp1, _ := GenerateKeyPair()
	kp2, _ := GenerateKeyPair()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ECDH(&kp1.PrivateKey, &kp2.PublicKey)
		if err != nil {
			b.Fatal(err)
		}
	}
}
