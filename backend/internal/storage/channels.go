package storage

import (
	"gorm.io/gorm"
)

// Channels 渠道仓库。
type Channels struct{ db *gorm.DB }

func NewChannels(db *gorm.DB) *Channels { return &Channels{db: db} }

func (r *Channels) Create(c *Channel) error { return r.db.Create(c).Error }
func (r *Channels) Update(c *Channel) error { return r.db.Save(c).Error }
func (r *Channels) Delete(id uint) error    { return r.db.Delete(&Channel{}, id).Error }

func (r *Channels) UpdateWithMetricScale(c *Channel, scale float64) error {
	if scale <= 0 {
		scale = 1
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		if scale != 1 && c.LastBalance != nil {
			v := *c.LastBalance * scale
			c.LastBalance = &v
		}
		if err := tx.Save(c).Error; err != nil {
			return err
		}
		if scale == 1 {
			return nil
		}
		if err := tx.Model(&BalanceSnapshot{}).
			Where("channel_id = ?", c.ID).
			Update("balance", gorm.Expr("balance * ?", scale)).Error; err != nil {
			return err
		}
		if err := tx.Model(&RateSnapshot{}).
			Where("channel_id = ?", c.ID).
			Updates(map[string]any{
				"ratio":            gorm.Expr("ratio * ?", scale),
				"completion_ratio": gorm.Expr("completion_ratio * ?", scale),
			}).Error; err != nil {
			return err
		}
		return tx.Model(&RateChangeLog{}).
			Where("channel_id = ?", c.ID).
			Updates(map[string]any{
				"old_ratio":            gorm.Expr("CASE WHEN old_ratio IS NULL THEN NULL ELSE old_ratio * ? END", scale),
				"new_ratio":            gorm.Expr("new_ratio * ?", scale),
				"old_completion_ratio": gorm.Expr("CASE WHEN old_completion_ratio IS NULL THEN NULL ELSE old_completion_ratio * ? END", scale),
				"new_completion_ratio": gorm.Expr("new_completion_ratio * ?", scale),
			}).Error
	})
}

func (r *Channels) FindByID(id uint) (*Channel, error) {
	var c Channel
	if err := r.db.First(&c, id).Error; err != nil {
		return nil, err
	}
	return &c, nil
}
func (r *Channels) List() ([]Channel, error) {
	var list []Channel
	if err := r.db.Order("id ASC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}
func (r *Channels) ListMonitorEnabled() ([]Channel, error) {
	var list []Channel
	if err := r.db.Where("monitor_enabled = ?", true).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}
func (r *Channels) UpdateBalance(id uint, balance float64, at any, lastErr string) error {
	return r.db.Model(&Channel{}).Where("id = ?", id).Updates(map[string]any{
		"last_balance":    balance,
		"last_balance_at": at,
		"last_error":      lastErr,
	}).Error
}
func (r *Channels) SetLastError(id uint, msg string) error {
	return r.db.Model(&Channel{}).Where("id = ?", id).Update("last_error", msg).Error
}
