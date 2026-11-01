package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"github.com/glebarez/sqlite"
	"github.com/wybo/backend/internal/config"
	"github.com/wybo/backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	backup := flag.String("backup", "", "SQLite backup path")
	expected := flag.String("sha256", "", "Expected SHA256 from admin backup list")
	flag.Parse()
	if *backup == "" || len(*expected) != 64 {
		log.Fatal("--backup and --sha256 are required")
	}
	cfg := config.Load()
	if cfg.DB.Driver != "sqlite" {
		log.Fatal("SQLite only")
	}
	unlock, err := model.DatabaseLock(cfg.DB.DSN, true)
	if err != nil {
		log.Fatal(err)
	}
	defer unlock()
	raw, err := os.ReadFile(*backup)
	if err != nil {
		log.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != strings.ToLower(*expected) {
		log.Fatal("Backup SHA256 mismatch")
	}
	path, err := filepath.Abs(*backup)
	if err != nil {
		log.Fatal(err)
	}
	db, err := gorm.Open(sqlite.Open("file:"+path+"?mode=ro"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		log.Fatal(err)
	}
	var check string
	if err = db.Raw("PRAGMA quick_check").Scan(&check).Error; err != nil || check != "ok" {
		log.Fatal("Backup integrity check failed")
	}
	for _, table := range []string{"admins", "products", "orders", "cards", "settings"} {
		if !db.Migrator().HasTable(table) {
			log.Fatal("Not a Wybo backup: missing ", table)
		}
	}
	sqlDB, _ := db.DB()
	sqlDB.Close()
	current, err := gorm.Open(sqlite.Open(cfg.DB.DSN), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		log.Fatal(err)
	}
	prior := cfg.DB.DSN + ".before-restore-" + time.Now().Format("20060102150405") + ".db"
	if err = current.Exec("VACUUM INTO '" + strings.ReplaceAll(prior, "'", "''") + "'").Error; err != nil {
		log.Fatal(err)
	}
	os.Chmod(prior, 0600)
	conn, _ := current.DB()
	conn.Close()
	stage := cfg.DB.DSN + ".restoring"
	if err = os.WriteFile(stage, raw, 0600); err != nil {
		log.Fatal(err)
	}
	f, err := os.OpenFile(stage, os.O_RDWR, 0600)
	if err != nil {
		log.Fatal(err)
	}
	if err = f.Sync(); err != nil {
		log.Fatal(err)
	}
	f.Close()
	os.Remove(cfg.DB.DSN + "-wal")
	os.Remove(cfg.DB.DSN + "-shm")
	if err = os.Rename(stage, cfg.DB.DSN); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Restored. Previous snapshot:", prior)
	fmt.Println("Restart the backend with the same JWT_SECRET to read encrypted settings.")
}
