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

// Package crypto provides end-to-end encryption functionality for OpenIM.
// It implements X25519 ECDH key exchange, AES-256-GCM encryption, and HKDF key derivation.
// The double ratchet algorithm provides forward secrecy and future secrecy.
package crypto

import (
	"encoding/json"
	"fmt"
)

// E2EEManager manages end-to-end encryption for the IM service
type E2EEManager struct {
	store   *SessionStore
	ratchet *DoubleRatchet
	cipher  *Cipher
}

// NewE2EEManager creates a new E2EE manager
func NewE2EEManager() *E2EEManager {
	store := NewSessionStore()
	return &E2EEManager{
		store:   store,
		ratchet: NewDoubleRatchet(store),
		cipher:  NewCipher(),
	}
}

// E2EEMessage represents a message with E2EE metadata
type E2EEMessage struct {
	IsEncrypted bool   `json:"is_encrypted"`
	Version     int    `json:"version"`
	Payload     string `json:"payload"` // JSON-encoded EncryptedMessage or plaintext
}

// IsE2EEnabled checks if E2EE is enabled for a session
func (m *E2EEManager) IsE2EEnabled(sessionID string) bool {
	session, exists := m.store.GetSession(sessionID)
	return exists && session.IsEstablished
}

// EncryptMessage encrypts a message if E2EE is enabled for the session
// Returns the encrypted message wrapper and true if encryption was performed
func (m *E2EEManager) EncryptMessage(sessionID string, plaintext []byte) (*E2EEMessage, bool, error) {
	if !m.IsE2EEnabled(sessionID) {
		return &E2EEMessage{
			IsEncrypted: false,
			Version:     0,
			Payload:     string(plaintext),
		}, false, nil
	}

	encrypted, err := m.ratchet.EncryptMessage(sessionID, plaintext)
	if err != nil {
		return nil, false, fmt.Errorf("failed to encrypt message: %w", err)
	}

	payload, err := json.Marshal(encrypted)
	if err != nil {
		return nil, false, fmt.Errorf("failed to marshal encrypted message: %w", err)
	}

	return &E2EEMessage{
		IsEncrypted: true,
		Version:     encrypted.Version,
		Payload:     string(payload),
	}, true, nil
}

// DecryptMessage decrypts a message if it was encrypted
// Returns the decrypted plaintext and true if decryption was performed
func (m *E2EEManager) DecryptMessage(sessionID string, e2eMsg *E2EEMessage) ([]byte, bool, error) {
	if !e2eMsg.IsEncrypted {
		return []byte(e2eMsg.Payload), false, nil
	}

	var encrypted EncryptedMessage
	err := json.Unmarshal([]byte(e2eMsg.Payload), &encrypted)
	if err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal encrypted message: %w", err)
	}

	plaintext, err := m.ratchet.DecryptMessage(sessionID, &encrypted)
	if err != nil {
		return nil, false, fmt.Errorf("failed to decrypt message: %w", err)
	}

	return plaintext, true, nil
}

// InitiateSession initiates a new E2EE session
func (m *E2EEManager) InitiateSession(sessionID string, localIdentityKey *KeyPair, remotePublicKey *[KeySize]byte) error {
	_, err := m.ratchet.InitiateSession(sessionID, localIdentityKey, remotePublicKey)
	return err
}

// RespondToSession responds to an E2EE session initiation
func (m *E2EEManager) RespondToSession(sessionID string, localIdentityKey *KeyPair, ephemeralPublicKey *[KeySize]byte) error {
	_, err := m.ratchet.RespondToSession(sessionID, localIdentityKey, ephemeralPublicKey)
	return err
}

// GetSession returns a session by ID
func (m *E2EEManager) GetSession(sessionID string) (*SessionState, bool) {
	return m.store.GetSession(sessionID)
}

// DeleteSession deletes a session
func (m *E2EEManager) DeleteSession(sessionID string) {
	m.store.DeleteSession(sessionID)
}

// GetSessionStore returns the underlying session store
func (m *E2EEManager) GetSessionStore() *SessionStore {
	return m.store
}

// GetRatchet returns the underlying double ratchet
func (m *E2EEManager) GetRatchet() *DoubleRatchet {
	return m.ratchet
}

// GetCipher returns the underlying cipher
func (m *E2EEManager) GetCipher() *Cipher {
	return m.cipher
}

// GenerateSessionID generates a unique session ID for a conversation
// For single chat: "e2e:single:<userID1>:<userID2>" (sorted)
// For group chat: "e2e:group:<groupID>"
func GenerateSessionID(sessionType int, id1, id2 string) string {
	if sessionType == 1 { // Single chat
		if id1 < id2 {
			return fmt.Sprintf("e2e:single:%s:%s", id1, id2)
		}
		return fmt.Sprintf("e2e:single:%s:%s", id2, id1)
	}
	// Group chat
	return fmt.Sprintf("e2e:group:%s", id1)
}
