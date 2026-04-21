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

// Package conversation defines the retention extension types for conversation service
package conversation

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
