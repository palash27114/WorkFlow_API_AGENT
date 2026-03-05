package repository

import (
	"context"

	"gorm.io/gorm"

	"taskflow/internal/models"
)

type TaskDependencyRepository interface {
	Create(ctx context.Context, dep *models.TaskDependency) error
	FindByTaskID(ctx context.Context, taskID uint) ([]models.TaskDependency, error)
	FindByProjectID(ctx context.Context, projectID uint) ([]models.TaskDependency, error)
}

type taskDependencyRepository struct {
	db *gorm.DB
}

func NewTaskDependencyRepository(db *gorm.DB) TaskDependencyRepository {
	return &taskDependencyRepository{db: db}
}

func (r *taskDependencyRepository) Create(ctx context.Context, dep *models.TaskDependency) error {
	return r.db.WithContext(ctx).Create(dep).Error
}

func (r *taskDependencyRepository) FindByTaskID(ctx context.Context, taskID uint) ([]models.TaskDependency, error) {
	var deps []models.TaskDependency
	if err := r.db.WithContext(ctx).Where("task_id = ?", taskID).Find(&deps).Error; err != nil {
		return nil, err
	}
	return deps, nil
}

func (r *taskDependencyRepository) FindByProjectID(ctx context.Context, projectID uint) ([]models.TaskDependency, error) {
	var deps []models.TaskDependency
	if err := r.db.WithContext(ctx).
		Joins("JOIN tasks ON tasks.id = task_dependencies.task_id").
		Where("tasks.project_id = ?", projectID).
		Find(&deps).Error; err != nil {
		return nil, err
	}
	return deps, nil
}

