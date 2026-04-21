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

// Package msg defines the retention extension types for msg service
package msg

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
