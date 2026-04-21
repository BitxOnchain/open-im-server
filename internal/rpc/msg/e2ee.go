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

package msg

import (
	"context"
	"encoding/json"

	"github.com/openimsdk/open-im-server/v3/pkg/crypto"
	"github.com/openimsdk/protocol/sdkws"
	"github.com/openimsdk/tools/log"
)

// E2EEMessageProcessor handles E2EE message processing
type E2EEMessageProcessor struct {
	sessionManager *crypto.SessionManager
}

// NewE2EEMessageProcessor creates a new E2EE message processor
func NewE2EEMessageProcessor() *E2EEMessageProcessor {
	return &E2EEMessageProcessor{
		sessionManager: crypto.NewSessionManager(),
	}
}

// ProcessOutgoingMessage processes an outgoing message for E2EE
// Returns true if the message was encrypted, false otherwise
func (e *E2EEMessageProcessor) ProcessOutgoingMessage(ctx context.Context, msgData *sdkws.MsgData) (bool, error) {
	// Check if E2EE is enabled for this conversation
	// For now, we check if there's an active session
	sessionID := crypto.GenerateSessionID(int(msgData.SessionType), msgData.SendID, msgData.RecvID)

	session, exists := e.sessionManager.GetE2EEManager().GetSession(sessionID)
	if !exists || !session.IsEstablished {
		// No active E2EE session, send as plaintext
		log.ZDebug(ctx, "No E2EE session, sending plaintext", "sessionID", sessionID)
		return false, nil
	}

	// Encrypt the message content
	e2eMsg, encrypted, err := e.sessionManager.GetE2EEManager().EncryptMessage(sessionID, msgData.Content)
	if err != nil {
		log.ZError(ctx, "Failed to encrypt message", err, "sessionID", sessionID)
		return false, err
	}

	if !encrypted {
		return false, nil
	}

	// Serialize the E2EE message wrapper
	e2eMsgJSON, err := json.Marshal(e2eMsg)
	if err != nil {
		log.ZError(ctx, "Failed to marshal E2EE message", err)
		return false, err
	}

	// Update the message content with encrypted data
	msgData.Content = e2eMsgJSON

	// Mark the message as encrypted
	// We use a special content type or options field to indicate E2EE
	// For now, we'll store it in the Options field
	if msgData.Options == nil {
		msgData.Options = make(map[string]bool)
	}
	msgData.Options["e2ee"] = true

	log.ZDebug(ctx, "Message encrypted", "sessionID", sessionID, "msgID", msgData.ClientMsgID)
	return true, nil
}

// ProcessIncomingMessage processes an incoming message for E2EE decryption
// Returns true if the message was decrypted, false otherwise
func (e *E2EEMessageProcessor) ProcessIncomingMessage(ctx context.Context, msgData *sdkws.MsgData) (bool, error) {
	// Check if message is E2EE encrypted
	if msgData.Options == nil || !msgData.Options["e2ee"] {
		// Not an E2EE message
		return false, nil
	}

	// Determine the session ID
	// For incoming messages, we use the sender's ID
	sessionID := crypto.GenerateSessionID(int(msgData.SessionType), msgData.SendID, msgData.RecvID)

	// Parse the E2EE message wrapper
	var e2eMsg crypto.E2EEMessage
	err := json.Unmarshal(msgData.Content, &e2eMsg)
	if err != nil {
		log.ZError(ctx, "Failed to unmarshal E2EE message", err, "sessionID", sessionID)
		return false, err
	}

	// Decrypt the message
	plaintext, decrypted, err := e.sessionManager.GetE2EEManager().DecryptMessage(sessionID, &e2eMsg)
	if err != nil {
		log.ZError(ctx, "Failed to decrypt message", err, "sessionID", sessionID)
		return false, err
	}

	if !decrypted {
		return false, nil
	}

	// Update the message content with decrypted data
	msgData.Content = plaintext

	log.ZDebug(ctx, "Message decrypted", "sessionID", sessionID, "msgID", msgData.ClientMsgID)
	return true, nil
}

// IsE2EEEnabled checks if E2EE is enabled for a conversation
func (e *E2EEMessageProcessor) IsE2EEEnabled(sessionType int, userID1, userID2 string) bool {
	sessionID := crypto.GenerateSessionID(sessionType, userID1, userID2)
	return e.sessionManager.GetE2EEManager().IsE2EEnabled(sessionID)
}

// GetSessionManager returns the session manager
func (e *E2EEMessageProcessor) GetSessionManager() *crypto.SessionManager {
	return e.sessionManager
}

// E2EEMetadata represents E2EE metadata stored with messages
type E2EEMetadata struct {
	IsEncrypted bool   `json:"is_encrypted"`
	Version     int    `json:"version"`
	SessionID   string `json:"session_id"`
}

// ExtractE2EEMetadata extracts E2EE metadata from message options
func ExtractE2EEMetadata(options map[string]bool) *E2EEMetadata {
	if options == nil {
		return nil
	}

	isEncrypted, ok := options["e2ee"]
	if !ok || !isEncrypted {
		return nil
	}

	return &E2EEMetadata{
		IsEncrypted: true,
		Version:     1,
	}
}
