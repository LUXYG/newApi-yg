package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type securityKeywordsImportRequest struct {
	Items []model.SecurityKeyword `json:"items"`
}

// normalizeSecurityKeywordInput 统一填充默认值并校验危险关键词字段合法性。
func normalizeSecurityKeywordInput(kw *model.SecurityKeyword) error {
	kw.Keyword = strings.TrimSpace(kw.Keyword)
	if kw.Keyword == "" {
		return fmt.Errorf("关键词不能为空")
	}
	if kw.MatchType == "" {
		kw.MatchType = "exact"
	}
	if kw.CheckScope == "" {
		kw.CheckScope = "user_only"
	}
	if kw.Action == "" {
		kw.Action = "ban_user"
	}
	if kw.Severity == "" {
		kw.Severity = "high"
	}

	switch kw.MatchType {
	case "exact", "regex":
	default:
		return fmt.Errorf("不支持的匹配类型: %s", kw.MatchType)
	}
	switch kw.CheckScope {
	case "user_only", "user_and_tool", "all":
	default:
		return fmt.Errorf("不支持的检查范围: %s", kw.CheckScope)
	}
	switch kw.Action {
	case "ban_user", "block_only":
	default:
		return fmt.Errorf("不支持的触发动作: %s", kw.Action)
	}
	switch kw.Severity {
	case "high", "medium", "low":
	default:
		return fmt.Errorf("不支持的危险等级: %s", kw.Severity)
	}

	if kw.MatchType == "regex" {
		if strings.Contains(kw.Keyword, `\|`) {
			return fmt.Errorf(`正则中的“或”运算符直接使用 |，不要写 \|`)
		}
		if _, err := regexp.Compile(kw.Keyword); err != nil {
			return fmt.Errorf("正则表达式语法错误: %v", err)
		}
	}
	return nil
}

// --- 危险关键词 CRUD ---

// GetSecurityKeywords 分页查询危险关键词。
func GetSecurityKeywords(c *gin.Context) {
	keyword := c.Query("keyword")
	severity := c.Query("severity")
	action := c.Query("action")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	keywords, total, err := model.GetSecurityKeywords(keyword, severity, action, page, pageSize)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    keywords,
		"total":   total,
	})
}

// CreateSecurityKeyword 创建危险关键词。
func CreateSecurityKeyword(c *gin.Context) {
	var kw model.SecurityKeyword
	if err := json.NewDecoder(c.Request.Body).Decode(&kw); err != nil {
		common.ApiErrorMsg(c, "参数解析失败")
		return
	}
	if err := normalizeSecurityKeywordInput(&kw); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	kw.CreatedAt = time.Now()
	kw.UpdatedAt = time.Now()

	if err := model.CreateSecurityKeyword(&kw); err != nil {
		common.ApiError(c, err)
		return
	}
	service.InvalidateSecurityKeywordsCache()
	if common.DebugEnabled {
		common.SysLog(fmt.Sprintf("Security: created keyword id=%d, keyword=%s, action=%s, severity=%s, matchType=%s, checkScope=%s",
			kw.Id, kw.Keyword, kw.Action, kw.Severity, kw.MatchType, kw.CheckScope))
	}
	common.ApiSuccess(c, kw)
}

// UpdateSecurityKeyword 更新危险关键词。
func UpdateSecurityKeyword(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "无效的 ID")
		return
	}

	existing, err := model.GetSecurityKeywordById(id)
	if err != nil {
		common.ApiErrorMsg(c, "关键词不存在")
		return
	}

	var update model.SecurityKeyword
	if err := json.NewDecoder(c.Request.Body).Decode(&update); err != nil {
		common.ApiErrorMsg(c, "参数解析失败")
		return
	}

	if update.Keyword != "" {
		existing.Keyword = update.Keyword
	}
	if update.MatchType != "" {
		existing.MatchType = update.MatchType
	}
	if update.CheckScope != "" {
		existing.CheckScope = update.CheckScope
	}
	if update.Action != "" {
		existing.Action = update.Action
	}
	if update.Severity != "" {
		existing.Severity = update.Severity
	}
	existing.Description = update.Description
	existing.NotifyAdmin = update.NotifyAdmin
	existing.Enabled = update.Enabled
	if err := normalizeSecurityKeywordInput(existing); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	existing.UpdatedAt = time.Now()

	if err := model.UpdateSecurityKeyword(existing); err != nil {
		common.ApiError(c, err)
		return
	}
	service.InvalidateSecurityKeywordsCache()
	if common.DebugEnabled {
		common.SysLog(fmt.Sprintf("Security: updated keyword id=%d, keyword=%s, action=%s, severity=%s",
			existing.Id, existing.Keyword, existing.Action, existing.Severity))
	}
	common.ApiSuccess(c, existing)
}

// DeleteSecurityKeyword 删除危险关键词。
func DeleteSecurityKeyword(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "无效的 ID")
		return
	}
	if err := model.DeleteSecurityKeyword(id); err != nil {
		common.ApiError(c, err)
		return
	}
	service.InvalidateSecurityKeywordsCache()
	if common.DebugEnabled {
		common.SysLog(fmt.Sprintf("Security: deleted keyword id=%d", id))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

// ImportSecurityKeywords 通过 JSON 数组或 {items:[...]} 批量导入危险关键词。
// 若 keyword 已存在，则按 keyword 做更新（upsert）；否则创建新记录。
func ImportSecurityKeywords(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		common.ApiErrorMsg(c, "读取请求体失败")
		return
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		common.ApiErrorMsg(c, "导入内容不能为空")
		return
	}

	var items []model.SecurityKeyword
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal(body, &items); err != nil {
			common.ApiErrorMsg(c, "JSON 数组解析失败")
			return
		}
	} else {
		var req securityKeywordsImportRequest
		if err := json.Unmarshal(body, &req); err != nil {
			common.ApiErrorMsg(c, "JSON 对象解析失败")
			return
		}
		items = req.Items
	}
	if len(items) == 0 {
		common.ApiErrorMsg(c, "未解析到任何关键词条目")
		return
	}

	now := time.Now()
	created := 0
	updated := 0
	failed := 0
	errorsList := make([]string, 0)

	for idx := range items {
		item := items[idx]
		if err := normalizeSecurityKeywordInput(&item); err != nil {
			failed++
			errorsList = append(errorsList, fmt.Sprintf("第 %d 条失败: %v", idx+1, err))
			continue
		}

		existing, getErr := model.GetSecurityKeywordByKeyword(item.Keyword)
		if getErr == nil {
			existing.MatchType = item.MatchType
			existing.CheckScope = item.CheckScope
			existing.Action = item.Action
			existing.Severity = item.Severity
			existing.Description = item.Description
			existing.NotifyAdmin = item.NotifyAdmin
			existing.Enabled = item.Enabled
			existing.UpdatedAt = now
			if err := model.UpdateSecurityKeyword(existing); err != nil {
				failed++
				errorsList = append(errorsList, fmt.Sprintf("第 %d 条失败: %v", idx+1, err))
				continue
			}
			updated++
			continue
		}
		if !errors.Is(getErr, gorm.ErrRecordNotFound) {
			failed++
			errorsList = append(errorsList, fmt.Sprintf("第 %d 条失败: %v", idx+1, getErr))
			continue
		}

		item.CreatedAt = now
		item.UpdatedAt = now
		if err := model.CreateSecurityKeyword(&item); err != nil {
			failed++
			errorsList = append(errorsList, fmt.Sprintf("第 %d 条失败: %v", idx+1, err))
			continue
		}
		created++
	}

	service.InvalidateSecurityKeywordsCache()
	if common.DebugEnabled {
		common.SysLog(fmt.Sprintf("Security: imported keywords created=%d updated=%d failed=%d", created, updated, failed))
	}
	common.ApiSuccess(c, gin.H{
		"created": created,
		"updated": updated,
		"failed":  failed,
		"errors":  errorsList,
	})
}

// ToggleSecurityKeyword 切换危险关键词的启用/禁用状态。
func ToggleSecurityKeyword(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "无效的 ID")
		return
	}
	if err := model.ToggleSecurityKeyword(id); err != nil {
		common.ApiError(c, err)
		return
	}
	service.InvalidateSecurityKeywordsCache()
	if common.DebugEnabled {
		common.SysLog(fmt.Sprintf("Security: toggled keyword id=%d", id))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

// --- 安全审计日志 ---

// GetSecurityAuditLogs 分页查询安全审计日志。
func GetSecurityAuditLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	params := model.SecurityAuditLogQuery{
		EventType: c.Query("event_type"),
		Username:  c.Query("username"),
		Action:    c.Query("action"),
		Page:      page,
		PageSize:  pageSize,
	}
	if startStr := c.Query("start_time"); startStr != "" {
		if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			params.StartTime = t
		}
	}
	if endStr := c.Query("end_time"); endStr != "" {
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			params.EndTime = t
		}
	}

	logs, total, err := model.GetSecurityAuditLogs(params)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    logs,
		"total":   total,
	})
}

// ClearSecurityAuditLogs 清理过期审计日志。
func ClearSecurityAuditLogs(c *gin.Context) {
	days, _ := strconv.Atoi(c.DefaultQuery("retention_days", "90"))
	if days < 1 {
		days = 90
	}
	affected, err := model.ClearSecurityAuditLogs(days)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if common.DebugEnabled {
		common.SysLog(fmt.Sprintf("Security: cleared audit logs older than %d days, deleted=%d", days, affected))
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    gin.H{"deleted": affected},
	})
}

// --- 安全模块全局配置（预留桩）---

// GetSecurityConfig 获取安全模块全局配置（预留）。
func GetSecurityConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"retention_days": 90,
			"whitelist_user_ids": []int{},
			"notify_feishu_enabled": false,
			"notify_feishu_webhook": "",
			"notify_email_enabled":  false,
			"notify_email_address":  "",
		},
	})
}

// UpdateSecurityConfig 保存安全模块全局配置（预留桩）。
func UpdateSecurityConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

// TestSecurityNotify 测试安全通知推送（预留桩）。
func TestSecurityNotify(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "通知推送功能开发中"})
}
