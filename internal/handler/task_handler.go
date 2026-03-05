package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"taskflow/internal/models"
	"taskflow/internal/service"
	"taskflow/internal/validator"
	pkgErrors "taskflow/pkg/errors"
	"taskflow/pkg/response"
)

type TaskHandler struct {
	svc       service.TaskService
	logger    *zap.Logger
	validator validator.Validator
}

func NewTaskHandler(svc service.TaskService, logger *zap.Logger, v validator.Validator) *TaskHandler {
	return &TaskHandler{
		svc:       svc,
		logger:    logger,
		validator: v,
	}
}

// Routes is kept for compatibility but no longer used for top-level /tasks mounting.
// All task endpoints are now nested under /projects/{project_id}/tasks.
func (h *TaskHandler) Routes() chi.Router {
	return chi.NewRouter()
}

type createTaskRequest struct {
	Title          string                `json:"title" validate:"required"`
	Description    string                `json:"description"`
	Status         models.TaskStatus     `json:"status"`
	Priority       models.TaskPriority   `json:"priority"`
	EstimatedHours float64               `json:"estimated_hours"`
	Deadline       *time.Time            `json:"deadline"`
}

type updateTaskRequest struct {
	Title          string                `json:"title" validate:"required"`
	Description    string                `json:"description"`
	Status         models.TaskStatus     `json:"status" validate:"required"`
	Priority       models.TaskPriority   `json:"priority"`
	EstimatedHours float64               `json:"estimated_hours"`
	Deadline       *time.Time            `json:"deadline"`
}

type addDependencyRequest struct {
	DependsOnTaskID uint `json:"depends_on_task_id" validate:"required"`
}

// CreateTaskForProject godoc
// @Summary Create task
// @Tags tasks
// @Accept json
// @Produce json
// @Param project_id path int true "Project ID"
// @Param task body createTaskRequest true "Task"
// @Success 201 {object} models.Task
// @Failure 400 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /projects/{project_id}/tasks [post]
func (h *TaskHandler) CreateTaskForProject(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseUintParam(chi.URLParam(r, "project_id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid project id")
		return
	}

	var req createTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	if err := h.validator.Struct(req); err != nil {
		response.Error(w, http.StatusBadRequest, "validation failed")
		return
	}

	status := req.Status
	if status == "" {
		status = models.TaskStatusPending
	}
	priority := req.Priority
	if priority == "" {
		priority = models.TaskPriorityMedium
	}

	task := &models.Task{
		ProjectID:      projectID,
		Title:          req.Title,
		Description:    req.Description,
		Status:         status,
		Priority:       priority,
		EstimatedHours: req.EstimatedHours,
		Deadline:       req.Deadline,
	}

	if err := h.svc.CreateTask(r.Context(), task); err != nil {
		h.logger.Error("create task failed", zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "could not create task")
		return
	}

	response.JSON(w, http.StatusCreated, task)
}

// ListTasksForProject godoc
// @Summary List project tasks
// @Tags tasks
// @Produce json
// @Param project_id path int true "Project ID"
// @Success 200 {array} models.Task
// @Failure 400 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /projects/{project_id}/tasks [get]
func (h *TaskHandler) ListTasksForProject(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseUintParam(chi.URLParam(r, "project_id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid project id")
		return
	}

	tasks, err := h.svc.ListTasksByProject(r.Context(), projectID)
	if err != nil {
		h.logger.Error("list tasks failed", zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "could not list tasks")
		return
	}

	response.JSON(w, http.StatusOK, tasks)
}

// GetTask godoc
// @Summary Get task within a project
// @Tags tasks
// @Produce json
// @Param project_id path int true "Project ID"
// @Param id path int true "Task ID"
// @Success 200 {object} models.Task
// @Failure 404 {object} response.ErrorResponse
// @Router /projects/{project_id}/tasks/{id} [get]
func (h *TaskHandler) GetTask(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseUintParam(chi.URLParam(r, "project_id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid project id")
		return
	}

	id, err := parseUintParam(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	task, err := h.svc.GetTask(r.Context(), id)
	if err != nil {
		if err == pkgErrors.ErrNotFound {
			response.Error(w, http.StatusNotFound, "task not found")
			return
		}
		h.logger.Error("get task failed", zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "could not get task")
		return
	}

	if task.ProjectID != projectID {
		response.Error(w, http.StatusNotFound, "task not found in specified project")
		return
	}

	response.JSON(w, http.StatusOK, task)
}

// UpdateTask godoc
// @Summary Update task within a project
// @Tags tasks
// @Accept json
// @Produce json
// @Param project_id path int true "Project ID"
// @Param id path int true "Task ID"
// @Param task body updateTaskRequest true "Task"
// @Success 200 {object} models.Task
// @Failure 400 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /projects/{project_id}/tasks/{id} [put]
func (h *TaskHandler) UpdateTask(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseUintParam(chi.URLParam(r, "project_id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid project id")
		return
	}

	id, err := parseUintParam(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	var req updateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	if err := h.validator.Struct(req); err != nil {
		response.Error(w, http.StatusBadRequest, "validation failed")
		return
	}

	task := &models.Task{
		ID:             id,
		Title:          req.Title,
		Description:    req.Description,
		Status:         req.Status,
		Priority:       req.Priority,
		EstimatedHours: req.EstimatedHours,
		Deadline:       req.Deadline,
	}

	if err := h.svc.UpdateTask(r.Context(), task); err != nil {
		switch err {
		case pkgErrors.ErrNotFound:
			response.Error(w, http.StatusNotFound, "task not found")
			return
		case pkgErrors.ErrInvalidStatusTransition:
			response.Error(w, http.StatusBadRequest, "invalid status transition")
			return
		case pkgErrors.ErrBlockedByDependencies:
			response.Error(w, http.StatusBadRequest, "task is blocked by incomplete dependencies")
			return
		case pkgErrors.ErrDeadlineConstraint:
			response.Error(w, http.StatusBadRequest, "deadline must be after all dependency deadlines")
			return
		default:
			h.logger.Error("update task failed", zap.Error(err))
			response.Error(w, http.StatusInternalServerError, "could not update task")
			return
		}
	}

	updated, err := h.svc.GetTask(r.Context(), id)
	if err != nil {
		h.logger.Error("get updated task failed", zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "could not fetch updated task")
		return
	}
	if updated.ProjectID != projectID {
		response.Error(w, http.StatusNotFound, "task not found in specified project")
		return
	}

	response.JSON(w, http.StatusOK, updated)
}

// DeleteTask godoc
// @Summary Delete task within a project
// @Tags tasks
// @Param project_id path int true "Project ID"
// @Param id path int true "Task ID"
// @Success 204 "No Content"
// @Router /projects/{project_id}/tasks/{id} [delete]
func (h *TaskHandler) DeleteTask(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseUintParam(chi.URLParam(r, "project_id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid project id")
		return
	}

	id, err := parseUintParam(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	task, err := h.svc.GetTask(r.Context(), id)
	if err != nil {
		if err == pkgErrors.ErrNotFound {
			response.Error(w, http.StatusNotFound, "task not found")
			return
		}
		h.logger.Error("get task before delete failed", zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "could not fetch task")
		return
	}
	if task.ProjectID != projectID {
		response.Error(w, http.StatusNotFound, "task not found in specified project")
		return
	}

	if err := h.svc.DeleteTask(r.Context(), id); err != nil {
		h.logger.Error("delete task failed", zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "could not delete task")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// AddDependencyForProject godoc
// @Summary Add task dependency within a project
// @Tags tasks
// @Accept json
// @Produce json
// @Param project_id path int true "Project ID"
// @Param id path int true "Task ID"
// @Param body body addDependencyRequest true "Dependency"
// @Success 201 "Created"
// @Failure 400 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /projects/{project_id}/tasks/{id}/dependencies [post]
func (h *TaskHandler) AddDependencyForProject(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseUintParam(chi.URLParam(r, "project_id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid project id")
		return
	}

	taskID, err := parseUintParam(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid task id")
		return
	}

	var req addDependencyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	if err := h.validator.Struct(req); err != nil {
		response.Error(w, http.StatusBadRequest, "validation failed")
		return
	}

	// ensure the task belongs to the given project
	task, err := h.svc.GetTask(r.Context(), taskID)
	if err != nil {
		if err == pkgErrors.ErrNotFound {
			response.Error(w, http.StatusNotFound, "task not found")
			return
		}
		h.logger.Error("get task for dependency failed", zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "could not fetch task")
		return
	}
	if task.ProjectID != projectID {
		response.Error(w, http.StatusBadRequest, "task does not belong to specified project")
		return
	}

	if err := h.svc.AddDependency(r.Context(), taskID, req.DependsOnTaskID); err != nil {
		switch err {
		case pkgErrors.ErrNotFound:
			response.Error(w, http.StatusNotFound, "task not found")
			return
		default:
			h.logger.Error("add dependency failed", zap.Error(err))
			response.Error(w, http.StatusInternalServerError, "could not add dependency")
			return
		}
	}

	w.WriteHeader(http.StatusCreated)
}

