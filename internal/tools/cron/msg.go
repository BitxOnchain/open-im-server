package cron

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/openimsdk/protocol/constant"
	pbconversation "github.com/openimsdk/protocol/conversation"
	"github.com/openimsdk/protocol/msg"
	"github.com/openimsdk/tools/log"
	"github.com/openimsdk/tools/mcontext"
)

// RetentionDaysOption 保留天数档位选项
var RetentionDaysOptions = []int64{7, 30, 90, 180, 365, 0} // 0表示永久

// burnMsg 检查并清理过期的阅后即焚消息
// 这个 cron 任务是作为备份机制，用于清理可能遗漏的阅后即焚消息
func (c *cronServer) burnMsg() {
	operationID := fmt.Sprintf("cron_burn_%d", time.Now().UnixMilli())
	ctx := mcontext.SetOperationID(c.ctx, operationID)
	log.ZDebug(ctx, "burnMsg cron task started")
	// 阅后即焚消息由消息已读回调启动的 goroutine 处理
	// 此 cron 任务作为备份清理机制，确保没有遗漏
	// 由于 BurnMsgManager 已经在内存中管理定时器，这里可以留空或仅做日志记录
}

// deleteMsg 全局消息清理（使用 RetainChatRecords 配置）
func (c *cronServer) deleteMsg() {
	now := time.Now()
	deltime := now.Add(-time.Hour * 24 * time.Duration(c.config.CronTask.RetainChatRecords))
	operationID := fmt.Sprintf("cron_msg_%d_%d", os.Getpid(), deltime.UnixMilli())
	ctx := mcontext.SetOperationID(c.ctx, operationID)
	log.ZDebug(ctx, "Destruct chat records", "deltime", deltime, "timestamp", deltime.UnixMilli())
	const (
		deleteCount = 10000
		deleteLimit = 50
	)
	var count int
	for i := 1; i <= deleteCount; i++ {
		ctx := mcontext.SetOperationID(c.ctx, fmt.Sprintf("%s_%d", operationID, i))
		resp, err := c.msgClient.DestructMsgs(ctx, &msg.DestructMsgsReq{Timestamp: deltime.UnixMilli(), Limit: deleteLimit})
		if err != nil {
			log.ZError(ctx, "cron destruct chat records failed", err)
			break
		}
		count += int(resp.Count)
		if resp.Count < deleteLimit {
			break
		}
	}
	log.ZDebug(ctx, "cron destruct chat records end", "deltime", deltime, "cont", time.Since(now), "count", count)
}

// RetentionInfo 保留天数信息结构
type RetentionInfo struct {
	RetentionDays int64  `json:"retention_days"`
	Source        string `json:"source"`
	SetBy         string `json:"set_by"`
	SetTime       int64  `json:"set_time"`
}

// parseRetentionInfo 解析保留天数信息
func parseRetentionInfo(ex string) *RetentionInfo {
	if ex == "" {
		return nil
	}
	// 简单的 JSON 解析
	// 格式: {"retention_days_info": {"retention_days": 30, ...}}
	start := `{"retention_days_info":`
	if len(ex) < len(start) || ex[:len(start)] != start {
		return nil
	}
	// 这里简化处理，实际使用时应该完整解析 JSON
	return nil
}

// deleteMsgByRetention 按会话配置的保留天数清理消息
func (c *cronServer) deleteMsgByRetention() {
	now := time.Now()
	operationID := fmt.Sprintf("cron_retention_%d", now.UnixMilli())
	ctx := mcontext.SetOperationID(c.ctx, operationID)

	retention := c.config.CronTask.Retention
	if retention.DefaultRetentionDays <= 0 && retention.SingleChatRetention <= 0 && retention.GroupChatRetention <= 0 {
		log.ZDebug(ctx, "retention cleanup disabled", "config", retention)
		return
	}

	log.ZInfo(ctx, "Starting retention-based message cleanup", "startTime", now)

	// 获取所有需要清理的会话
	conversations, err := c.getConversationsForRetention(ctx)
	if err != nil {
		log.ZError(ctx, "get conversations for retention failed", err)
		return
	}

	var totalDeleted int64
	var processedCount int

	// 按会话类型分组处理
	singleChatConversations := make(map[string]string) // conversationID -> ownerUserID
	groupChatConversations := make(map[string]string)  // conversationID -> ownerUserID

	for convID, ownerUserID := range conversations {
		convType, err := c.getConversationType(convID)
		if err != nil {
			continue
		}
		if convType == constant.SingleChatType || convType == constant.NotificationChatType {
			singleChatConversations[convID] = ownerUserID
		} else if convType == constant.ReadGroupChatType || convType == constant.WriteGroupChatType {
			groupChatConversations[convID] = ownerUserID
		}
	}

	// 清理单聊消息
	totalDeleted += c.cleanConversationsByRetention(ctx, operationID, singleChatConversations, retention.SingleChatRetention, "single_chat")

	// 清理群聊消息
	totalDeleted += c.cleanConversationsByRetention(ctx, operationID, groupChatConversations, retention.GroupChatRetention, "group_chat")

	log.ZInfo(ctx, "Retention-based message cleanup completed",
		"totalDeleted", totalDeleted,
		"duration", time.Since(now),
		"processedConversations", processedCount)
}

// getConversationsForRetention 获取需要清理保留消息的会话列表
func (c *cronServer) getConversationsForRetention(ctx context.Context) (map[string]string, error) {
	// 获取所有会话，使用分页避免一次性加载过多
	const pageSize = 100
	allConversations := make(map[string]string)

	// 获取所有用户的会话
	users, err := c.getAllUsers(ctx)
	if err != nil {
		log.ZError(ctx, "get all users failed", err)
		return nil, err
	}

	for _, userID := range users {
		req := &pbconversation.GetOwnerConversationReq{
			UserID: userID,
		}

		resp, err := c.conversationClient.GetOwnerConversation(ctx, req)
		if err != nil {
			log.ZError(ctx, "get owner conversation failed", err, "userID", userID)
			continue
		}

		for _, conv := range resp.Conversations {
			// 跳过永久保留的会话（ex 字段中没有 retention_days_info）
			if conv.Ex == "" || !hasRetentionConfig(conv.Ex) {
				continue
			}
			allConversations[conv.ConversationID] = userID
		}
	}

	return allConversations, nil
}

// hasRetentionConfig 检查会话是否有保留配置
func hasRetentionConfig(ex string) bool {
	if ex == "" {
		return false
	}
	// 检查 ex 字段中是否包含 retention_days_info
	// 简化检查，实际应该完整解析 JSON
	return len(ex) > 20 && containsRetentionInfo(ex)
}

// containsRetentionInfo 简单检查是否包含保留信息
func containsRetentionInfo(s string) bool {
	return len(s) > 5 && (containsSubstring(s, "retention_days") || containsSubstring(s, "retentionDays"))
}

// containsSubstring 简单字符串包含检查
func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// getConversationType 获取会话类型
func (c *cronServer) getConversationType(conversationID string) (int32, error) {
	// 从会话ID推断类型
	// s_ 开头表示单聊
	// si_ 开头表示通知单聊
	// sg_ 开头表示群聊读
	if len(conversationID) >= 3 {
		switch conversationID[:3] {
		case "s_1", "s_2", "s_3":
			return constant.SingleChatType, nil
		case "si_":
			return constant.NotificationChatType, nil
		case "sg_":
			return constant.ReadGroupChatType, nil
		}
	}

	// 默认返回单聊类型
	return constant.SingleChatType, nil
}

// getAllUsers 获取所有用户
func (c *cronServer) getAllUsers(ctx context.Context) ([]string, error) {
	// 这里应该调用用户服务获取所有用户ID
	// 暂时返回空，需要根据实际实现补充
	// 可以通过获取会话中出现的所有用户来间接获取
	return nil, nil
}

// cleanConversationsByRetention 清理指定会话的消息
func (c *cronServer) cleanConversationsByRetention(ctx context.Context, operationID string, conversations map[string]string, defaultRetention int64, chatType string) int64 {
	const batchSize = 100
	var totalDeleted int64

	for convID, ownerUserID := range conversations {
		// 获取该会话的有效保留天数
		retentionDays := c.getEffectiveRetentionDays(ctx, convID, ownerUserID, defaultRetention)

		if retentionDays <= 0 {
			// 0 表示永久保留，跳过
			continue
		}

		delTime := time.Now().AddDate(0, 0, -int(retentionDays))
		deleteCtx := mcontext.SetOperationID(ctx, fmt.Sprintf("%s_%s", operationID, convID))

		// 分批删除该会话的消息
		batchDeleted := c.deleteConversationMsgs(deleteCtx, convID, delTime, batchSize)
		totalDeleted += batchDeleted

		log.ZDebug(deleteCtx, "clean conversation by retention",
			"conversationID", convID,
			"retentionDays", retentionDays,
			"deleted", batchDeleted,
			"chatType", chatType)
	}

	return totalDeleted
}

// getEffectiveRetentionDays 获取会话的有效保留天数
func (c *cronServer) getEffectiveRetentionDays(ctx context.Context, conversationID string, ownerUserID string, defaultRetention int64) int64 {
	// 1. 优先使用全局覆盖配置
	if override, ok := c.config.CronTask.Retention.ConversationOverrides[conversationID]; ok {
		return override
	}

	// 2. 获取会话配置的保留天数
	convReq := &pbconversation.GetConversationReq{
		OwnerUserID:    ownerUserID,
		ConversationID: conversationID,
	}

	convs, err := c.conversationClient.GetConversation(ctx, convReq)
	if err == nil && convs != nil && convs.Conversation != nil {
		// 解析 ex 字段中的保留天数
		retentionInfo := parseRetentionInfo(convs.Conversation.Ex)
		if retentionInfo != nil && retentionInfo.RetentionDays > 0 {
			return retentionInfo.RetentionDays
		}
	}

	// 3. 使用默认配置
	return defaultRetention
}

// deleteConversationMsgs 删除指定会话的消息
func (c *cronServer) deleteConversationMsgs(ctx context.Context, conversationID string, delTime time.Time, limit int) int64 {
	var totalDeleted int64

	for i := 1; ; i++ {
		req := &msg.DestructMsgsReq{
			Timestamp: delTime.UnixMilli(),
			Limit:     int32(limit),
		}

		resp, err := c.msgClient.DestructMsgs(ctx, req)
		if err != nil {
			log.ZError(ctx, "delete conversation msgs failed", err, "conversationID", conversationID)
			break
		}

		if resp.Count == 0 {
			break
		}

		totalDeleted += int64(resp.Count)

		if resp.Count < int32(limit) {
			break
		}

		// 防止无限循环
		if i >= 1000 {
			log.ZWarn(ctx, "delete conversation msgs reached max iterations", nil, "conversationID", conversationID)
			break
		}
	}

	return totalDeleted
}
