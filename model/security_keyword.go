package model

import (
	"time"

	"gorm.io/gorm"
)

// SecurityKeyword 危险关键词配置，用于信息安全模块检测和拦截。
// Phase 2 将在 relay 层集成 Aho-Corasick 匹配引擎。
type SecurityKeyword struct {
	Id           int            `json:"id" gorm:"primaryKey;autoIncrement"`
	Keyword      string         `json:"keyword" gorm:"type:varchar(500);not null;uniqueIndex"`
	MatchType    string         `json:"match_type" gorm:"type:varchar(20);default:'exact'"`
	CheckScope   string         `json:"check_scope" gorm:"type:varchar(20);default:'user_only'"`
	Action       string         `json:"action" gorm:"type:varchar(50);default:'ban_user'"`
	Severity     string         `json:"severity" gorm:"type:varchar(20);default:'high'"`
	Description  string         `json:"description" gorm:"type:text"`
	NotifyAdmin  bool           `json:"notify_admin" gorm:"default:true"`
	Enabled      bool           `json:"enabled" gorm:"default:true"`
	TriggerCount int            `json:"trigger_count" gorm:"default:0"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `json:"-" gorm:"index"`
}

func (SecurityKeyword) TableName() string {
	return "security_keywords"
}

// GetSecurityKeywords 分页查询危险关键词，支持按关键词文本、等级、动作过滤。
func GetSecurityKeywords(keyword, severity, action string, page, pageSize int) (keywords []SecurityKeyword, total int64, err error) {
	tx := DB.Model(&SecurityKeyword{})
	if keyword != "" {
		tx = tx.Where("keyword LIKE ?", "%"+keyword+"%")
	}
	if severity != "" {
		tx = tx.Where("severity = ?", severity)
	}
	if action != "" {
		tx = tx.Where("action = ?", action)
	}
	err = tx.Count(&total).Error
	if err != nil {
		return
	}
	err = tx.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&keywords).Error
	return
}

// GetSecurityKeywordById 根据 ID 查询单条记录。
func GetSecurityKeywordById(id int) (*SecurityKeyword, error) {
	var kw SecurityKeyword
	err := DB.First(&kw, id).Error
	if err != nil {
		return nil, err
	}
	return &kw, nil
}

// GetSecurityKeywordByKeyword 根据关键词文本查询单条记录。
func GetSecurityKeywordByKeyword(keyword string) (*SecurityKeyword, error) {
	var kw SecurityKeyword
	err := DB.Where("keyword = ?", keyword).First(&kw).Error
	if err != nil {
		return nil, err
	}
	return &kw, nil
}

// CreateSecurityKeyword 创建危险关键词。
func CreateSecurityKeyword(kw *SecurityKeyword) error {
	return DB.Create(kw).Error
}

// UpdateSecurityKeyword 更新危险关键词。
func UpdateSecurityKeyword(kw *SecurityKeyword) error {
	return DB.Save(kw).Error
}

// DeleteSecurityKeyword 软删除危险关键词。
func DeleteSecurityKeyword(id int) error {
	return DB.Delete(&SecurityKeyword{}, id).Error
}

// ToggleSecurityKeyword 切换危险关键词的启用/禁用状态。
func ToggleSecurityKeyword(id int) error {
	return DB.Exec("UPDATE security_keywords SET enabled = NOT enabled, updated_at = ? WHERE id = ? AND deleted_at IS NULL", time.Now(), id).Error
}

// GetAllEnabledSecurityKeywords 获取所有启用的关键词（供 Phase 2 匹配引擎加载）。
func GetAllEnabledSecurityKeywords() ([]SecurityKeyword, error) {
	var keywords []SecurityKeyword
	err := DB.Where("enabled = ?", true).Find(&keywords).Error
	return keywords, err
}
