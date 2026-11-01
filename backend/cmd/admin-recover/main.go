// admin-recover is an explicit local-only recovery utility; it is never exposed by HTTP.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/wybo/backend/internal/config"
	"github.com/wybo/backend/internal/model"
	"github.com/wybo/backend/internal/service"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"log"
	"os"
	"strings"
)

func main() {
	username := flag.String("username", "", "existing administrator username")
	reset := flag.Bool("reset-2fa", false, "remove lost authenticator")
	path := flag.String("admin-path", "", "restore a known admin path")
	pass := flag.Bool("password-stdin", false, "read a new password from standard input")
	flag.Parse()
	if *username == "" || (!*reset && !*pass && *path == "") {
		log.Fatal("Specify --username and at least one of --reset-2fa, --password-stdin, --admin-path")
	}
	updates := map[string]interface{}{"session_version": gorm.Expr("COALESCE(session_version,0) + 1")}
	if *pass {
		raw, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && len(raw) == 0 {
			log.Fatal("Could not read password")
		}
		password := strings.TrimRight(raw, "\r\n")
		if len(password) < 12 || len(password) > 72 {
			log.Fatal("Password must be 12–72 bytes")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			log.Fatal("Could not hash password")
		}
		updates["password_hash"] = string(hash)
	}
	if *reset {
		updates["totp_secret"] = ""
		updates["totp_pending"] = ""
		updates["totp_expires_at"] = nil
		updates["last_totp_step"] = 0
	}
	cfg := config.Load()
	db, err := model.Connect(cfg)
	if err != nil {
		log.Fatal("Could not open configured database")
	}
	svc := service.New(db, cfg, zap.NewNop())
	if *path != "" {
		if err := svc.ValidateAdminPath(*path); err != nil {
			log.Fatal(err)
		}
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		var a model.Admin
		if tx.Where("username = ?", *username).First(&a).Error != nil {
			return fmt.Errorf("administrator does not exist")
		}
		if err := tx.Model(&a).Updates(updates).Error; err != nil {
			return err
		}
		if *path != "" {
			return tx.Where("key = ?", "admin_path").Assign(model.Setting{Value: *path}).FirstOrCreate(&model.Setting{Key: "admin_path"}).Error
		}
		return nil
	})
	if err != nil {
		log.Fatal("Recovery failed: ", err)
	}
	fmt.Println("Recovery complete. Existing sessions were revoked.")
}
