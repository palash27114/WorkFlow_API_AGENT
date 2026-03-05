package database

import (
	"fmt"

	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"taskflow/internal/config"
	"taskflow/internal/models"
)

func Connect(cfg *config.Config, logger *zap.Logger) (*gorm.DB, error) {
	var (
		db  *gorm.DB
		err error
	)

	switch cfg.DBDriver {
	case "postgres":
		db, err = gorm.Open(postgres.Open(cfg.DBDSN), &gorm.Config{})
	case "sqlite":
		db, err = gorm.Open(sqlite.Open(cfg.DBDSN), &gorm.Config{})
	default:
		return nil, fmt.Errorf("unsupported db driver: %s", cfg.DBDriver)
	}

	if err != nil {
		return nil, err
	}

	logger.Info("database connected", zap.String("driver", cfg.DBDriver))
	return db, nil
}

func AutoMigrate(db *gorm.DB, logger *zap.Logger) error {
	if err := db.AutoMigrate(&models.Project{}, &models.Task{}, &models.TaskDependency{}); err != nil {
		return err
	}
	logger.Info("database migrations applied")
	return nil
}

