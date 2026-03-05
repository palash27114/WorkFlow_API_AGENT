package service

import (
	"context"
	"time"

	"taskflow/internal/graph"
	"taskflow/internal/models"
	"taskflow/internal/repository"
	pkgErrors "taskflow/pkg/errors"
)

type ProjectService interface {
	CreateProject(ctx context.Context, p *models.Project) error
	ListProjects(ctx context.Context) ([]models.Project, error)
	GetProject(ctx context.Context, id uint) (*models.Project, error)
	UpdateProject(ctx context.Context, p *models.Project) error
	DeleteProject(ctx context.Context, id uint) error

	GetExecutionPlan(ctx context.Context, projectID uint) ([]graph.ExecutionNode, error)
	GetStats(ctx context.Context, projectID uint) (*ProjectStats, error)
	GetRisks(ctx context.Context, projectID uint) ([]RiskyTask, error)
}

type ProjectStats struct {
	TotalEstimatedHours float64            `json:"total_estimated_hours"`
	CriticalPathHours   float64            `json:"critical_path_hours"`
	OverdueTasks        int               `json:"overdue_tasks"`
	WorkloadPerDay      map[string]float64 `json:"workload_per_day"`
}

type RiskyTask struct {
	TaskID uint   `json:"task_id"`
	Title  string `json:"title"`
	Reason string `json:"reason"`
}

type projectService struct {
	projectRepo repository.ProjectRepository
	taskRepo    repository.TaskRepository
	depRepo     repository.TaskDependencyRepository
}

func NewProjectService(
	projectRepo repository.ProjectRepository,
	taskRepo repository.TaskRepository,
	depRepo repository.TaskDependencyRepository,
) ProjectService {
	return &projectService{
		projectRepo: projectRepo,
		taskRepo:    taskRepo,
		depRepo:     depRepo,
	}
}

func (s *projectService) CreateProject(ctx context.Context, p *models.Project) error {
	return s.projectRepo.Create(ctx, p)
}

func (s *projectService) ListProjects(ctx context.Context) ([]models.Project, error) {
	return s.projectRepo.FindAll(ctx)
}

func (s *projectService) GetProject(ctx context.Context, id uint) (*models.Project, error) {
	p, err := s.projectRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, pkgErrors.ErrNotFound
	}
	return p, nil
}

func (s *projectService) UpdateProject(ctx context.Context, p *models.Project) error {
	// ensure project exists
	if _, err := s.GetProject(ctx, p.ID); err != nil {
		return err
	}
	return s.projectRepo.Update(ctx, p)
}

func (s *projectService) DeleteProject(ctx context.Context, id uint) error {
	// soft delete via GORM
	return s.projectRepo.Delete(ctx, id)
}

func (s *projectService) GetExecutionPlan(ctx context.Context, projectID uint) ([]graph.ExecutionNode, error) {
	tasks, err := s.taskRepo.FindByProjectID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	deps, err := s.depRepo.FindByProjectID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return graph.GetExecutionPlan(projectID, tasks, deps)
}

func (s *projectService) GetStats(ctx context.Context, projectID uint) (*ProjectStats, error) {
	tasks, err := s.taskRepo.FindByProjectID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	deps, err := s.depRepo.FindByProjectID(ctx, projectID)
	if err != nil {
		return nil, err
	}

	now := time.Now()

	stats := &ProjectStats{
		WorkloadPerDay: make(map[string]float64),
	}

	for _, t := range tasks {
		stats.TotalEstimatedHours += t.EstimatedHours
		if t.Deadline != nil && t.Status != models.TaskStatusCompleted && t.Deadline.Before(now) {
			stats.OverdueTasks++
		}

		// naive workload allocation: all hours on deadline date if present
		if t.Deadline != nil && t.EstimatedHours > 0 {
			day := t.Deadline.Format("2006-01-02")
			stats.WorkloadPerDay[day] += t.EstimatedHours
		}
	}

	// critical path: longest path in DAG using topological order
	plan, err := graph.GetExecutionPlan(projectID, tasks, deps)
	if err != nil {
		if err == pkgErrors.ErrCircularDependency {
			// if cycle, we still return other stats but critical path is zero
			return stats, nil
		}
		return nil, err
	}

	idToTask := make(map[uint]models.Task, len(tasks))
	for _, t := range tasks {
		idToTask[t.ID] = t
	}

	// build reverse adjacency (dependencies)
	parents := make(map[uint][]uint, len(tasks))
	for _, d := range deps {
		parents[d.TaskID] = append(parents[d.TaskID], d.DependsOnTaskID)
	}

	dist := make(map[uint]float64, len(tasks))
	for _, node := range plan {
		task := idToTask[node.TaskID]
		maxParent := 0.0
		for _, p := range parents[task.ID] {
			if dist[p] > maxParent {
				maxParent = dist[p]
			}
		}
		dist[task.ID] = maxParent + task.EstimatedHours
		if dist[task.ID] > stats.CriticalPathHours {
			stats.CriticalPathHours = dist[task.ID]
		}
	}

	return stats, nil
}

func (s *projectService) GetRisks(ctx context.Context, projectID uint) ([]RiskyTask, error) {
	tasks, err := s.taskRepo.FindByProjectID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	deps, err := s.depRepo.FindByProjectID(ctx, projectID)
	if err != nil {
		return nil, err
	}

	depsByTask := make(map[uint][]uint)
	for _, d := range deps {
		depsByTask[d.TaskID] = append(depsByTask[d.TaskID], d.DependsOnTaskID)
	}

	idToTask := make(map[uint]models.Task, len(tasks))
	for _, t := range tasks {
		idToTask[t.ID] = t
	}

	now := time.Now()
	const hoursPerDay = 8.0

	var risks []RiskyTask

	for _, t := range tasks {
		if t.Deadline == nil || t.Status == models.TaskStatusCompleted {
			continue
		}

		remainingHours := t.EstimatedHours
		if remainingHours <= 0 {
			continue
		}

		daysUntilDeadline := t.Deadline.Sub(now).Hours() / 24
		if daysUntilDeadline <= 0 {
			risks = append(risks, RiskyTask{
				TaskID: t.ID,
				Title:  t.Title,
				Reason: "deadline passed with remaining work",
			})
			continue
		}

		loadPerDay := remainingHours / daysUntilDeadline
		if loadPerDay > hoursPerDay {
			risks = append(risks, RiskyTask{
				TaskID: t.ID,
				Title:  t.Title,
				Reason: "required daily workload exceeds capacity",
			})
			continue
		}

		// check if any dependency is itself overdue or close to deadline
		for _, depID := range depsByTask[t.ID] {
			depTask, ok := idToTask[depID]
			if !ok || depTask.Deadline == nil {
				continue
			}
			if depTask.Deadline.Before(now) && depTask.Status != models.TaskStatusCompleted {
				risks = append(risks, RiskyTask{
					TaskID: t.ID,
					Title:  t.Title,
					Reason: "blocked by overdue dependency",
				})
				break
			}
		}
	}

	return risks, nil
}

