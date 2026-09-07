package storage

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const sub2APIOpsConfigID uint = 1

// Sub2APIOpsConfigs 管理单实例 Sub2API 运维连接。保留独立表，避免把全权限
// admin key 混入用于余额采集的普通渠道凭据。
type Sub2APIOpsConfigs struct{ db *gorm.DB }

func NewSub2APIOpsConfigs(db *gorm.DB) *Sub2APIOpsConfigs {
	return &Sub2APIOpsConfigs{db: db}
}

func (r *Sub2APIOpsConfigs) Get() (*Sub2APIOpsConfig, error) {
	var config Sub2APIOpsConfig
	if err := r.db.First(&config, sub2APIOpsConfigID).Error; err != nil {
		return nil, err
	}
	return &config, nil
}

func (r *Sub2APIOpsConfigs) UpsertConnection(config *Sub2APIOpsConfig) error {
	config.ID = sub2APIOpsConfigID
	config.UpdatedAt = time.Now()
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "site_url", "admin_key_cipher", "updated_at"}),
	}).Create(config).Error
}

// UpdateConnectionMetadata updates non-secret fields only if the address still matches the
// version read by the caller. This prevents a stale keyless request from splitting a newer
// address/key pair written concurrently.
func (r *Sub2APIOpsConfigs) UpdateConnectionMetadata(config *Sub2APIOpsConfig) error {
	now := time.Now()
	result := r.db.Model(&Sub2APIOpsConfig{}).
		Where("id = ? AND site_url = ?", sub2APIOpsConfigID, config.SiteURL).
		Updates(map[string]any{
			"name":       config.Name,
			"updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	config.UpdatedAt = now
	return nil
}

func (r *Sub2APIOpsConfigs) UpdateLayout(layout string) error {
	result := r.db.Model(&Sub2APIOpsConfig{}).
		Where("id = ?", sub2APIOpsConfigID).
		Updates(map[string]any{
			"dashboard_layout": layout,
			"updated_at":       time.Now(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
