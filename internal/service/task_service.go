package service

import (
	"context"
	"time"

	"taskflow/internal/models"
	"taskflow/internal/repository"
	pkgErrors "taskflow/pkg/errors"
)

type TaskService interface {
	CreateTask(ctx context.Context, task *models.Task) error
	ListTasksByProject(ctx context.Context, projectID uint) ([]models.Task, error)
	GetTask(ctx context.Context, id uint) (*models.Task, error)
	UpdateTask(ctx context.Context, task *models.Task) error
	DeleteTask(ctx context.Context, id uint) error

	AddDependency(ctx context.Context, taskID uint, dependsOnTaskID uint) error
}

type taskService struct {
	taskRepo    repository.TaskRepository
	depRepo     repository.TaskDependencyRepository
	projectRepo repository.ProjectRepository
}

func NewTaskService(
	taskRepo repository.TaskRepository,
	depRepo repository.TaskDependencyRepository,
	projectRepo repository.ProjectRepository,
) TaskService {
	return &taskService{
		taskRepo:    taskRepo,
		depRepo:     depRepo,
		projectRepo: projectRepo,
	}
}

func (s *taskService) CreateTask(ctx context.Context, task *models.Task) error {
	// ensure project exists
	if _, err := s.projectRepo.FindByID(ctx, task.ProjectID); err != nil {
		return err
	}
	return s.taskRepo.Create(ctx, task)
}

func (s *taskService) ListTasksByProject(ctx context.Context, projectID uint) ([]models.Task, error) {
	return s.taskRepo.FindByProjectID(ctx, projectID)
}

func (s *taskService) GetTask(ctx context.Context, id uint) (*models.Task, error) {
	task, err := s.taskRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, pkgErrors.ErrNotFound
	}
	return task, nil
}

func (s *taskService) UpdateTask(ctx context.Context, updated *models.Task) error {
	current, err := s.GetTask(ctx, updated.ID)
	if err != nil {
		return err
	}

	// enforce state machine rules
	if err := validateStatusTransition(current.Status, updated.Status); err != nil {
		return err
	}

	// when moving to in_progress, ensure all dependencies completed
	if current.Status != models.TaskStatusInProgress && updated.Status == models.TaskStatusInProgress {
		deps, err := s.depRepo.FindByTaskID(ctx, updated.ID)
		if err != nil {
			return err
		}

		for _, d := range deps {
			depTask, err := s.taskRepo.FindByID(ctx, d.DependsOnTaskID)
			if err != nil {
				return err
			}
			if depTask == nil || depTask.Status != models.TaskStatusCompleted {
				return pkgErrors.ErrBlockedByDependencies
			}
		}
	}

	// enforce deadline rule: deadline after all dependency deadlines
	if updated.Deadline != nil {
		if err := s.ensureDeadlineAfterDependencies(ctx, updated); err != nil {
			return err
		}
	}

	// copy mutable fields
	current.Title = updated.Title
	current.Description = updated.Description
	current.Status = updated.Status
	current.Priority = updated.Priority
	current.EstimatedHours = updated.EstimatedHours
	current.Deadline = updated.Deadline

	return s.taskRepo.Update(ctx, current)
}

func (s *taskService) DeleteTask(ctx context.Context, id uint) error {
	return s.taskRepo.Delete(ctx, id)
}

func (s *taskService) AddDependency(ctx context.Context, taskID uint, dependsOnTaskID uint) error {
	// ensure both tasks exist
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	depTask, err := s.GetTask(ctx, dependsOnTaskID)
	if err != nil {
		return err
	}

	// must belong to same project
	if task.ProjectID != depTask.ProjectID {
		return pkgErrors.ErrDeadlineConstraint
	}

	dep := &models.TaskDependency{
		TaskID:          taskID,
		DependsOnTaskID: dependsOnTaskID,
	}
	return s.depRepo.Create(ctx, dep)
}

func validateStatusTransition(from, to models.TaskStatus) error {
	if from == to {
		return nil
	}

	switch from {
	case models.TaskStatusPending:
		if to == models.TaskStatusInProgress || to == models.TaskStatusBlocked {
			return nil
		}
	case models.TaskStatusInProgress:
		if to == models.TaskStatusCompleted {
			return nil
		}
	case models.TaskStatusBlocked:
		if to == models.TaskStatusPending {
			return nil
		}
	}

	return pkgErrors.ErrInvalidStatusTransition
}

func (s *taskService) ensureDeadlineAfterDependencies(ctx context.Context, task *models.Task) error {
	if task.Deadline == nil {
		return nil
	}

	deps, err := s.depRepo.FindByTaskID(ctx, task.ID)
	if err != nil {
		return err
	}

	for _, d := range deps {
		depTask, err := s.taskRepo.FindByID(ctx, d.DependsOnTaskID)
		if err != nil {
			return err
		}
		if depTask == nil || depTask.Deadline == nil {
			continue
		}
		if task.Deadline.Before(*depTask.Deadline) || task.Deadline.Equal(depTask.Deadline.Truncate(time.Second)) {
			return pkgErrors.ErrDeadlineConstraint
		}
	}

	return nil
}

