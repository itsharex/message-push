package migration

import (
	"database/sql"
	"fmt"

	"cnb.cool/mliev/push/message-push/internal/interfaces"
	// 导入迁移文件以触发 init() 注册
	"cnb.cool/mliev/push/message-push/migrations"
	"github.com/muleiwu/gsr"
	"github.com/pressly/goose/v3"
	"gorm.io/gorm"
)

type Migration struct {
	Helper    interfaces.HelperInterface
	Migration []any
}

func (receiver *Migration) Run() error {
	if !receiver.Helper.GetConfig().GetBool("app.installed", false) {
		receiver.Helper.GetLogger().Warn("[db migration] 数据库未安装，不执行迁移")
		return nil
	}

	// 获取 GORM 底层的 sql.DB 连接
	gormDB := receiver.Helper.GetDatabase()
	if gormDB == nil {
		return fmt.Errorf("[db migration] 数据库连接未初始化")
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return fmt.Errorf("[db migration] 获取 sql.DB 失败: %w", err)
	}

	// 执行 goose 迁移，传入 GORM DB 供 initial_schema 使用
	logger := &gsrLoggerAdapter{receiver.Helper.GetLogger()}
	if err := RunGooseMigrationsWithGorm(sqlDB, gormDB, logger); err != nil {
		return fmt.Errorf("[db migration] goose 迁移失败: %w", err)
	}

	return nil
}

// Logger 定义日志接口
type Logger interface {
	Info(msg string)
	Warn(msg string)
	Error(msg string)
}

// simpleLogger 简单日志实现（用于安装时没有 helper 的情况）
type simpleLogger struct{}

func (l *simpleLogger) Info(msg string)  { fmt.Println("[INFO]", msg) }
func (l *simpleLogger) Warn(msg string)  { fmt.Println("[WARN]", msg) }
func (l *simpleLogger) Error(msg string) { fmt.Println("[ERROR]", msg) }

// gsrLoggerAdapter 适配 gsr.Logger 接口
type gsrLoggerAdapter struct {
	logger gsr.Logger
}

func (l *gsrLoggerAdapter) Info(msg string)  { l.logger.Info(msg) }
func (l *gsrLoggerAdapter) Warn(msg string)  { l.logger.Warn(msg) }
func (l *gsrLoggerAdapter) Error(msg string) { l.logger.Error(msg) }

// RunGooseMigrations 执行 goose 版本化迁移（简单版本，供安装控制器使用）
// 安装控制器需要先调用 migrations.SetExternalDB() 设置 GORM 连接
func RunGooseMigrations(db *sql.DB, logger Logger) error {
	return RunGooseMigrationsWithGorm(db, nil, logger)
}

// RunGooseMigrationsWithGorm 执行 goose 版本化迁移（完整版本）
// gormDB 用于 initial_schema 迁移中的 AutoMigrate
func RunGooseMigrationsWithGorm(db *sql.DB, gormDB *gorm.DB, logger Logger) error {
	if logger == nil {
		logger = &simpleLogger{}
	}

	// 如果提供了 GORM DB，设置外部连接供 initial_schema 使用
	if gormDB != nil {
		migrations.SetExternalDB(gormDB)
		defer migrations.ClearExternalDB()
	}

	// 设置 goose 方言为 MySQL
	if err := goose.SetDialect("mysql"); err != nil {
		return fmt.Errorf("设置 goose 方言失败: %w", err)
	}

	// 获取当前版本
	currentVersion, err := goose.GetDBVersion(db)
	if err != nil {
		// 如果表不存在，会自动创建
		logger.Info("[db migration] 初始化 goose 迁移表...")
		currentVersion = 0
	} else {
		logger.Info(fmt.Sprintf("[db migration] 当前数据库版本: %d", currentVersion))
	}

	// 执行所有待执行的迁移
	// 使用 UpByOne 循环执行，这样可以更好地控制和记录每个迁移
	for {
		err := goose.UpByOne(db, ".")
		if err != nil {
			if err == goose.ErrNoNextVersion {
				// 没有更多迁移需要执行
				break
			}
			return fmt.Errorf("执行迁移失败: %w", err)
		}
	}

	// 获取迁移后的版本
	newVersion, _ := goose.GetDBVersion(db)
	if newVersion != currentVersion {
		logger.Info(fmt.Sprintf("[db migration] goose 迁移完成: 版本 %d -> %d", currentVersion, newVersion))
	} else {
		logger.Info("[db migration] goose 迁移已是最新版本")
	}

	return nil
}

// Stop Migration 服务不需要停止操作，空实现
func (receiver *Migration) Stop() error {
	return nil
}
