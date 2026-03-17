package service

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

var (
	securityKeywordsCache   atomic.Value // []model.SecurityKeyword
	securityCacheExpireAt   atomic.Int64
	securityCacheMu         sync.Mutex
	securityCacheTTLSeconds int64 = 30

	// regexpCache 缓存已编译的正则表达式，避免每次请求重复编译，key 为正则字符串。
	regexpCache sync.Map // map[string]*regexp.Regexp
)

// loadSecurityKeywords 从数据库加载启用的关键词，带本地缓存避免频繁查库。
func loadSecurityKeywords() []model.SecurityKeyword {
	now := time.Now().Unix()
	if cached := securityKeywordsCache.Load(); cached != nil {
		if now < securityCacheExpireAt.Load() {
			return cached.([]model.SecurityKeyword)
		}
	}
	securityCacheMu.Lock()
	defer securityCacheMu.Unlock()
	if now < securityCacheExpireAt.Load() {
		if cached := securityKeywordsCache.Load(); cached != nil {
			return cached.([]model.SecurityKeyword)
		}
	}
	keywords, err := model.GetAllEnabledSecurityKeywords()
	if err != nil {
		if common.DebugEnabled {
			common.SysLog("Security: failed to load keywords from DB: " + err.Error())
		}
		return nil
	}
	securityKeywordsCache.Store(keywords)
	securityCacheExpireAt.Store(now + securityCacheTTLSeconds)
	if common.DebugEnabled {
		common.SysLog(fmt.Sprintf("Security: loaded %d enabled keywords from DB", len(keywords)))
	}
	return keywords
}

// InvalidateSecurityKeywordsCache 在关键词 CRUD 操作后调用，强制下次请求重新加载，
// 同时清除正则缓存（关键词可能已修改）。
func InvalidateSecurityKeywordsCache() {
	securityCacheExpireAt.Store(0)
	regexpCache.Range(func(key, _ any) bool {
		regexpCache.Delete(key)
		return true
	})
}

// getOrCompileRegexp 返回缓存的已编译正则，不存在则编译并缓存。
// 编译失败返回 nil，调用方应跳过该关键词。
func getOrCompileRegexp(pattern string) *regexp.Regexp {
	if v, ok := regexpCache.Load(pattern); ok {
		return v.(*regexp.Regexp)
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		if common.DebugEnabled {
			common.SysLog("Security: invalid regex pattern: " + pattern + " err=" + err.Error())
		}
		return nil
	}
	actual, _ := regexpCache.LoadOrStore(pattern, re)
	return actual.(*regexp.Regexp)
}

// SecurityCheckResult 安全检测结果。
type SecurityCheckResult struct {
	Hit         bool
	Keyword     model.SecurityKeyword
	Matched     string // 实际匹配到的文本片段
	UserMessage string // 触发时的完整用户消息，用于审计日志摘要
}

// ExtractTextByScope 根据 checkScope 从 messages 中提取待检测文本，
// 同时返回最后一条 user 消息（用于审计日志摘要，不受 scope 影响）。
//
// check_scope 三档含义：
//   - "user_only"     : 所有 role=user 消息（含历史轮次），防止用户手动粘贴敏感内容
//   - "user_and_tool" : role=user + role=tool，防止工具读取的外部文档中含敏感信息
//   - "all"           : 全部消息（含 system、assistant），最严格，性能开销最大
func ExtractTextByScope(messages []dto.Message, checkScope string) (checkText, lastUserText string) {
	var sb strings.Builder
	for _, msg := range messages {
		var shouldCheck bool
		switch checkScope {
		case "all":
			shouldCheck = true
		case "user_and_tool":
			shouldCheck = msg.Role == "user" || msg.Role == "tool"
		default: // "user_only"
			shouldCheck = msg.Role == "user"
		}
		if shouldCheck {
			if text := msg.StringContent(); text != "" {
				if sb.Len() > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString(text)
			}
		}
		// 始终跟踪最后一条 user 消息，用于审计日志摘要
		if msg.Role == "user" {
			if text := msg.StringContent(); text != "" {
				lastUserText = text
			}
		}
	}
	return sb.String(), lastUserText
}

// ExtractLastUserText 从 messages 中提取最后一条 role=user 的消息文本内容。
// 保留此函数以兼容其他可能的调用方。
func ExtractLastUserText(messages []dto.Message) string {
	_, last := ExtractTextByScope(messages, "user_only")
	return last
}

// HandleSecurityHit 在检测命中后异步写入审计日志并更新触发计数。
func HandleSecurityHit(userId int, username string, result *SecurityCheckResult, modelName, ip string) {
	actionStr := "request_blocked"
	if result.Keyword.Action == "ban_user" {
		actionStr = "user_banned"
	}

	// 优先使用完整用户消息作为摘要，其次降级为匹配片段
	summary := result.UserMessage
	if summary == "" {
		summary = result.Matched
	}
	if len(summary) > 200 {
		summary = summary[:200]
	}
	log := &model.SecurityAuditLog{
		UserId:         userId,
		Username:       username,
		KeywordId:      result.Keyword.Id,
		TriggerKeyword: result.Keyword.Keyword,
		EventType:      "dangerous_keyword",
		Action:         actionStr,
		RequestSummary: summary,
		ModelName:      modelName,
		IpAddress:      ip,
	}
	if err := model.CreateSecurityAuditLog(log); err != nil && common.DebugEnabled {
		common.SysLog("Security: failed to create audit log: " + err.Error())
	}

	// 更新触发计数
	model.DB.Exec("UPDATE security_keywords SET trigger_count = trigger_count + 1 WHERE id = ?", result.Keyword.Id)
}

// HandleSensitiveWordHit 在现有屏蔽词检测命中后异步写入审计日志，统一安全事件追踪。
func HandleSensitiveWordHit(userId int, username string, words []string, modelName, ip string) {
	triggerKeyword := strings.Join(words, ", ")
	summary := triggerKeyword
	if len(summary) > 200 {
		summary = summary[:200]
	}
	log := &model.SecurityAuditLog{
		UserId:         userId,
		Username:       username,
		TriggerKeyword: triggerKeyword,
		EventType:      "sensitive_word",
		Action:         "request_blocked",
		RequestSummary: summary,
		ModelName:      modelName,
		IpAddress:      ip,
	}
	if err := model.CreateSecurityAuditLog(log); err != nil && common.DebugEnabled {
		common.SysLog("Security: failed to create sensitive word audit log: " + err.Error())
	}
	if common.DebugEnabled {
		common.SysLog(fmt.Sprintf("Security: sensitive word audit logged, user=%s, words=%s", username, triggerKeyword))
	}
}

// BanUserForSecurity 因安全关键词命中而禁用用户。
// 只禁用 User 账号 + 清除 Redis 缓存，不操作 Token。
// Token 的可用性由 middleware/auth.go TokenAuth 中的 GetUserCache → Status 检查保障：
// 用户被禁用后，所有使用该用户 Token 的请求都会被中间件拒绝（"用户已被封禁"），
// 即使用户通过 Web 页面重新启用了 Token 也无法绕过。
func BanUserForSecurity(userId int, username string) {
	if err := model.DB.Model(&model.User{}).Where("id = ?", userId).Update("status", common.UserStatusDisabled).Error; err != nil {
		common.SysError("Security: failed to ban user " + username + ": " + err.Error())
		return
	}

	common.SysLog(fmt.Sprintf("Security: user banned, userId=%d, username=%s", userId, username))

	// 清除 Redis 用户缓存，确保下次请求立即从 DB 读取到 disabled 状态
	_ = common.RedisDel(fmt.Sprintf("user:%d", userId))
}

// CheckSecurityText 供 relay 层调用的检测入口。
// 根据每条关键词的 check_scope 分别提取对应范围的文本进行匹配。
// 支持三档扫描范围：user_only / user_and_tool / all。
// 正则表达式使用全局缓存，只在首次使用时编译，之后复用已编译对象。
func CheckSecurityText(fullText string, messages []dto.Message) *SecurityCheckResult {
	keywords := loadSecurityKeywords()
	if len(keywords) == 0 {
		return nil
	}

	// 预计算各 scope 对应的文本，避免对每条关键词重复提取
	// scopeTextCache: checkScope → (lowerText, lastUserText)
	type scopeText struct {
		text     string // 小写，用于 AC 匹配
		raw      string // 原始大小写，用于正则匹配
		lastUser string // 最后一条 user 消息，用于审计日志摘要
	}
	scopeCache := make(map[string]*scopeText, 3)
	getScope := func(scope string) *scopeText {
		if v, ok := scopeCache[scope]; ok {
			return v
		}
		raw, last := ExtractTextByScope(messages, scope)
		v := &scopeText{text: strings.ToLower(raw), raw: raw, lastUser: last}
		scopeCache[scope] = v
		return v
	}

	// 收集 exact 关键词，按 scope 分组，每组用 Aho-Corasick 批量匹配
	type exactGroup struct {
		words    []string
		kwMap    map[string]*model.SecurityKeyword
		scope    string
	}
	groupMap := make(map[string]*exactGroup)
	for i := range keywords {
		kw := &keywords[i]
		if kw.MatchType == "regex" {
			continue
		}
		scope := kw.CheckScope
		if scope == "" {
			scope = "user_only"
		}
		g, ok := groupMap[scope]
		if !ok {
			g = &exactGroup{scope: scope, kwMap: make(map[string]*model.SecurityKeyword)}
			groupMap[scope] = g
		}
		lower := strings.ToLower(kw.Keyword)
		g.words = append(g.words, lower)
		g.kwMap[lower] = kw
	}

	// 按 scope 批量匹配 exact 关键词（性能最优，优先处理）
	for _, g := range groupMap {
		st := getScope(g.scope)
		if len(g.words) == 0 || st.text == "" {
			continue
		}
		if hit, matched := AcSearch(st.text, g.words, true); hit && len(matched) > 0 {
			if kw, ok := g.kwMap[strings.ToLower(matched[0])]; ok {
				return &SecurityCheckResult{Hit: true, Keyword: *kw, Matched: matched[0], UserMessage: st.lastUser}
			}
		}
	}

	// 逐条匹配 regex 关键词（使用缓存的已编译正则，避免重复编译）
	for i := range keywords {
		kw := &keywords[i]
		if kw.MatchType != "regex" {
			continue
		}
		re := getOrCompileRegexp(kw.Keyword)
		if re == nil {
			continue
		}
		scope := kw.CheckScope
		if scope == "" {
			scope = "user_only"
		}
		st := getScope(scope)
		if st.raw == "" {
			continue
		}
		if loc := re.FindString(st.raw); loc != "" {
			return &SecurityCheckResult{Hit: true, Keyword: *kw, Matched: loc, UserMessage: st.lastUser}
		}
	}

	return nil
}
