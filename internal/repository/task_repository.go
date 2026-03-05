package repository

import (
	"context"

	"gorm.io/gorm"

	"taskflow/internal/models"
)

type TaskRepository interface {
	Create(ctx context.Context, task *models.Task) error
	FindByID(ctx context.Context, id uint) (*models.Task, error)
	FindByProjectID(ctx context.Context, projectID uint) ([]models.Task, error)
	Update(ctx context.Context, task *models.Task) error
	Delete(ctx context.Context, id uint) error
}

type taskRepository struct {
	db *gorm.DB
}

func NewTaskRepository(db *gorm.DB) TaskRepository {
	return &taskRepository{db: db}
}

func (r *taskRepository) Create(ctx context.Context, task *models.Task) error {
	return r.db.WithContext(ctx).Create(task).Error
}

func (r *taskRepository) FindByID(ctx context.Context, id uint) (*models.Task, error) {
	var task models.Task
	if err := r.db.WithContext(ctx).Preload("Dependencies").First(&task, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &task, nil
}

func (r *taskRepository) FindByProjectID(ctx context.Context, projectID uint) ([]models.Task, error) {
	var tasks []models.Task
	if err := r.db.WithContext(ctx).Where("project_id = ?", projectID).Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}

func (r *taskRepository) Update(ctx context.Context, task *models.Task) error {
	return r.db.WithContext(ctx).Save(task).Error
}

func (r *taskRepository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&models.Task{}, id).Error
}

