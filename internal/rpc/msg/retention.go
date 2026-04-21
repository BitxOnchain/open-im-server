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
// WITHOUT WARRANTIES OF CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package msg

import (
	"context"
	"encoding/json"
	"time"

	"github.com/openimsdk/open-im-server/v3/pkg/common/config"
	"github.com/openimsdk/open-im-server/v3/pkg/common/servererrs"
	"github.com/openimsdk/protocol/constant"
	pbconversation "github.com/openimsdk/protocol/conversation"
	pbmsg "github.com/openimsdk/protocol/msg"
	"github.com/openimsdk/protocol/wrapperspb"
	"github.com/openimsdk/tools/errs"
	"github.com/openimsdk/tools/log"
)

// RetentionDaysInfo 保留天数信息结构
type RetentionDaysInfo struct {
	RetentionDays int64  `json:"retention_days"`
	Source        string `json:"source"`
	SetBy         string `json:"set_by"`
	SetTime       int64  `json:"set_time"`
}

// GetRetentionConfigReq 获取保留配置请求
type GetRetentionConfigReq struct{}

// GetRetentionConfigResp 获取保留配置响应
type GetRetentionConfigResp struct {
	DefaultRetentionDays int64 `json:"defaultRetentionDays"`
	MinRetentionDays     int64 `json:"minRetentionDays"`
	MaxRetentionDays     int64 `json:"maxRetentionDays"`
	SingleChatRetention  int64 `json:"singleChatRetention"`
	GroupChatRetention   int64 `json:"groupChatRetention"`
}

// SetConversationRetentionReq 设置会话保留天数请求
type SetConversationRetentionReq struct {
	ConversationID string `json:"conversationID"`
	OwnerUserID    string `json:"ownerUserID"`
	RetentionDays  int64  `json:"retentionDays"`
}

// SetConversationRetentionResp 设置会话保留天数响应
type SetConversationRetentionResp struct{}

// GetConversationRetentionReq 获取会话保留天数请求
type GetConversationRetentionReq struct {
	ConversationID string `json:"conversationID"`
	OwnerUserID    string `json:"ownerUserID"`
}

// GetConversationRetentionResp 获取会话保留天数响应
type GetConversationRetentionResp struct {
	ConversationID   string `json:"conversationID"`
	RetentionDays    int64  `json:"retentionDays"`
	SourceRetention  string `json:"sourceRetention"`
	ConversationType int32  `json:"conversationType"`
}

// DestructMsgsByRetentionReq 按保留时间清理消息请求
type DestructMsgsByRetentionReq struct {
	ConversationID string `json:"conversationID"`
	OwnerUserID    string `json:"ownerUserID"`
	RetentionDays  int64  `json:"retentionDays"`
	Limit          int64  `json:"limit"`
}

// DestructMsgsByRetentionResp 按保留时间清理消息响应
type DestructMsgsByRetentionResp struct {
	Count   int64 `json:"count"`
	Skipped int64 `json:"skipped"`
	DelTime int64 `json:"delTime"`
}

// GetRetentionOptionsReq 获取保留天数选项请求
type GetRetentionOptionsReq struct{}

// GetRetentionOptionsResp 获取保留天数选项响应
type GetRetentionOptionsResp struct {
	Options []int64 `json:"options"`
}

// GetRetentionConfig 获取当前保留配置
func (m *msgServer) GetRetentionConfig(ctx context.Context, req *GetRetentionConfigReq) (*GetRetentionConfigResp, error) {
	if m.config.CronTask.Retention.DefaultRetentionDays <= 0 {
		return nil, errs.ErrArgs.WrapMsg("retention not enabled")
	}
	return &GetRetentionConfigResp{
		DefaultRetentionDays: m.config.CronTask.Retention.DefaultRetentionDays,
		MinRetentionDays:     m.config.CronTask.Retention.MinRetentionDays,
		MaxRetentionDays:     m.config.CronTask.Retention.MaxRetentionDays,
		SingleChatRetention:  m.config.CronTask.Retention.SingleChatRetention,
		GroupChatRetention:   m.config.CronTask.Retention.GroupChatRetention,
	}, nil
}

// SetConversationRetention 设置会话保留天数
func (m *msgServer) SetConversationRetention(ctx context.Context, req *SetConversationRetentionReq) (*SetConversationRetentionResp, error) {
	if err := m.validateRetentionDays(req.RetentionDays); err != nil {
		return nil, err
	}

	// 获取会话信息
	conversations, err := m.ConversationLocalCache.GetConversations(ctx, req.OwnerUserID, []string{req.ConversationID})
	if err != nil {
		return nil, err
	}
	if len(conversations) == 0 {
		return nil, servererrs.ErrRecordNotFound.WrapMsg("conversation not found")
	}

	conv := conversations[0]

	// 构建保留天数信息 JSON
	retentionInfo := RetentionDaysInfo{
		RetentionDays: req.RetentionDays,
		Source:        "msg_service",
		SetBy:         req.OwnerUserID,
		SetTime:       time.Now().UnixMilli(),
	}

	// 更新会话的 ex 字段存储保留信息
	exMap := make(map[string]interface{})
	if conv.Ex != "" {
		if err := json.Unmarshal([]byte(conv.Ex), &exMap); err != nil {
			log.ZWarn(ctx, "failed to unmarshal existing ex", err)
		}
	}
	exMap["retention_days_info"] = retentionInfo

	exJSON, err := json.Marshal(exMap)
	if err != nil {
		return nil, errs.ErrInternalServer.WrapMsg("failed to marshal ex")
	}

	// 调用 conversation 服务更新
	_, err = m.conversationClient.UpdateConversation(ctx, &pbconversation.UpdateConversationReq{
		UserIDs:        []string{req.OwnerUserID},
		ConversationID: req.ConversationID,
		Ex:             wrapperspb.String(string(exJSON)),
	})
	if err != nil {
		return nil, err
	}

	log.ZDebug(ctx, "SetConversationRetention success", "conversationID", req.ConversationID, "retentionDays", req.RetentionDays, "ownerUserID", req.OwnerUserID, "conversationType", conv.ConversationType)
	return &SetConversationRetentionResp{}, nil
}

// GetConversationRetention 获取会话保留天数
func (m *msgServer) GetConversationRetention(ctx context.Context, req *GetConversationRetentionReq) (*GetConversationRetentionResp, error) {
	conversations, err := m.ConversationLocalCache.GetConversations(ctx, req.OwnerUserID, []string{req.ConversationID})
	if err != nil {
		return nil, err
	}
	if len(conversations) == 0 {
		return nil, servererrs.ErrRecordNotFound.WrapMsg("conversation not found")
	}

	conv := conversations[0]

	// 解析保留天数信息
	retentionInfo := m.parseRetentionInfo(conv.Ex)

	// 确定使用的保留天数
	retentionDays := m.getEffectiveRetentionDays(conv, retentionInfo)

	return &GetConversationRetentionResp{
		ConversationID:   req.ConversationID,
		RetentionDays:    retentionDays,
		SourceRetention:  m.getRetentionDaysSource(conv, retentionInfo),
		ConversationType: conv.ConversationType,
	}, nil
}

// parseRetentionInfo 解析会话附加信息中的保留天数
func (m *msgServer) parseRetentionInfo(ex string) *RetentionDaysInfo {
	if ex == "" {
		return nil
	}
	exMap := make(map[string]interface{})
	if err := json.Unmarshal([]byte(ex), &exMap); err != nil {
		return nil
	}
	if info, ok := exMap["retention_days_info"].(map[string]interface{}); ok {
		retentionInfo := &RetentionDaysInfo{}
		if days, ok := info["retention_days"].(float64); ok {
			retentionInfo.RetentionDays = int64(days)
		}
		if source, ok := info["source"].(string); ok {
			retentionInfo.Source = source
		}
		if setBy, ok := info["set_by"].(string); ok {
			retentionInfo.SetBy = setBy
		}
		if setTime, ok := info["set_time"].(float64); ok {
			retentionInfo.SetTime = int64(setTime)
		}
		return retentionInfo
	}
	return nil
}

// getEffectiveRetentionDays 获取会话的有效保留天数
func (m *msgServer) getEffectiveRetentionDays(conv *pbconversation.Conversation, retentionInfo *RetentionDaysInfo) int64 {
	// 1. 首先检查会话级别覆盖配置
	if override, ok := m.config.CronTask.Retention.ConversationOverrides[conv.ConversationID]; ok {
		return override
	}

	// 2. 如果会话设置了自定义保留天数，使用会话设置的
	if retentionInfo != nil && retentionInfo.RetentionDays > 0 {
		return retentionInfo.RetentionDays
	}

	// 3. 根据会话类型使用默认值
	if conv.ConversationType == constant.SingleChatType || conv.ConversationType == constant.NotificationChatType {
		return m.config.CronTask.Retention.SingleChatRetention
	}

	// 4. 群聊使用群聊默认配置
	return m.config.CronTask.Retention.GroupChatRetention
}

// getRetentionDaysSource 获取保留天数的来源
func (m *msgServer) getRetentionDaysSource(conv *pbconversation.Conversation, retentionInfo *RetentionDaysInfo) string {
	if _, ok := m.config.CronTask.Retention.ConversationOverrides[conv.ConversationID]; ok {
		return "config_override"
	}
	if retentionInfo != nil && retentionInfo.RetentionDays > 0 {
		return "conversation_setting"
	}
	if conv.ConversationType == constant.SingleChatType || conv.ConversationType == constant.NotificationChatType {
		return "single_chat_default"
	}
	return "group_chat_default"
}

// validateRetentionDays 验证保留天数是否有效
func (m *msgServer) validateRetentionDays(days int64) error {
	if days < 0 {
		return errs.ErrArgs.WrapMsg("retention days cannot be negative")
	}
	if days == 0 {
		return nil // 0 表示使用默认配置
	}
	if days < m.config.CronTask.Retention.MinRetentionDays {
		return errs.ErrArgs.WrapMsg("retention days below minimum", "min", m.config.CronTask.Retention.MinRetentionDays)
	}
	if m.config.CronTask.Retention.MaxRetentionDays > 0 && days > m.config.CronTask.Retention.MaxRetentionDays {
		return errs.ErrArgs.WrapMsg("retention days exceeds maximum", "max", m.config.CronTask.Retention.MaxRetentionDays)
	}
	return nil
}

// DestructMsgsByRetention 按保留时间清理消息（支持按会话配置）
// 注意：此函数通过调用 DestructMsgs API 来实现，该 API 会清理早于指定时间的消息
func (m *msgServer) DestructMsgsByRetention(ctx context.Context, req *DestructMsgsByRetentionReq) (*DestructMsgsByRetentionResp, error) {
	now := time.Now()

	// 计算清理时间点
	var delTime time.Time
	if req.RetentionDays > 0 {
		// 使用指定的保留天数
		delTime = now.AddDate(0, 0, -int(req.RetentionDays))
	} else if req.ConversationID != "" && req.OwnerUserID != "" {
		// 获取会话配置的保留天数
		conversations, err := m.ConversationLocalCache.GetConversations(ctx, req.OwnerUserID, []string{req.ConversationID})
		if err != nil {
			return nil, err
		}
		if len(conversations) == 0 {
			return nil, servererrs.ErrRecordNotFound.WrapMsg("conversation not found")
		}
		retentionInfo := m.parseRetentionInfo(conversations[0].Ex)
		retentionDays := m.getEffectiveRetentionDays(conversations[0], retentionInfo)
		if retentionDays == 0 {
			// 永久保留，不清理
			return &DestructMsgsByRetentionResp{Count: 0, Skipped: 0}, nil
		}
		delTime = now.AddDate(0, 0, -int(retentionDays))
	} else {
		// 使用全局默认配置
		if m.config.CronTask.Retention.DefaultRetentionDays == 0 {
			return &DestructMsgsByRetentionResp{Count: 0, Skipped: 0}, nil
		}
		delTime = now.AddDate(0, 0, -int(m.config.CronTask.Retention.DefaultRetentionDays))
	}

	// 执行分批删除
	const deleteLimit = 50
	var totalDeleted int64
	var skipped int64

	limit := req.Limit
	if limit <= 0 {
		limit = deleteLimit
	}

	// 调用 DestructMsgs API
	for i := 1; ; i++ {
		resp, err := m.DestructMsgs(ctx, &pbmsg.DestructMsgsReq{
			Timestamp: delTime.UnixMilli(),
			Limit:     int32(limit),
		})
		if err != nil {
			log.ZError(ctx, "DestructMsgsByRetention failed", err, "delTime", delTime, "round", i)
			break
		}

		if resp.Count == 0 {
			break
		}

		totalDeleted += int64(resp.Count)
		skipped += int64(resp.Count)

		if resp.Count < int32(limit) {
			break
		}

		// 防止无限循环
		if i >= 1000 {
			log.ZWarn(ctx, "DestructMsgsByRetention reached max iterations", nil, "totalDeleted", totalDeleted)
			break
		}
	}

	log.ZDebug(ctx, "DestructMsgsByRetention completed",
		"delTime", delTime,
		"totalDeleted", totalDeleted,
		"skipped", skipped,
		"duration", time.Since(now))

	return &DestructMsgsByRetentionResp{
		Count:   totalDeleted,
		Skipped: skipped,
		DelTime: delTime.UnixMilli(),
	}, nil
}

// GetRetentionOptions 获取可用的保留天数选项
func (m *msgServer) GetRetentionOptions(ctx context.Context, req *GetRetentionOptionsReq) (*GetRetentionOptionsResp, error) {
	return &GetRetentionOptionsResp{
		Options: config.RetentionOptions,
	}, nil
}
