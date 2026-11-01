// init-plans adds the requested ChatGPT Pro plans without changing existing prices.
// Run from backend/: go run ./cmd/init-plans
package main

import (
	"errors"
	"fmt"
	"log"

	"github.com/wybo/backend/internal/config"
	"github.com/wybo/backend/internal/model"
	"gorm.io/gorm"
)

func main() {
	db, err := model.Connect(config.Load())
	if err != nil {
		log.Fatal(err)
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		var category model.Category
		err := tx.Where("LOWER(name) = ?", "chatgpt").First(&category).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			category = model.Category{Name: "ChatGPT", Enabled: true}
			if err = tx.Create(&category).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		for i, multiplier := range []int{5, 20, 50} {
			name := fmt.Sprintf("ChatGPT Pro %dx", multiplier)
			var count int64
			if err := tx.Model(&model.Product{}).Where("category_id = ? AND LOWER(name) = LOWER(?)", category.ID, name).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				fmt.Printf("保留已有套餐及其价格：%s\n", name)
				continue
			}
			product := model.Product{
				CategoryID:  category.ID,
				Name:        name,
				Description: name + " 套餐。购买前请确认商品权益、使用期限和兑换说明。",
				PriceCents:  0,
				RedeemURL:   "https://chatgpt.com",
				RedeemGuide: "支付完成后，在订单查询中领取卡密，并按照商品附带的说明使用。",
				Enabled:     true,
				Sort:        i + 1,
			}
			if err := tx.Create(&product).Error; err != nil {
				return err
			}
			fmt.Printf("已添加：%s（待定价，可通过后台修改）\n", name)
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
}
