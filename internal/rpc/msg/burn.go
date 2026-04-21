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
	"sync"
	"time"

	"github.com/openimsdk/open-im-server/v3/pkg/authverify"
	"github.com/openimsdk/protocol/msg"
	"github.com/openimsdk/tools/errs"
	"github.com/openimsdk/tools/log"
)

// BurnDuration constants for burn-after-reading options (in seconds).
const (
	BurnDuration5Sec   = 5
	BurnDuration30Sec  = 30
	BurnDuration1Min   = 60
	BurnDuration5Min   = 300
	BurnDuration1Hour  = 3600
	BurnDuration24Hour = 86400
)

// BurnStatus represents the status of a burn task.
type BurnStatus int32

const (
	BurnStatusPending BurnStatus = iota
	BurnStatusScheduled
	BurnStatusCancelled
	BurnStatusExecuted
)

// BurnTask represents a single burn task for a message.
type BurnTask struct {
	UserID         string
	ConversationID string
	Seq            int64
	ClientMsgID    string
	BurnAt         time.Time // Time when the message should be burned
	Status         BurnStatus
	CreatedAt      time.Time
}

// BurnMsgManager manages all burn tasks.
type BurnMsgManager struct {
	mu    sync.RWMutex
	tasks map[string]*BurnTask // key: conversationID:seq or clientMsgID
}

// Global burn manager instance.
var burnManager *BurnMsgManager

func init() {
	burnManager = &BurnMsgManager{
		tasks: make(map[string]*BurnTask),
	}
}

// GetBurnTaskKey generates a unique key for a burn task.
func GetBurnTaskKey(conversationID string, seq int64) string {
	return conversationID + ":" + string(rune(seq))
}

// GetBurnTaskKeyByMsgID generates a unique key using client message ID.
func GetBurnTaskKeyByMsgID(clientMsgID string) string {
	return "msg:" + clientMsgID
}

// ScheduleBurn schedules a message for burn after reading.
// duration is the number of seconds until the message should be burned.
func (m *msgServer) ScheduleBurn(ctx context.Context, conversationID string, seq int64, clientMsgID string, duration int64) error {
	if duration <= 0 {
		return errs.ErrArgs.WrapMsg("burn duration must be positive")
	}

	// Calculate burn time
	burnAt := time.Now().Add(time.Duration(duration) * time.Second)

	// Generate userID from conversationID or fetch it
	userID := ""
	if conversationID != "" {
		// Extract userID from conversationID format: si_{userID}_{userID} or similar
		userID = extractUserIDFromConversationID(conversationID)
	}

	task := &BurnTask{
		UserID:         userID,
		ConversationID: conversationID,
		Seq:            seq,
		ClientMsgID:    clientMsgID,
		BurnAt:         burnAt,
		Status:         BurnStatusScheduled,
		CreatedAt:      time.Now(),
	}

	// Store task in manager
	key := GetBurnTaskKey(conversationID, seq)
	if clientMsgID != "" {
		key = GetBurnTaskKeyByMsgID(clientMsgID)
	}

	burnManager.mu.Lock()
	burnManager.tasks[key] = task
	burnManager.mu.Unlock()

	log.ZDebug(ctx, "ScheduleBurn", "conversationID", conversationID, "seq", seq, "clientMsgID", clientMsgID, "burnAt", burnAt)

	// Schedule the actual burn in a goroutine
	go m.executeBurnAfterDelay(ctx, conversationID, seq, clientMsgID, duration)

	return nil
}

// CancelBurn cancels a scheduled burn task.
func (m *msgServer) CancelBurn(ctx context.Context, conversationID string, seq int64, clientMsgID string) error {
	key := GetBurnTaskKey(conversationID, seq)
	if clientMsgID != "" {
		key = GetBurnTaskKeyByMsgID(clientMsgID)
	}

	burnManager.mu.Lock()
	defer burnManager.mu.Unlock()

	if task, ok := burnManager.tasks[key]; ok {
		task.Status = BurnStatusCancelled
		delete(burnManager.tasks, key)
		log.ZDebug(ctx, "CancelBurn", "conversationID", conversationID, "seq", seq, "clientMsgID", clientMsgID)
	}

	return nil
}

// ExecuteBurn physically deletes the message and its associated files.
func (m *msgServer) ExecuteBurn(ctx context.Context, conversationID string, seq int64) error {
	if conversationID == "" || seq <= 0 {
		return errs.ErrArgs.WrapMsg("invalid conversationID or seq")
	}

	log.ZInfo(ctx, "ExecuteBurn", "conversationID", conversationID, "seq", seq)

	// Use the physical delete method to remove the message
	if err := m.MsgDatabase.DeleteMsgsPhysicalBySeqs(ctx, conversationID, []int64{seq}); err != nil {
		log.ZError(ctx, "ExecuteBurn failed", err, "conversationID", conversationID, "seq", seq)
		return err
	}

	// Clean up burn task
	key := GetBurnTaskKey(conversationID, seq)
	burnManager.mu.Lock()
	delete(burnManager.tasks, key)
	burnManager.mu.Unlock()

	log.ZInfo(ctx, "ExecuteBurn success", "conversationID", conversationID, "seq", seq)

	return nil
}

// ExecuteBurnByClientMsgID burns a message by its client message ID.
func (m *msgServer) ExecuteBurnByClientMsgID(ctx context.Context, conversationID string, clientMsgID string) error {
	if clientMsgID == "" {
		return errs.ErrArgs.WrapMsg("clientMsgID is required")
	}

	// Get the seq from the message
	_, _, msgs, err := m.MsgDatabase.GetMsgBySeqs(ctx, "", conversationID, nil)
	if err != nil {
		return err
	}

	var targetSeq int64
	for _, msg := range msgs {
		if msg.ClientMsgID == clientMsgID {
			targetSeq = msg.Seq
			break
		}
	}

	if targetSeq <= 0 {
		return errs.ErrRecordNotFound.WrapMsg("message not found")
	}

	return m.ExecuteBurn(ctx, conversationID, targetSeq)
}

// executeBurnAfterDelay waits for the specified duration and then burns the message.
func (m *msgServer) executeBurnAfterDelay(ctx context.Context, conversationID string, seq int64, clientMsgID string, duration int64) {
	select {
	case <-time.After(time.Duration(duration) * time.Second):
		// Create a new context with operation ID
		operationCtx := context.Background()
		if err := m.ExecuteBurn(operationCtx, conversationID, seq); err != nil {
			log.ZError(operationCtx, "executeBurnAfterDelay ExecuteBurn failed", err,
				"conversationID", conversationID, "seq", seq)
		}
	case <-ctx.Done():
		log.ZDebug(ctx, "executeBurnAfterDelay cancelled", "conversationID", conversationID, "seq", seq)
	}
}

// GetScheduledBurns returns all scheduled burn tasks (for monitoring/debugging).
func (m *msgServer) GetScheduledBurns(ctx context.Context) ([]*BurnTask, error) {
	burnManager.mu.RLock()
	defer burnManager.mu.RUnlock()

	tasks := make([]*BurnTask, 0, len(burnManager.tasks))
	for _, task := range burnManager.tasks {
		if task.Status == BurnStatusScheduled {
			tasks = append(tasks, task)
		}
	}
	return tasks, nil
}

// DestructExpiredBurnMsgs checks and executes burns for expired messages.
// This is called by the cron task as a fallback mechanism.
func (m *msgServer) DestructExpiredBurnMsgs(ctx context.Context, req *msg.DestructMsgsReq) (*msg.DestructMsgsResp, error) {
	if err := authverify.CheckAdmin(ctx); err != nil {
		return nil, err
	}

	burnManager.mu.Lock()
	defer burnManager.mu.Unlock()

	now := time.Now()
	expiredKeys := make([]string, 0)

	for key, task := range burnManager.tasks {
		if task.Status == BurnStatusScheduled && task.BurnAt.Before(now) {
			expiredKeys = append(expiredKeys, key)
		}
	}

	var count int32
	for _, key := range expiredKeys {
		task := burnManager.tasks[key]
		if err := m.ExecuteBurn(ctx, task.ConversationID, task.Seq); err != nil {
			log.ZError(ctx, "DestructExpiredBurnMsgs ExecuteBurn failed", err,
				"conversationID", task.ConversationID, "seq", task.Seq)
			continue
		}
		task.Status = BurnStatusExecuted
		count++
	}

	return &msg.DestructMsgsResp{Count: count}, nil
}

// BurnExpiredMessages burns messages that have passed their burn time.
// This method is used by the cron task to cleanup burn messages.
// limit: maximum number of messages to burn per call.
func (m *msgServer) BurnExpiredMessages(ctx context.Context, limit int) (int, error) {
	burnManager.mu.Lock()
	defer burnManager.mu.Unlock()

	now := time.Now()
	expiredKeys := make([]string, 0, limit)

	for key, task := range burnManager.tasks {
		if task.Status == BurnStatusScheduled && task.BurnAt.Before(now) {
			expiredKeys = append(expiredKeys, key)
			if len(expiredKeys) >= limit {
				break
			}
		}
	}

	var count int
	for _, key := range expiredKeys {
		task := burnManager.tasks[key]
		if err := m.ExecuteBurn(ctx, task.ConversationID, task.Seq); err != nil {
			log.ZError(ctx, "BurnExpiredMessages ExecuteBurn failed", err,
				"conversationID", task.ConversationID, "seq", task.Seq)
			continue
		}
		task.Status = BurnStatusExecuted
		count++
	}

	return count, nil
}

// StartBurnTimer starts a burn timer when a message is marked as read.
// This should be called from the message read callback.
func (m *msgServer) StartBurnTimer(ctx context.Context, conversationID string, userID string, seqs []int64, burnDuration int64) error {
	if burnDuration <= 0 {
		return nil // Not a burn message
	}

	// Get message details to find clientMsgID
	_, _, msgs, err := m.MsgDatabase.GetMsgBySeqs(ctx, userID, conversationID, seqs)
	if err != nil {
		return err
	}

	for _, msgData := range msgs {
		if msgData != nil {
			if err := m.ScheduleBurn(ctx, conversationID, msgData.Seq, msgData.ClientMsgID, burnDuration); err != nil {
				log.ZError(ctx, "StartBurnTimer ScheduleBurn failed", err,
					"conversationID", conversationID, "seq", msgData.Seq)
				continue
			}
		}
	}

	return nil
}

// GetBurnDurationFromSeconds converts seconds to burn duration option.
// Returns 0 if the duration is not a valid burn option.
func GetBurnDurationFromSeconds(seconds int64) int64 {
	switch seconds {
	case BurnDuration5Sec, BurnDuration30Sec, BurnDuration1Min,
		BurnDuration5Min, BurnDuration1Hour, BurnDuration24Hour:
		return seconds
	default:
		return 0
	}
}

// IsValidBurnDuration checks if a duration is a valid burn option.
func IsValidBurnDuration(seconds int64) bool {
	return GetBurnDurationFromSeconds(seconds) > 0
}

// extractUserIDFromConversationID extracts user ID from conversation ID.
// Conversation ID formats: si_{userID}_{userID}, gr_{groupID}, etc.
func extractUserIDFromConversationID(conversationID string) string {
	// Simple extraction for single chat format: si_{userID1}_{userID2}
	// This needs to be adapted based on actual conversation ID format
	return ""
}
