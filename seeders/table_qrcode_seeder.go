package seeders

import (
	"fmt"

	"pos-backend/models"

	"gorm.io/gorm"
)

func seedTables(db *gorm.DB) ([]models.Table, error) {
	tables := make([]models.Table, 0, 20)
	for number := 1; number <= 20; number++ {
		table := models.Table{
			TableNumber: fmt.Sprintf("Meja %d", number),
			Status:      models.TableAvailable,
		}
		if err := db.Where("table_number = ?", table.TableNumber).FirstOrCreate(&table).Error; err != nil {
			return nil, fmt.Errorf("create table %s: %w", table.TableNumber, err)
		}
		tables = append(tables, table)
	}
	return tables, nil
}

func seedQRCodes(db *gorm.DB, tables []models.Table) error {
	for _, table := range tables {
		code := fmt.Sprintf("DINE-IN-TABLE-%02d", table.ID)
		qrCode := models.QRCode{
			Code:     code,
			Type:     models.QRCodeDineIn,
			TableID:  &table.ID,
			URL:      fmt.Sprintf("/order?type=dine_in&table=%d", table.ID),
			IsActive: true,
		}
		if err := db.Where("table_id = ? AND type = ?", table.ID, models.QRCodeDineIn).FirstOrCreate(&qrCode).Error; err != nil {
			return fmt.Errorf("create QR code for table %d: %w", table.ID, err)
		}
		if table.QRCode == "" {
			if err := db.Model(&table).Update("qr_code", qrCode.Code).Error; err != nil {
				return fmt.Errorf("link QR code to table %d: %w", table.ID, err)
			}
		}
	}

	takeaway := models.QRCode{
		Code:     "TAKE-AWAY-DEFAULT",
		Type:     models.QRCodeTakeAway,
		URL:      "/order?type=take_away",
		IsActive: true,
	}
	if err := db.Where("type = ?", models.QRCodeTakeAway).FirstOrCreate(&takeaway).Error; err != nil {
		return fmt.Errorf("create takeaway QR code: %w", err)
	}
	return nil
}
