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
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/openimsdk/tools/log"
)

// SessionManager provides high-level session management for E2EE
type SessionManager struct {
	manager *E2EEManager
}

// NewSessionManager creates a new session manager
func NewSessionManager() *SessionManager {
	return &SessionManager{
		manager: NewE2EEManager(),
	}
}

// GetE2EEManager returns the underlying E2EE manager
func (sm *SessionManager) GetE2EEManager() *E2EEManager {
	return sm.manager
}

// InitiateKeyExchange initiates a key exchange as the initiator
// This should be called when a user wants to start an encrypted conversation
func (sm *SessionManager) InitiateKeyExchange(ctx context.Context, sessionID string, localIdentityKey *KeyPair, remotePublicKey string) (string, error) {
	// Decode remote public key
	remotePubKey, err := DecodePublicKey(remotePublicKey)
	if err != nil {
		return "", fmt.Errorf("invalid remote public key: %w", err)
	}

	// Create session
	_, err = sm.manager.ratchet.InitiateSession(sessionID, localIdentityKey, &remotePubKey)
	if err != nil {
		return "", fmt.Errorf("failed to initiate session: %w", err)
	}

	// Get ephemeral public key to send to remote
	ephemeralPubKey, err := sm.manager.ratchet.GetSessionPublicKey(sessionID)
	if err != nil {
		return "", fmt.Errorf("failed to get session public key: %w", err)
	}

	log.ZDebug(ctx, "Initiated E2EE session", "sessionID", sessionID, "ephemeralPubKey", ephemeralPubKey)
	return ephemeralPubKey, nil
}

// HandleKeyResponse handles a key exchange response as the initiator
// This is called when the responder sends back their ephemeral public key
func (sm *SessionManager) HandleKeyResponse(ctx context.Context, sessionID string, responderEphemeralPubKey string) error {
	session, exists := sm.manager.store.GetSession(sessionID)
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	if !session.IsInitiator {
		return fmt.Errorf("only initiator should handle key response")
	}

	// Decode responder's ephemeral public key
	remoteEphemeralPub, err := DecodePublicKey(responderEphemeralPubKey)
	if err != nil {
		return fmt.Errorf("invalid responder ephemeral public key: %w", err)
	}

	// Perform DH ratchet step
	err = sm.manager.ratchet.dhRatchetStep(session, &remoteEphemeralPub)
	if err != nil {
		return fmt.Errorf("failed to perform DH ratchet: %w", err)
	}

	log.ZDebug(ctx, "Handled E2EE key response", "sessionID", sessionID)
	return nil
}

// RespondToKeyExchange responds to a key exchange as the responder
// This should be called when a user receives a key exchange request
func (sm *SessionManager) RespondToKeyExchange(ctx context.Context, sessionID string, localIdentityKey *KeyPair, initiatorEphemeralPubKey string) (string, error) {
	// Decode initiator's ephemeral public key
	initiatorPubKey, err := DecodePublicKey(initiatorEphemeralPubKey)
	if err != nil {
		return "", fmt.Errorf("invalid initiator ephemeral public key: %w", err)
	}

	// Create session
	_, err = sm.manager.ratchet.RespondToSession(sessionID, localIdentityKey, &initiatorPubKey)
	if err != nil {
		return "", fmt.Errorf("failed to respond to session: %w", err)
	}

	// Get ephemeral public key to send back
	ephemeralPubKey, err := sm.manager.ratchet.GetSessionPublicKey(sessionID)
	if err != nil {
		return "", fmt.Errorf("failed to get session public key: %w", err)
	}

	log.ZDebug(ctx, "Responded to E2EE key exchange", "sessionID", sessionID, "ephemeralPubKey", ephemeralPubKey)
	return ephemeralPubKey, nil
}

// GetSessionState returns the current state of a session
func (sm *SessionManager) GetSessionState(sessionID string) (*SessionStateInfo, error) {
	session, exists := sm.manager.store.GetSession(sessionID)
	if !exists {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	return &SessionStateInfo{
		SessionID:      session.SessionID,
		IsEstablished:  session.IsEstablished,
		IsInitiator:    session.IsInitiator,
		SendMessageNum: session.SendMessageNum,
		RecvMessageNum: session.RecvMessageNum,
		CreatedAt:      session.CreatedAt,
		UpdatedAt:      session.UpdatedAt,
	}, nil
}

// SessionStateInfo provides a read-only view of session state
type SessionStateInfo struct {
	SessionID      string `json:"session_id"`
	IsEstablished  bool   `json:"is_established"`
	IsInitiator    bool   `json:"is_initiator"`
	SendMessageNum uint32 `json:"send_message_num"`
	RecvMessageNum uint32 `json:"recv_message_num"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
}

// ListSessions returns all active session IDs
func (sm *SessionManager) ListSessions() []string {
	return sm.manager.store.ListSessions()
}

// CloseSession closes and removes a session
func (sm *SessionManager) CloseSession(sessionID string) error {
	sm.manager.store.DeleteSession(sessionID)
	return nil
}

// ExportSession exports a session state for backup/sync
func (sm *SessionManager) ExportSession(sessionID string) ([]byte, error) {
	session, exists := sm.manager.store.GetSession(sessionID)
	if !exists {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	return json.Marshal(session)
}

// ImportSession imports a session state from backup/sync
func (sm *SessionManager) ImportSession(data []byte) error {
	session, err := sm.manager.ratchet.DeserializeSession(data)
	if err != nil {
		return fmt.Errorf("failed to deserialize session: %w", err)
	}

	sm.manager.store.SaveSession(session)
	return nil
}

// EncryptForSession encrypts a message for a specific session
func (sm *SessionManager) EncryptForSession(sessionID string, plaintext []byte) (*E2EEMessage, error) {
	if !sm.manager.IsE2EEnabled(sessionID) {
		return nil, fmt.Errorf("E2EE not enabled for session: %s", sessionID)
	}

	e2eMsg, _, err := sm.manager.EncryptMessage(sessionID, plaintext)
	return e2eMsg, err
}

// DecryptForSession decrypts a message for a specific session
func (sm *SessionManager) DecryptForSession(sessionID string, e2eMsg *E2EEMessage) ([]byte, error) {
	if !e2eMsg.IsEncrypted {
		return []byte(e2eMsg.Payload), nil
	}

	if !sm.manager.IsE2EEnabled(sessionID) {
		return nil, fmt.Errorf("E2EE not enabled for session: %s", sessionID)
	}

	plaintext, _, err := sm.manager.DecryptMessage(sessionID, e2eMsg)
	return plaintext, err
}

// SessionStoreInterface defines the interface for session storage
type SessionStoreInterface interface {
	GetSession(sessionID string) (*SessionState, bool)
	SaveSession(session *SessionState)
	DeleteSession(sessionID string)
	ListSessions() []string
}

// PersistentSessionStore provides persistent storage for sessions
type PersistentSessionStore struct {
	memoryStore *SessionStore
	// TODO: Add database/redis backend for persistence
}

// NewPersistentSessionStore creates a new persistent session store
func NewPersistentSessionStore() *PersistentSessionStore {
	return &PersistentSessionStore{
		memoryStore: NewSessionStore(),
	}
}

// GetSession retrieves a session
func (ps *PersistentSessionStore) GetSession(sessionID string) (*SessionState, bool) {
	return ps.memoryStore.GetSession(sessionID)
}

// SaveSession saves a session
func (ps *PersistentSessionStore) SaveSession(session *SessionState) {
	ps.memoryStore.SaveSession(session)
	// TODO: Persist to database/redis
}

// DeleteSession removes a session
func (ps *PersistentSessionStore) DeleteSession(sessionID string) {
	ps.memoryStore.DeleteSession(sessionID)
	// TODO: Remove from database/redis
}

// ListSessions returns all session IDs
func (ps *PersistentSessionStore) ListSessions() []string {
	return ps.memoryStore.ListSessions()
}

// CleanupExpiredSessions removes sessions that have been inactive for too long
func (sm *SessionManager) CleanupExpiredSessions(maxInactiveDuration time.Duration) int {
	cutoff := time.Now().Add(-maxInactiveDuration).Unix()
	count := 0

	for _, sessionID := range sm.manager.store.ListSessions() {
		session, exists := sm.manager.store.GetSession(sessionID)
		if exists && session.UpdatedAt < cutoff {
			sm.manager.store.DeleteSession(sessionID)
			count++
		}
	}

	return count
}
