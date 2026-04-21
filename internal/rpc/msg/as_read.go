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
	"errors"

	"github.com/openimsdk/open-im-server/v3/pkg/authverify"
	cbapi "github.com/openimsdk/open-im-server/v3/pkg/callbackstruct"
	"github.com/openimsdk/protocol/constant"
	"github.com/openimsdk/protocol/msg"
	"github.com/openimsdk/protocol/sdkws"
	"github.com/openimsdk/tools/errs"
	"github.com/openimsdk/tools/log"
	"github.com/openimsdk/tools/utils/datautil"
	"github.com/redis/go-redis/v9"
)

// ConversationExConfig represents the extended configuration stored in Ex field.
type ConversationExConfig struct {
	BurnAfterReading        bool `json:"burnAfterReading"`
	BurnAfterReadingSeconds int  `json:"burnAfterReadingSeconds"`
}

// burnAfterReadingOptions defines the burn-after-reading configuration keys.
const (
	// BurnAfterReadingOptionKey is the option key for burn after reading.
	BurnAfterReadingOptionKey = "burn_after_reading"
	// BurnAfterReadingSecondsKey is the key for burn duration in seconds.
	BurnAfterReadingSecondsKey = "burn_after_reading_seconds"
	// BurnDurationDefault is the default burn duration when not specified.
	BurnDurationDefault = BurnDuration30Sec
)

func (m *msgServer) GetConversationsHasReadAndMaxSeq(ctx context.Context, req *msg.GetConversationsHasReadAndMaxSeqReq) (*msg.GetConversationsHasReadAndMaxSeqResp, error) {
	if err := authverify.CheckAccess(ctx, req.UserID); err != nil {
		return nil, err
	}
	var conversationIDs []string
	if len(req.ConversationIDs) == 0 {
		var err error
		conversationIDs, err = m.ConversationLocalCache.GetConversationIDs(ctx, req.UserID)
		if err != nil {
			return nil, err
		}
	} else {
		conversationIDs = req.ConversationIDs
	}

	hasReadSeqs, err := m.MsgDatabase.GetHasReadSeqs(ctx, req.UserID, conversationIDs)
	if err != nil {
		return nil, err
	}

	conversations, err := m.ConversationLocalCache.GetConversations(ctx, req.UserID, conversationIDs)
	if err != nil {
		return nil, err
	}

	conversationMaxSeqMap := make(map[string]int64)
	for _, conversation := range conversations {
		if conversation.MaxSeq != 0 {
			conversationMaxSeqMap[conversation.ConversationID] = conversation.MaxSeq
		}
	}
	maxSeqs, err := m.MsgDatabase.GetMaxSeqsWithTime(ctx, conversationIDs)
	if err != nil {
		return nil, err
	}
	resp := &msg.GetConversationsHasReadAndMaxSeqResp{Seqs: make(map[string]*msg.Seqs)}
	if req.ReturnPinned {
		pinnedConversationIDs, err := m.ConversationLocalCache.GetPinnedConversationIDs(ctx, req.UserID)
		if err != nil {
			return nil, err
		}
		resp.PinnedConversationIDs = pinnedConversationIDs
	}
	for conversationID, maxSeq := range maxSeqs {
		resp.Seqs[conversationID] = &msg.Seqs{
			HasReadSeq: hasReadSeqs[conversationID],
			MaxSeq:     maxSeq.Seq,
			MaxSeqTime: maxSeq.Time,
		}
		if v, ok := conversationMaxSeqMap[conversationID]; ok {
			resp.Seqs[conversationID].MaxSeq = v
		}
	}
	return resp, nil
}

func (m *msgServer) SetConversationHasReadSeq(ctx context.Context, req *msg.SetConversationHasReadSeqReq) (*msg.SetConversationHasReadSeqResp, error) {
	if err := authverify.CheckAccess(ctx, req.UserID); err != nil {
		return nil, err
	}
	maxSeq, err := m.MsgDatabase.GetMaxSeq(ctx, req.ConversationID)
	if err != nil {
		return nil, err
	}
	if req.HasReadSeq > maxSeq {
		return nil, errs.ErrArgs.WrapMsg("hasReadSeq must not be bigger than maxSeq")
	}
	if err := m.MsgDatabase.SetHasReadSeq(ctx, req.UserID, req.ConversationID, req.HasReadSeq); err != nil {
		return nil, err
	}
	m.sendMarkAsReadNotification(ctx, req.ConversationID, constant.SingleChatType, req.UserID, req.UserID, nil, req.HasReadSeq)
	return &msg.SetConversationHasReadSeqResp{}, nil
}

func (m *msgServer) MarkMsgsAsRead(ctx context.Context, req *msg.MarkMsgsAsReadReq) (*msg.MarkMsgsAsReadResp, error) {
	if err := authverify.CheckAccess(ctx, req.UserID); err != nil {
		return nil, err
	}
	maxSeq, err := m.MsgDatabase.GetMaxSeq(ctx, req.ConversationID)
	if err != nil {
		return nil, err
	}
	hasReadSeq := req.Seqs[len(req.Seqs)-1]
	if hasReadSeq > maxSeq {
		return nil, errs.ErrArgs.WrapMsg("hasReadSeq must not be bigger than maxSeq")
	}
	conversation, err := m.ConversationLocalCache.GetConversation(ctx, req.UserID, req.ConversationID)
	if err != nil {
		return nil, err
	}
	if err := m.MsgDatabase.MarkSingleChatMsgsAsRead(ctx, req.UserID, req.ConversationID, req.Seqs); err != nil {
		return nil, err
	}
	currentHasReadSeq, err := m.MsgDatabase.GetHasReadSeq(ctx, req.UserID, req.ConversationID)
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}
	if hasReadSeq > currentHasReadSeq {
		err = m.MsgDatabase.SetHasReadSeq(ctx, req.UserID, req.ConversationID, hasReadSeq)
		if err != nil {
			return nil, err
		}
	}

	// Check for burn-after-reading and start burn timer
	go m.checkAndStartBurnTimer(ctx, req.UserID, req.ConversationID, req.Seqs)

	reqCallback := &cbapi.CallbackSingleMsgReadReq{
		ConversationID: conversation.ConversationID,
		UserID:         req.UserID,
		Seqs:           req.Seqs,
		ContentType:    conversation.ConversationType,
	}
	m.webhookAfterSingleMsgRead(ctx, &m.config.WebhooksConfig.AfterSingleMsgRead, reqCallback)
	m.sendMarkAsReadNotification(ctx, req.ConversationID, conversation.ConversationType, req.UserID,
		m.conversationAndGetRecvID(conversation, req.UserID), req.Seqs, hasReadSeq)
	return &msg.MarkMsgsAsReadResp{}, nil
}

func (m *msgServer) MarkConversationAsRead(ctx context.Context, req *msg.MarkConversationAsReadReq) (*msg.MarkConversationAsReadResp, error) {
	if err := authverify.CheckAccess(ctx, req.UserID); err != nil {
		return nil, err
	}
	conversation, err := m.ConversationLocalCache.GetConversation(ctx, req.UserID, req.ConversationID)
	if err != nil {
		return nil, err
	}
	hasReadSeq, err := m.MsgDatabase.GetHasReadSeq(ctx, req.UserID, req.ConversationID)
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}
	var seqs []int64

	log.ZDebug(ctx, "MarkConversationAsRead", "hasReadSeq", hasReadSeq, "req.HasReadSeq", req.HasReadSeq)
	if conversation.ConversationType == constant.SingleChatType {
		for i := hasReadSeq + 1; i <= req.HasReadSeq; i++ {
			seqs = append(seqs, i)
		}
		// avoid client missed call MarkConversationMessageAsRead by order
		for _, val := range req.Seqs {
			if !datautil.Contain(val, seqs...) {
				seqs = append(seqs, val)
			}
		}
		if len(seqs) > 0 {
			log.ZDebug(ctx, "MarkConversationAsRead", "seqs", seqs, "conversationID", req.ConversationID)
			if err = m.MsgDatabase.MarkSingleChatMsgsAsRead(ctx, req.UserID, req.ConversationID, seqs); err != nil {
				return nil, err
			}
		}
		if req.HasReadSeq > hasReadSeq {
			err = m.MsgDatabase.SetHasReadSeq(ctx, req.UserID, req.ConversationID, req.HasReadSeq)
			if err != nil {
				return nil, err
			}
			hasReadSeq = req.HasReadSeq
		}
		m.sendMarkAsReadNotification(ctx, req.ConversationID, conversation.ConversationType, req.UserID,
			m.conversationAndGetRecvID(conversation, req.UserID), seqs, hasReadSeq)
	} else if conversation.ConversationType == constant.ReadGroupChatType ||
		conversation.ConversationType == constant.NotificationChatType {
		if req.HasReadSeq > hasReadSeq {
			err = m.MsgDatabase.SetHasReadSeq(ctx, req.UserID, req.ConversationID, req.HasReadSeq)
			if err != nil {
				return nil, err
			}
			hasReadSeq = req.HasReadSeq
		}
		m.sendMarkAsReadNotification(ctx, req.ConversationID, constant.SingleChatType, req.UserID,
			req.UserID, seqs, hasReadSeq)
	}

	if conversation.ConversationType == constant.SingleChatType {
		reqCall := &cbapi.CallbackSingleMsgReadReq{
			ConversationID: conversation.ConversationID,
			UserID:         conversation.OwnerUserID,
			Seqs:           req.Seqs,
			ContentType:    conversation.ConversationType,
		}
		m.webhookAfterSingleMsgRead(ctx, &m.config.WebhooksConfig.AfterSingleMsgRead, reqCall)
	} else if conversation.ConversationType == constant.ReadGroupChatType {
		reqCall := &cbapi.CallbackGroupMsgReadReq{
			SendID:       conversation.OwnerUserID,
			ReceiveID:    req.UserID,
			UnreadMsgNum: req.HasReadSeq,
			ContentType:  int64(conversation.ConversationType),
		}
		m.webhookAfterGroupMsgRead(ctx, &m.config.WebhooksConfig.AfterGroupMsgRead, reqCall)
	}
	return &msg.MarkConversationAsReadResp{}, nil
}

func (m *msgServer) sendMarkAsReadNotification(ctx context.Context, conversationID string, sessionType int32, sendID, recvID string, seqs []int64, hasReadSeq int64) {
	tips := &sdkws.MarkAsReadTips{
		MarkAsReadUserID: sendID,
		ConversationID:   conversationID,
		Seqs:             seqs,
		HasReadSeq:       hasReadSeq,
	}
	m.notificationSender.NotificationWithSessionType(ctx, sendID, recvID, constant.HasReadReceipt, sessionType, tips)
}

// checkAndStartBurnTimer checks if messages should be burned and starts the burn timer.
func (m *msgServer) checkAndStartBurnTimer(ctx context.Context, userID, conversationID string, seqs []int64) {
	defer func() {
		if r := recover(); r != nil {
			log.ZPanic(ctx, "checkAndStartBurnTimer panic", errs.ErrPanic(r))
		}
	}()

	// Only process single chat messages for burn-after-reading
	conversationPB, err := m.ConversationLocalCache.GetConversation(ctx, userID, conversationID)
	if err != nil {
		log.ZError(ctx, "checkAndStartBurnTimer GetConversation failed", err, "conversationID", conversationID)
		return
	}

	if conversationPB.ConversationType != constant.SingleChatType {
		return
	}

	// Get the conversation's burn-after-reading setting from Ex field (JSON)
	burnEnabled, burnDuration := parseBurnConfigFromEx(conversationPB.Ex)
	if burnEnabled || burnDuration > 0 {
		if burnDuration <= 0 {
			burnDuration = int64(BurnDurationDefault)
		}
		if err := m.StartBurnTimer(ctx, conversationID, userID, seqs, burnDuration); err != nil {
			log.ZError(ctx, "checkAndStartBurnTimer StartBurnTimer failed", err,
				"conversationID", conversationID, "userID", userID, "seqs", seqs)
		}
		return
	}

	// Get message details to check individual message burn settings
	_, _, msgs, err := m.MsgDatabase.GetMsgBySeqs(ctx, userID, conversationID, seqs)
	if err != nil {
		log.ZError(ctx, "checkAndStartBurnTimer GetMsgBySeqs failed", err,
			"conversationID", conversationID, "userID", userID)
		return
	}

	for _, msgData := range msgs {
		if msgData == nil {
			continue
		}

		// Check if this message has burn-after-reading enabled
		burnDuration := getBurnDurationFromOptions(msgData.Options)
		if burnDuration > 0 {
			if err := m.StartBurnTimer(ctx, conversationID, userID, []int64{msgData.Seq}, burnDuration); err != nil {
				log.ZError(ctx, "checkAndStartBurnTimer StartBurnTimer failed", err,
					"conversationID", conversationID, "seq", msgData.Seq)
			}
		}
	}
}

// getBurnDurationFromOptions extracts burn duration from message options.
// Returns 0 if burn-after-reading is not enabled.
func getBurnDurationFromOptions(options map[string]bool) int64 {
	if options == nil {
		return 0
	}

	// Check if burn_after_reading option is enabled
	if enabled, ok := options[BurnAfterReadingOptionKey]; !ok || !enabled {
		return 0
	}

	// Return the burn duration if specified, otherwise use default
	// Note: The actual duration value would come from a separate field or Ex field
	return int64(BurnDurationDefault)
}

// parseBurnConfigFromEx parses burn-after-reading configuration from the conversation's Ex field.
// Returns (burnEnabled, burnDurationSeconds).
func parseBurnConfigFromEx(ex string) (bool, int64) {
	if ex == "" {
		return false, 0
	}
	var config ConversationExConfig
	if err := json.Unmarshal([]byte(ex), &config); err != nil {
		return false, 0
	}
	return config.BurnAfterReading, int64(config.BurnAfterReadingSeconds)
}

// formatBurnConfigToEx formats burn-after-reading configuration into JSON for Ex field.
func formatBurnConfigToEx(enabled bool, seconds int) string {
	config := ConversationExConfig{
		BurnAfterReading:        enabled,
		BurnAfterReadingSeconds: seconds,
	}
	data, _ := json.Marshal(config)
	return string(data)
}

// GetConversationBurnConfig retrieves the burn configuration for a conversation.
func (m *msgServer) GetConversationBurnConfig(ctx context.Context, userID, conversationID string) (bool, int64, error) {
	conversation, err := m.ConversationLocalCache.GetConversation(ctx, userID, conversationID)
	if err != nil {
		return false, 0, err
	}
	enabled, duration := parseBurnConfigFromEx(conversation.Ex)
	return enabled, duration, nil
}

// SetConversationBurnConfig sets the burn-after-reading configuration for a conversation.
func (m *msgServer) SetConversationBurnConfig(ctx context.Context, conversationID string, enabled bool, seconds int64) error {
	if seconds > 0 && !IsValidBurnDuration(seconds) {
		return errs.ErrArgs.WrapMsg("invalid burn duration")
	}

	// This would typically call the conversation client to update the conversation settings
	// For now, this is a placeholder that would be implemented with conversation service integration
	log.ZInfo(ctx, "SetConversationBurnConfig", "conversationID", conversationID, "enabled", enabled, "seconds", seconds)
	return nil
}
