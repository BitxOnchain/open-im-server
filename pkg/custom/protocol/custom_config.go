// Copyright © 2024 OpenIM. All rights reserved.
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

package protocol

import "time"

// ============================================================
// E2E Encryption Configuration (Session Level)
// ============================================================

// E2EEncryptionConfig E2E加密配置（会话级别）
type E2EEncryptionConfig struct {
	Enabled   bool  `json:"enabled" bson:"enabled"`      // 是否启用E2E加密
	KeyIndex  int32 `json:"keyIndex" bson:"key_index"`   // 加密密钥索引
	UpdatedAt int64 `json:"updatedAt" bson:"updated_at"` // 更新时间戳（毫秒）
}

// SetE2EEncryptionReq 设置E2E加密请求
type SetE2EEncryptionReq struct {
	ConversationID string `json:"conversationId" binding:"required"`
	Enabled        bool   `json:"enabled"`
	KeyIndex       int32  `json:"keyIndex"`
}

// SetE2EEncryptionResp 设置E2E加密响应
type SetE2EEncryptionResp struct {
	ErrCode int32                `json:"errCode"`
	ErrMsg  string               `json:"errMsg"`
	Config  *E2EEncryptionConfig `json:"config"`
}

// GetE2EEncryptionReq 获取E2E加密状态请求
type GetE2EEncryptionReq struct {
	ConversationIDs []string `json:"conversationIds" binding:"required"`
}

// GetE2EEncryptionResp 获取E2E加密状态响应
type GetE2EEncryptionResp struct {
	ErrCode int32                           `json:"errCode"`
	ErrMsg  string                          `json:"errMsg"`
	Configs map[string]*E2EEncryptionConfig `json:"configs"`
}

// ============================================================
// Burn After Read (Message Destruct) Configuration
// ============================================================

// MessageBurnConfig 消息阅后即焚配置
type MessageBurnConfig struct {
	IsBurn       bool  `json:"isBurn" bson:"is_burn"`             // 是否启用阅后即焚
	BurnDuration int32 `json:"burnDuration" bson:"burn_duration"` // 销毁时间（秒）
	SetAt        int64 `json:"setAt" bson:"set_at"`               // 设置时间戳（毫秒）
}

// SendBurnMessageReq 发送阅后即焚消息请求
type SendBurnMessageReq struct {
	Msg          interface{} `json:"msg" binding:"required"`
	IsBurn       bool        `json:"isBurn"`
	BurnDuration int32       `json:"burnDuration"`
}

// SendBurnMessageResp 发送阅后即焚消息响应
type SendBurnMessageResp struct {
	ErrCode     int32  `json:"errCode"`
	ErrMsg      string `json:"errMsg"`
	ClientMsgID string `json:"clientMsgId"`
	ServerMsgID string `json:"serverMsgId"`
	SendTime    int64  `json:"sendTime"`
}

// SetConversationBurnReq 设置会话阅后即焚请求
type SetConversationBurnReq struct {
	ConversationID      string `json:"conversationId" binding:"required"`
	IsBurnEnabled       bool   `json:"isBurnEnabled"`
	DefaultBurnDuration int32  `json:"defaultBurnDuration"` // 秒
}

// SetConversationBurnResp 设置会话阅后即焚响应
type SetConversationBurnResp struct {
	ErrCode int32  `json:"errCode"`
	ErrMsg  string `json:"errMsg"`
}

// GetConversationBurnReq 获取会话阅后即焚配置请求
type GetConversationBurnReq struct {
	ConversationIDs []string `json:"conversationIds" binding:"required"`
}

// GetConversationBurnResp 获取会话阅后即焚配置响应
type GetConversationBurnResp struct {
	ErrCode int32                              `json:"errCode"`
	ErrMsg  string                             `json:"errMsg"`
	Configs map[string]*ConversationBurnConfig `json:"configs"`
}

// ConversationBurnConfig 会话级别阅后即焚配置
type ConversationBurnConfig struct {
	IsBurnEnabled       bool  `json:"isBurnEnabled" bson:"is_burn_enabled"`             // 是否默认启用
	DefaultBurnDuration int32 `json:"defaultBurnDuration" bson:"default_burn_duration"` // 默认销毁时间（秒）
	UpdatedAt           int64 `json:"updatedAt" bson:"updated_at"`                      // 更新时间戳
}

// ============================================================
// Message Retention (Auto Cleanup) Configuration
// ============================================================

// RetentionPolicy 全局消息保留策略
type RetentionPolicy struct {
	DefaultRetentionDays int32 `json:"defaultRetentionDays" yaml:"defaultRetentionDays"` // 默认保留天数
	MinRetentionDays     int32 `json:"minRetentionDays" yaml:"minRetentionDays"`         // 最小保留天数
	MaxRetentionDays     int32 `json:"maxRetentionDays" yaml:"maxRetentionDays"`         // 最大保留天数
	Enabled              bool  `json:"enabled" yaml:"enabled"`                           // 是否启用自动清理
}

// SetRetentionReq 设置消息保留期请求
type SetRetentionReq struct {
	ConversationID string `json:"conversationId" binding:"required"`
	RetentionDays  int32  `json:"retentionDays"` // 保留天数，0表示使用全局默认值
}

// SetRetentionResp 设置消息保留期响应
type SetRetentionResp struct {
	ErrCode int32  `json:"errCode"`
	ErrMsg  string `json:"errMsg"`
}

// GetRetentionReq 获取消息保留期请求
type GetRetentionReq struct {
	ConversationIDs []string `json:"conversationIds" binding:"required"`
}

// GetRetentionResp 获取消息保留期响应
type GetRetentionResp struct {
	ErrCode      int32                             `json:"errCode"`
	ErrMsg       string                            `json:"errMsg"`
	Retentions   map[string]*ConversationRetention `json:"retentions"`
	GlobalPolicy *RetentionPolicy                  `json:"globalPolicy"`
}

// ConversationRetention 会话级别消息保留配置
type ConversationRetention struct {
	RetentionDays int32 `json:"retentionDays" bson:"retention_days"` // 保留天数，0表示使用全局默认值
	UpdatedAt     int64 `json:"updatedAt" bson:"updated_at"`         // 更新时间戳
	DeletedUntil  int64 `json:"deletedUntil" bson:"deleted_until"`   // 清理截止时间（时间戳）
}

// TriggerCleanupReq 触发手动清理请求
type TriggerCleanupReq struct {
	ConversationID string `json:"conversationId"` // 空字符串表示所有会话
	Force          bool   `json:"force"`          // 是否强制清理
}

// TriggerCleanupResp 触发手动清理响应
type TriggerCleanupResp struct {
	ErrCode      int32  `json:"errCode"`
	ErrMsg       string `json:"errMsg"`
	DeletedCount int64  `json:"deletedCount"` // 删除的消息数量
}

// ============================================================
// Global Configuration Management
// ============================================================

// SetGlobalRetentionPolicyReq 设置全局保留策略请求
type SetGlobalRetentionPolicyReq struct {
	Policy *RetentionPolicy `json:"policy" binding:"required"`
}

// SetGlobalRetentionPolicyResp 设置全局保留策略响应
type SetGlobalRetentionPolicyResp struct {
	ErrCode int32            `json:"errCode"`
	ErrMsg  string           `json:"errMsg"`
	Policy  *RetentionPolicy `json:"policy"`
}

// GetGlobalRetentionPolicyReq 获取全局保留策略请求
type GetGlobalRetentionPolicyReq struct {
}

// GetGlobalRetentionPolicyResp 获取全局保留策略响应
type GetGlobalRetentionPolicyResp struct {
	ErrCode int32            `json:"errCode"`
	ErrMsg  string           `json:"errMsg"`
	Policy  *RetentionPolicy `json:"policy"`
}

// ============================================================
// MongoDB Storage Models
// ============================================================

// E2EEncryptionModel MongoDB存储模型
type E2EEncryptionModel struct {
	ConversationID string    `bson:"conversation_id"`
	Enabled        bool      `bson:"enabled"`
	KeyIndex       int32     `bson:"key_index"`
	UpdatedAt      time.Time `bson:"updated_at"`
}

// ConversationBurnModel MongoDB存储模型
type ConversationBurnModel struct {
	ConversationID      string    `bson:"conversation_id"`
	IsBurnEnabled       bool      `bson:"is_burn_enabled"`
	DefaultBurnDuration int32     `bson:"default_burn_duration"`
	UpdatedAt           time.Time `bson:"updated_at"`
}

// ConversationRetentionModel MongoDB存储模型
type ConversationRetentionModel struct {
	ConversationID string    `bson:"conversation_id"`
	RetentionDays  int32     `bson:"retention_days"`
	UpdatedAt      time.Time `bson:"updated_at"`
}

// GlobalRetentionModel MongoDB存储模型
type GlobalRetentionModel struct {
	ID        string           `bson:"_id"`
	Policy    *RetentionPolicy `bson:"policy"`
	UpdatedAt time.Time        `bson:"updated_at"`
}
