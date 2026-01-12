package database

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/rehiy/web-modem/models"
)

var (
	db     *gorm.DB
	once   sync.Once
	dbPath string
)

// GetDB 获取数据库实例
func GetDB() *gorm.DB {
	return db
}

// Close 关闭数据库连接
func Close() error {
	if db != nil {
		if sqlDB, err := db.DB(); err == nil {
			return sqlDB.Close()
		} else {
			return err
		}
	}
	return nil
}

// InitDB 初始化数据库连接
func InitDB() error {
	var err error
	once.Do(func() {
		// 获取数据库路径
		dbPath = os.Getenv("DB_PATH")
		if dbPath == "" {
			dbPath = "data/modem.db"
		}

		// 创建目录
		dir := filepath.Dir(dbPath)
		if err = os.MkdirAll(dir, 0755); err != nil {
			return
		}

		// 连接数据库
		db, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if err != nil {
			return
		}

		// 创建表
		if err = createTables(); err != nil {
			return
		}

		log.Printf("Database initialized at: %s", dbPath)
	})
	return err
}

// createTables 创建数据表
func createTables() error {
	// 自动迁移
	err := db.AutoMigrate(
		&models.SMS{},
		&models.Webhook{},
		&models.Setting{},
	)
	if err != nil {
		return fmt.Errorf("failed to auto migrate: %w", err)
	}

	// 初始化默认设置
	if err := InitDefaultSettings(); err != nil {
		return fmt.Errorf("failed to init default settings: %w", err)
	}

	return nil
}
