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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/sha3"
)

const (
	// MaxSkippedMessages is the maximum number of skipped message keys to store
	MaxSkippedMessages = 1000
	// RootKeySize is the size of the root key in bytes
	RootKeySize = 32
	// ChainKeySize is the size of chain keys in bytes
	ChainKeySize = 32
	// MessageKeySize is the size of message keys in bytes
	MessageKeySize = 32
)

// SessionState represents the state of an E2EE session between two users
type SessionState struct {
	SessionID string `json:"session_id"`

	// Identity keys (long-term)
	LocalIdentityPrivate string `json:"local_identity_private"` // Base64 encoded
	LocalIdentityPublic  string `json:"local_identity_public"`  // Base64 encoded
	RemoteIdentityPublic string `json:"remote_identity_public"` // Base64 encoded (optional, for verification)

	// Root key for deriving chain keys
	RootKey string `json:"root_key"` // Base64 encoded

	// Sending chain (for messages we send)
	SendChainKey   string `json:"send_chain_key"` // Base64 encoded
	SendMessageNum uint32 `json:"send_message_num"`

	// Receiving chain (for messages we receive)
	RecvChainKey   string `json:"recv_chain_key"` // Base64 encoded
	RecvMessageNum uint32 `json:"recv_message_num"`

	// Ratchet state
	SendEphemeralPrivate string `json:"send_ephemeral_private"` // Base64 encoded (current ephemeral private key)
	RecvEphemeralPublic  string `json:"recv_ephemeral_public"`  // Base64 encoded (last received ephemeral public key)

	// Skipped message keys (for out-of-order messages)
	SkippedMessageKeys map[string]string `json:"skipped_message_keys"` // Key: "chainKey:messageNum", Value: base64 message key

	// Metadata
	CreatedAt     int64 `json:"created_at"`
	UpdatedAt     int64 `json:"updated_at"`
	IsInitiator   bool  `json:"is_initiator"`
	IsEstablished bool  `json:"is_established"`
}

// SessionStore manages E2EE session states
type SessionStore struct {
	sessions map[string]*SessionState
	mutex    sync.RWMutex
}

// NewSessionStore creates a new session store
func NewSessionStore() *SessionStore {
	return &SessionStore{
		sessions: make(map[string]*SessionState),
	}
}

// GetSession retrieves a session by ID
func (s *SessionStore) GetSession(sessionID string) (*SessionState, bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	session, exists := s.sessions[sessionID]
	return session, exists
}

// SaveSession saves a session to the store
func (s *SessionStore) SaveSession(session *SessionState) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	session.UpdatedAt = time.Now().Unix()
	s.sessions[session.SessionID] = session
}

// DeleteSession removes a session from the store
func (s *SessionStore) DeleteSession(sessionID string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	delete(s.sessions, sessionID)
}

// ListSessions returns all session IDs
func (s *SessionStore) ListSessions() []string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	ids := make([]string, 0, len(s.sessions))
	for id := range s.sessions {
		ids = append(ids, id)
	}
	return ids
}

// DoubleRatchet manages the double ratchet algorithm for a session
type DoubleRatchet struct {
	store  *SessionStore
	cipher *Cipher
}

// NewDoubleRatchet creates a new double ratchet manager
func NewDoubleRatchet(store *SessionStore) *DoubleRatchet {
	return &DoubleRatchet{
		store:  store,
		cipher: NewCipher(),
	}
}

// InitiateSession creates a new session as the initiator
// This is called by the user who starts the encrypted conversation
func (dr *DoubleRatchet) InitiateSession(sessionID string, localIdentityKey *KeyPair, remotePublicKey *[KeySize]byte) (*SessionState, error) {
	// Generate ephemeral key pair for initial key exchange
	ephemeralKP, err := GenerateEphemeralKeyPair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate ephemeral key: %w", err)
	}

	// Perform X3DH-like key agreement:
	// sharedSecret = ECDH(ephemeralPrivate, remoteIdentityPublic) || ECDH(localIdentityPrivate, remoteIdentityPublic)
	sharedSecret1, err := ECDH(&ephemeralKP.PrivateKey, remotePublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to compute ECDH: %w", err)
	}

	sharedSecret2, err := ECDH(&localIdentityKey.PrivateKey, remotePublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to compute ECDH: %w", err)
	}

	// Combine shared secrets
	combinedSecret := make([]byte, 0, KeySize*2)
	combinedSecret = append(combinedSecret, sharedSecret1[:]...)
	combinedSecret = append(combinedSecret, sharedSecret2[:]...)

	// Derive root key using HKDF
	rootKey, err := dr.deriveRootKey(combinedSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to derive root key: %w", err)
	}

	// Derive initial chain keys
	sendChainKey, recvChainKey, err := dr.deriveInitialChainKeys(rootKey)
	if err != nil {
		return nil, fmt.Errorf("failed to derive chain keys: %w", err)
	}

	session := &SessionState{
		SessionID:            sessionID,
		LocalIdentityPrivate: EncodePrivateKey(&localIdentityKey.PrivateKey),
		LocalIdentityPublic:  EncodePublicKey(&localIdentityKey.PublicKey),
		RemoteIdentityPublic: EncodePublicKey(remotePublicKey),
		RootKey:              base64.StdEncoding.EncodeToString(rootKey),
		SendChainKey:         base64.StdEncoding.EncodeToString(sendChainKey),
		SendMessageNum:       0,
		RecvChainKey:         base64.StdEncoding.EncodeToString(recvChainKey),
		RecvMessageNum:       0,
		SendEphemeralPrivate: EncodePrivateKey(&ephemeralKP.PrivateKey),
		SkippedMessageKeys:   make(map[string]string),
		CreatedAt:            time.Now().Unix(),
		UpdatedAt:            time.Now().Unix(),
		IsInitiator:          true,
		IsEstablished:        true,
	}

	dr.store.SaveSession(session)
	return session, nil
}

// RespondToSession creates a new session as the responder
// This is called by the user who accepts the encrypted conversation
func (dr *DoubleRatchet) RespondToSession(sessionID string, localIdentityKey *KeyPair, ephemeralPublicKey *[KeySize]byte) (*SessionState, error) {
	// Generate ephemeral key pair for this session
	ephemeralKP, err := GenerateEphemeralKeyPair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate ephemeral key: %w", err)
	}

	// Perform X3DH-like key agreement (reverse of initiator):
	// sharedSecret = ECDH(localIdentityPrivate, ephemeralPublic) || ECDH(localEphemeralPrivate, remoteIdentityPublic)
	// Note: In a real implementation, we'd also include the initiator's identity key
	sharedSecret1, err := ECDH(&localIdentityKey.PrivateKey, ephemeralPublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to compute ECDH: %w", err)
	}

	// For the responder, we need to wait for the initiator's identity key
	// For now, we use a simplified approach
	combinedSecret := make([]byte, 0, KeySize)
	combinedSecret = append(combinedSecret, sharedSecret1[:]...)

	// Derive root key using HKDF
	rootKey, err := dr.deriveRootKey(combinedSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to derive root key: %w", err)
	}

	// Derive initial chain keys (note: send/recv are swapped for responder)
	recvChainKey, sendChainKey, err := dr.deriveInitialChainKeys(rootKey)
	if err != nil {
		return nil, fmt.Errorf("failed to derive chain keys: %w", err)
	}

	session := &SessionState{
		SessionID:            sessionID,
		LocalIdentityPrivate: EncodePrivateKey(&localIdentityKey.PrivateKey),
		LocalIdentityPublic:  EncodePublicKey(&localIdentityKey.PublicKey),
		RootKey:              base64.StdEncoding.EncodeToString(rootKey),
		SendChainKey:         base64.StdEncoding.EncodeToString(sendChainKey),
		SendMessageNum:       0,
		RecvChainKey:         base64.StdEncoding.EncodeToString(recvChainKey),
		RecvMessageNum:       0,
		SendEphemeralPrivate: EncodePrivateKey(&ephemeralKP.PrivateKey),
		RecvEphemeralPublic:  EncodePublicKey(ephemeralPublicKey),
		SkippedMessageKeys:   make(map[string]string),
		CreatedAt:            time.Now().Unix(),
		UpdatedAt:            time.Now().Unix(),
		IsInitiator:          false,
		IsEstablished:        true,
	}

	dr.store.SaveSession(session)
	return session, nil
}

// deriveRootKey derives the root key from initial shared secret
func (dr *DoubleRatchet) deriveRootKey(sharedSecret []byte) ([]byte, error) {
	hkdfReader := hkdf.New(sha3.New256, sharedSecret, nil, []byte("OpenIM-RootKey-v1"))

	rootKey := make([]byte, RootKeySize)
	_, err := hkdfReader.Read(rootKey)
	if err != nil {
		return nil, err
	}

	return rootKey, nil
}

// deriveInitialChainKeys derives the initial sending and receiving chain keys from root key
func (dr *DoubleRatchet) deriveInitialChainKeys(rootKey []byte) (sendChainKey, recvChainKey []byte, err error) {
	hkdfReader := hkdf.New(sha3.New256, rootKey, nil, []byte("OpenIM-ChainKeys-v1"))

	keys := make([]byte, ChainKeySize*2)
	_, err = hkdfReader.Read(keys)
	if err != nil {
		return nil, nil, err
	}

	return keys[:ChainKeySize], keys[ChainKeySize:], nil
}

// kdfRatchetStep performs a KDF ratchet step to derive next chain key and message key
func (dr *DoubleRatchet) kdfRatchetStep(chainKey []byte) (nextChainKey, messageKey []byte, err error) {
	hkdfReader := hkdf.New(sha3.New256, chainKey, nil, []byte("OpenIM-MessageKey-v1"))

	keys := make([]byte, ChainKeySize+MessageKeySize)
	_, err = hkdfReader.Read(keys)
	if err != nil {
		return nil, nil, err
	}

	return keys[:ChainKeySize], keys[ChainKeySize:], nil
}

// dhRatchetStep performs a DH ratchet step when a new ephemeral key is received
func (dr *DoubleRatchet) dhRatchetStep(session *SessionState, newRemoteEphemeralPublic *[KeySize]byte) error {
	// Decode current ephemeral private key
	ephemeralPrivate, err := DecodePrivateKey(session.SendEphemeralPrivate)
	if err != nil {
		return fmt.Errorf("failed to decode ephemeral private key: %w", err)
	}

	// Decode root key
	rootKey, err := base64.StdEncoding.DecodeString(session.RootKey)
	if err != nil {
		return fmt.Errorf("failed to decode root key: %w", err)
	}

	// Perform DH with new remote ephemeral key
	sharedSecret, err := ECDH(&ephemeralPrivate, newRemoteEphemeralPublic)
	if err != nil {
		return fmt.Errorf("failed to compute ECDH: %w", err)
	}

	// Update root key with new shared secret
	newRootKeyInput := make([]byte, len(rootKey)+KeySize)
	copy(newRootKeyInput, rootKey)
	copy(newRootKeyInput[len(rootKey):], sharedSecret[:])

	newRootKey, err := dr.deriveRootKey(newRootKeyInput)
	if err != nil {
		return fmt.Errorf("failed to derive new root key: %w", err)
	}

	// Generate new ephemeral key pair for future messages
	newEphemeralKP, err := GenerateEphemeralKeyPair()
	if err != nil {
		return fmt.Errorf("failed to generate new ephemeral key: %w", err)
	}

	// Derive new chain keys
	sendChainKey, recvChainKey, err := dr.deriveInitialChainKeys(newRootKey)
	if err != nil {
		return fmt.Errorf("failed to derive new chain keys: %w", err)
	}

	// Update session state
	session.RootKey = base64.StdEncoding.EncodeToString(newRootKey)
	session.SendChainKey = base64.StdEncoding.EncodeToString(sendChainKey)
	session.RecvChainKey = base64.StdEncoding.EncodeToString(recvChainKey)
	session.SendEphemeralPrivate = EncodePrivateKey(&newEphemeralKP.PrivateKey)
	session.RecvEphemeralPublic = EncodePublicKey(newRemoteEphemeralPublic)
	session.SendMessageNum = 0
	session.RecvMessageNum = 0

	dr.store.SaveSession(session)
	return nil
}

// EncryptMessage encrypts a message for the given session
func (dr *DoubleRatchet) EncryptMessage(sessionID string, plaintext []byte) (*EncryptedMessage, error) {
	session, exists := dr.store.GetSession(sessionID)
	if !exists {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	if !session.IsEstablished {
		return nil, fmt.Errorf("session not established: %s", sessionID)
	}

	// Decode current sending chain key
	chainKey, err := base64.StdEncoding.DecodeString(session.SendChainKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode chain key: %w", err)
	}

	// Derive message key and next chain key
	nextChainKey, messageKey, err := dr.kdfRatchetStep(chainKey)
	if err != nil {
		return nil, fmt.Errorf("failed to derive message key: %w", err)
	}

	// Encrypt message with message key
	em, err := dr.cipher.EncryptWithSharedSecret(plaintext, messageKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt message: %w", err)
	}

	// Update session state
	session.SendChainKey = base64.StdEncoding.EncodeToString(nextChainKey)
	session.SendMessageNum++
	dr.store.SaveSession(session)

	return em, nil
}

// DecryptMessage decrypts a message for the given session
func (dr *DoubleRatchet) DecryptMessage(sessionID string, em *EncryptedMessage) ([]byte, error) {
	session, exists := dr.store.GetSession(sessionID)
	if !exists {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	if !session.IsEstablished {
		return nil, fmt.Errorf("session not established: %s", sessionID)
	}

	// Check if this is a new ephemeral key (DH ratchet step needed)
	if em.EphemeralPubKey != "" && em.EphemeralPubKey != session.RecvEphemeralPublic {
		newEphemeralPub, err := DecodePublicKey(em.EphemeralPubKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decode ephemeral public key: %w", err)
		}

		err = dr.dhRatchetStep(session, &newEphemeralPub)
		if err != nil {
			return nil, fmt.Errorf("failed to perform DH ratchet: %w", err)
		}
	}

	// Decode current receiving chain key
	chainKey, err := base64.StdEncoding.DecodeString(session.RecvChainKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode chain key: %w", err)
	}

	// Derive message key and next chain key
	nextChainKey, messageKey, err := dr.kdfRatchetStep(chainKey)
	if err != nil {
		return nil, fmt.Errorf("failed to derive message key: %w", err)
	}

	// Decrypt message
	plaintext, err := dr.cipher.DecryptWithSharedSecret(em, messageKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt message: %w", err)
	}

	// Update session state
	session.RecvChainKey = base64.StdEncoding.EncodeToString(nextChainKey)
	session.RecvMessageNum++
	dr.store.SaveSession(session)

	return plaintext, nil
}

// SerializeSession serializes a session state to JSON
func (dr *DoubleRatchet) SerializeSession(session *SessionState) ([]byte, error) {
	return json.Marshal(session)
}

// DeserializeSession deserializes a session state from JSON
func (dr *DoubleRatchet) DeserializeSession(data []byte) (*SessionState, error) {
	var session SessionState
	err := json.Unmarshal(data, &session)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize session: %w", err)
	}

	if session.SkippedMessageKeys == nil {
		session.SkippedMessageKeys = make(map[string]string)
	}

	return &session, nil
}

// GetSessionPublicKey returns the current ephemeral public key for a session
// This is used to share with the other party
func (dr *DoubleRatchet) GetSessionPublicKey(sessionID string) (string, error) {
	session, exists := dr.store.GetSession(sessionID)
	if !exists {
		return "", fmt.Errorf("session not found: %s", sessionID)
	}

	ephemeralPrivate, err := DecodePrivateKey(session.SendEphemeralPrivate)
	if err != nil {
		return "", fmt.Errorf("failed to decode ephemeral private key: %w", err)
	}

	publicKey := GetPublicKeyFromPrivate(&ephemeralPrivate)
	return EncodePublicKey(&publicKey), nil
}
