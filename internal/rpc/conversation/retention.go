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

package conversation

import (
	"context"
	"encoding/json"
	"time"

	"github.com/openimsdk/open-im-server/v3/pkg/authverify"
	"github.com/openimsdk/open-im-server/v3/pkg/common/servererrs"
	"github.com/openimsdk/protocol/constant"
	"github.com/openimsdk/tools/errs"
	"github.com/openimsdk/tools/log"
)

// RetentionDaysInfo 存储在会话扩展字段中的保留天数信息
type RetentionDaysInfo struct {
	RetentionDays int64  `json:"retention_days"`
	Source        string `json:"source"` // "user_setting" 或 "admin_setting"
	SetBy         string `json:"set_by"`
	SetTime       int64  `json:"set_time"`
}

// SetConversationRetentionDaysReq 设置会话保留天数请求
type SetConversationRetentionDaysReq struct {
	ConversationID string `json:"conversationID"`
	OwnerUserID    string `json:"ownerUserID"`
	RetentionDays  int64  `json:"retentionDays"`
}

// SetConversationRetentionDaysResp 设置会话保留天数响应
type SetConversationRetentionDaysResp struct{}

// GetConversationRetentionDaysReq 获取会话保留天数请求
type GetConversationRetentionDaysReq struct {
	ConversationID string `json:"conversationID"`
	OwnerUserID    string `json:"ownerUserID"`
}

// GetConversationRetentionDaysResp 获取会话保留天数响应
type GetConversationRetentionDaysResp struct {
	ConversationID   string `json:"conversationID"`
	RetentionDays    int64  `json:"retentionDays"`
	HasCustomSetting bool   `json:"hasCustomSetting"`
	SetBy            string `json:"setBy"`
	SetTime          int64  `json:"setTime"`
}

// AdminSetConversationRetentionDaysReq 管理员设置会话保留天数请求
type AdminSetConversationRetentionDaysReq struct {
	ConversationID string `json:"conversationID"`
	RetentionDays  int64  `json:"retentionDays"`
}

// AdminSetConversationRetentionDaysResp 管理员设置会话保留天数响应
type AdminSetConversationRetentionDaysResp struct{}

// GetConversationsRetentionReq 获取多个会话保留天数请求
type GetConversationsRetentionReq struct {
	OwnerUserID     string   `json:"ownerUserID"`
	ConversationIDs []string `json:"conversationIDs"`
}

// ConversationRetention 会话保留信息
type ConversationRetention struct {
	ConversationID   string `json:"conversationID"`
	RetentionDays    int64  `json:"retentionDays"`
	HasCustomSetting bool   `json:"hasCustomSetting"`
	ConversationType int32  `json:"conversationType"`
}

// GetConversationsRetentionResp 获取多个会话保留天数响应
type GetConversationsRetentionResp struct {
	Retentions []*ConversationRetention `json:"retentions"`
}

// SetConversationRetentionDays 设置会话保留天数
func (c *conversationServer) SetConversationRetentionDays(ctx context.Context, req *SetConversationRetentionDaysReq) (*SetConversationRetentionDaysResp, error) {
	// 验证权限
	if err := authverify.CheckAccess(ctx, req.OwnerUserID); err != nil {
		return nil, err
	}

	// 验证保留天数
	if err := validateRetentionDays(req.RetentionDays); err != nil {
		return nil, err
	}

	// 获取现有会话
	conversations, err := c.conversationDatabase.FindConversations(ctx, req.OwnerUserID, []string{req.ConversationID})
	if err != nil {
		return nil, err
	}
	if len(conversations) == 0 {
		return nil, servererrs.ErrRecordNotFound.WrapMsg("conversation not found")
	}

	// 创建保留天数信息
	retentionInfo := RetentionDaysInfo{
		RetentionDays: req.RetentionDays,
		Source:        "user_setting",
		SetBy:         req.OwnerUserID,
		SetTime:       time.Now().UnixMilli(),
	}

	// 序列化为 JSON
	exMap := make(map[string]interface{})
	if conversations[0].Ex != "" {
		if err := json.Unmarshal([]byte(conversations[0].Ex), &exMap); err != nil {
			log.ZWarn(ctx, "failed to unmarshal existing ex", err, "ex", conversations[0].Ex)
		}
	}
	exMap["retention_days_info"] = retentionInfo

	exJSON, err := json.Marshal(exMap)
	if err != nil {
		return nil, errs.ErrInternalServer.WrapMsg("failed to marshal ex")
	}

	// 更新会话
	updateFields := map[string]any{
		"ex": string(exJSON),
	}

	if err := c.conversationDatabase.UpdateUsersConversationField(ctx, []string{req.OwnerUserID}, req.ConversationID, updateFields); err != nil {
		return nil, err
	}

	log.ZDebug(ctx, "SetConversationRetentionDays success",
		"conversationID", req.ConversationID,
		"ownerUserID", req.OwnerUserID,
		"retentionDays", req.RetentionDays)

	return &SetConversationRetentionDaysResp{}, nil
}

// GetConversationRetentionDays 获取会话保留天数
func (c *conversationServer) GetConversationRetentionDays(ctx context.Context, req *GetConversationRetentionDaysReq) (*GetConversationRetentionDaysResp, error) {
	// 验证权限
	if err := authverify.CheckAccess(ctx, req.OwnerUserID); err != nil {
		return nil, err
	}

	// 获取会话
	conversations, err := c.conversationDatabase.FindConversations(ctx, req.OwnerUserID, []string{req.ConversationID})
	if err != nil {
		return nil, err
	}
	if len(conversations) == 0 {
		return nil, servererrs.ErrRecordNotFound.WrapMsg("conversation not found")
	}

	conv := conversations[0]

	// 解析保留天数信息
	var retentionInfo RetentionDaysInfo
	if conv.Ex != "" {
		exMap := make(map[string]interface{})
		if err := json.Unmarshal([]byte(conv.Ex), &exMap); err != nil {
			log.ZWarn(ctx, "failed to unmarshal ex", err, "ex", conv.Ex)
		}
		if info, ok := exMap["retention_days_info"].(map[string]interface{}); ok {
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
		}
	}

	// 优先使用会话配置的保留天数，否则使用默认值
	retentionDays := retentionInfo.RetentionDays
	if retentionDays == 0 {
		// 使用默认值
		if conv.ConversationType == constant.SingleChatType {
			retentionDays = getDefaultRetentionDays("single_chat")
		} else {
			retentionDays = getDefaultRetentionDays("group_chat")
		}
	}

	return &GetConversationRetentionDaysResp{
		ConversationID:   req.ConversationID,
		RetentionDays:    retentionDays,
		HasCustomSetting: retentionInfo.RetentionDays > 0,
		SetBy:            retentionInfo.SetBy,
		SetTime:          retentionInfo.SetTime,
	}, nil
}

// AdminSetConversationRetentionDays 管理员设置会话保留天数
func (c *conversationServer) AdminSetConversationRetentionDays(ctx context.Context, req *AdminSetConversationRetentionDaysReq) (*AdminSetConversationRetentionDaysResp, error) {
	// 验证管理员权限
	if err := authverify.CheckAdmin(ctx); err != nil {
		return nil, err
	}

	// 获取会话的所有者
	conversations, err := c.conversationDatabase.FindConversations(ctx, "", []string{req.ConversationID})
	if err != nil {
		return nil, err
	}
	if len(conversations) == 0 {
		return nil, servererrs.ErrRecordNotFound.WrapMsg("conversation not found")
	}

	// 获取会话的所有用户
	ownerUserID := conversations[0].OwnerUserID

	// 创建保留天数信息
	retentionInfo := RetentionDaysInfo{
		RetentionDays: req.RetentionDays,
		Source:        "admin_setting",
		SetBy:         "admin",
		SetTime:       time.Now().UnixMilli(),
	}

	// 更新所有用户的会话
	exMap := make(map[string]interface{})
	if conversations[0].Ex != "" {
		if err := json.Unmarshal([]byte(conversations[0].Ex), &exMap); err != nil {
			log.ZWarn(ctx, "failed to unmarshal existing ex", err, "ex", conversations[0].Ex)
		}
	}
	exMap["retention_days_info"] = retentionInfo

	exJSON, err := json.Marshal(exMap)
	if err != nil {
		return nil, errs.ErrInternalServer.WrapMsg("failed to marshal ex")
	}

	updateFields := map[string]any{
		"ex": string(exJSON),
	}

	if err := c.conversationDatabase.UpdateUsersConversationField(ctx, []string{ownerUserID}, req.ConversationID, updateFields); err != nil {
		return nil, err
	}

	log.ZDebug(ctx, "AdminSetConversationRetentionDays success",
		"conversationID", req.ConversationID,
		"retentionDays", req.RetentionDays)

	return &AdminSetConversationRetentionDaysResp{}, nil
}

// GetConversationsRetention 获取多个会话的保留天数
func (c *conversationServer) GetConversationsRetention(ctx context.Context, req *GetConversationsRetentionReq) (*GetConversationsRetentionResp, error) {
	if err := authverify.CheckAccess(ctx, req.OwnerUserID); err != nil {
		return nil, err
	}

	conversations, err := c.conversationDatabase.FindConversations(ctx, req.OwnerUserID, req.ConversationIDs)
	if err != nil {
		return nil, err
	}

	retentions := make([]*ConversationRetention, 0, len(conversations))
	for _, conv := range conversations {
		var retentionDays int64
		var hasCustomSetting bool

		if conv.Ex != "" {
			exMap := make(map[string]interface{})
			if err := json.Unmarshal([]byte(conv.Ex), &exMap); err == nil {
				if info, ok := exMap["retention_days_info"].(map[string]interface{}); ok {
					if days, ok := info["retention_days"].(float64); ok {
						retentionDays = int64(days)
						hasCustomSetting = retentionDays > 0
					}
				}
			}
		}

		if retentionDays == 0 {
			if conv.ConversationType == constant.SingleChatType {
				retentionDays = getDefaultRetentionDays("single_chat")
			} else {
				retentionDays = getDefaultRetentionDays("group_chat")
			}
		}

		retentions = append(retentions, &ConversationRetention{
			ConversationID:   conv.ConversationID,
			RetentionDays:    retentionDays,
			HasCustomSetting: hasCustomSetting,
			ConversationType: conv.ConversationType,
		})
	}

	return &GetConversationsRetentionResp{
		Retentions: retentions,
	}, nil
}

// validateRetentionDays 验证保留天数
func validateRetentionDays(days int64) error {
	if days < 0 {
		return errs.ErrArgs.WrapMsg("retention days cannot be negative")
	}
	// 0 表示使用默认配置，永久保留
	if days == 0 {
		return nil
	}
	// 检查是否在允许的档位内
	validOptions := []int64{7, 30, 90, 180, 365}
	for _, opt := range validOptions {
		if days == opt {
			return nil
		}
	}
	return errs.ErrArgs.WrapMsg("invalid retention days, must be one of 7, 30, 90, 180, 365, or 0")
}

// getDefaultRetentionDays 获取默认保留天数
func getDefaultRetentionDays(chatType string) int64 {
	// 默认值：单聊1年，群聊180天
	switch chatType {
	case "single_chat":
		return 365
	case "group_chat":
		return 180
	default:
		return 365
	}
}
