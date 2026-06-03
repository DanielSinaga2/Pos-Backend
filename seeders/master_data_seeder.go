package seeders

import (
	"fmt"

	"pos-backend/models"

	"gorm.io/gorm"
)

type defaultCategory struct {
	Name        string
	Description string
}

type defaultMenu struct {
	CategoryName string
	Name         string
	Description  string
	Price        int64
}

func seedCategories(db *gorm.DB) (map[string]models.Category, error) {
	defaults := []defaultCategory{
		{Name: "Makanan", Description: "Menu makanan utama"},
		{Name: "Minuman", Description: "Menu minuman dingin dan hangat"},
		{Name: "Paket Hemat", Description: "Paket makanan dan minuman"},
		{Name: "Dessert", Description: "Menu pencuci mulut"},
	}
	categories := make(map[string]models.Category, len(defaults))
	for _, item := range defaults {
		category := models.Category{Name: item.Name, Description: item.Description, IsActive: true}
		if err := db.Where("name = ?", item.Name).FirstOrCreate(&category).Error; err != nil {
			return nil, fmt.Errorf("create category %s: %w", item.Name, err)
		}
		categories[item.Name] = category
	}
	return categories, nil
}

func seedMenus(db *gorm.DB, categories map[string]models.Category) error {
	defaults := []defaultMenu{
		{CategoryName: "Makanan", Name: "Mie Pedas Level 1", Description: "Mie pedas level ringan", Price: 12000},
		{CategoryName: "Makanan", Name: "Mie Pedas Level 2", Description: "Mie pedas level sedang", Price: 12000},
		{CategoryName: "Makanan", Name: "Mie Pedas Level 3", Description: "Mie pedas level tinggi", Price: 12000},
		{CategoryName: "Makanan", Name: "Mie Original", Description: "Mie gurih tanpa tambahan cabai", Price: 11000},
		{CategoryName: "Makanan", Name: "Ayam Geprek", Description: "Ayam geprek sambal pedas", Price: 18000},
		{CategoryName: "Makanan", Name: "Nasi Ayam", Description: "Nasi dengan ayam berbumbu", Price: 17000},
		{CategoryName: "Minuman", Name: "Es Teh", Description: "Teh manis dingin", Price: 5000},
		{CategoryName: "Minuman", Name: "Es Jeruk", Description: "Minuman jeruk dingin", Price: 7000},
		{CategoryName: "Minuman", Name: "Lemon Tea", Description: "Teh lemon segar", Price: 8000},
		{CategoryName: "Minuman", Name: "Air Mineral", Description: "Air mineral kemasan", Price: 4000},
		{CategoryName: "Paket Hemat", Name: "Paket Mie + Es Teh", Description: "Paket mie original dan es teh", Price: 15000},
		{CategoryName: "Dessert", Name: "Puding Coklat", Description: "Puding coklat lembut", Price: 9000},
	}

	for _, item := range defaults {
		category, exists := categories[item.CategoryName]
		if !exists {
			return fmt.Errorf("category %s not found", item.CategoryName)
		}
		menu := models.Menu{
			CategoryID:  category.ID,
			Name:        item.Name,
			Description: item.Description,
			Price:       item.Price,
			IsAvailable: true,
		}
		if err := db.Where("name = ?", item.Name).FirstOrCreate(&menu).Error; err != nil {
			return fmt.Errorf("create menu %s: %w", item.Name, err)
		}
	}
	return nil
}
