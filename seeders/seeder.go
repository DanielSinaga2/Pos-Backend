package seeders

import (
	"fmt"

	"gorm.io/gorm"
)

func Run(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := SeedDefaultUsers(tx); err != nil {
			return fmt.Errorf("seed users: %w", err)
		}
		categories, err := seedCategories(tx)
		if err != nil {
			return fmt.Errorf("seed categories: %w", err)
		}
		if err := seedMenus(tx, categories); err != nil {
			return fmt.Errorf("seed menus: %w", err)
		}
		tables, err := seedTables(tx)
		if err != nil {
			return fmt.Errorf("seed tables: %w", err)
		}
		if err := seedQRCodes(tx, tables); err != nil {
			return fmt.Errorf("seed QR codes: %w", err)
		}
		return nil
	})
}
