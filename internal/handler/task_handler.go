package handler

import (
	"encoding/json"
	"errors"
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
		response.ErrorWithCode(w, http.StatusBadRequest, response.CodeInvalidRequest, "Invalid project ID in URL. Please use a positive number.")
		return
	}

	var req createTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.ErrorWithCode(w, http.StatusBadRequest, response.CodeInvalidRequest, "Invalid JSON in request body. Please send a valid JSON object with at least 'title' and optional description, status, priority, estimated_hours, deadline.")
		return
	}

	if err := h.validator.Struct(req); err != nil {
		msg := validationErrorMessage(err)
		response.ErrorWithCode(w, http.StatusBadRequest, response.CodeValidationError, msg)
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
		response.ErrorWithCode(w, http.StatusInternalServerError, response.CodeInternalError, "Could not create task. Ensure the project exists and try again.")
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
		response.ErrorWithCode(w, http.StatusBadRequest, response.CodeInvalidRequest, "Invalid project ID in URL. Please use a positive number.")
		return
	}

	tasks, err := h.svc.ListTasksByProject(r.Context(), projectID)
	if err != nil {
		h.logger.Error("list tasks failed", zap.Error(err))
		response.ErrorWithCode(w, http.StatusInternalServerError, response.CodeInternalError, "Could not list tasks. Please try again later.")
		return
	}
	if len(tasks) == 0 {
		response.ErrorWithCode(w, http.StatusNotFound, response.CodeEmptyResult, "There is nothing to show. This project has no tasks yet. Add tasks first.")
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
		response.ErrorWithCode(w, http.StatusBadRequest, response.CodeInvalidRequest, "Invalid project ID in URL. Please use a positive number.")
		return
	}

	id, err := parseUintParam(chi.URLParam(r, "id"))
	if err != nil {
		response.ErrorWithCode(w, http.StatusBadRequest, response.CodeInvalidRequest, "Invalid task ID in URL. Please use a positive number.")
		return
	}

	task, err := h.svc.GetTask(r.Context(), id)
	if err != nil {
		if errors.Is(err, pkgErrors.ErrNotFound) {
			response.ErrorWithCode(w, http.StatusNotFound, response.CodeNotFound, "Task not found. Check that the task ID exists.")
			return
		}
		h.logger.Error("get task failed", zap.Error(err))
		response.ErrorWithCode(w, http.StatusInternalServerError, response.CodeInternalError, "Could not fetch task. Please try again later.")
		return
	}

	if task.ProjectID != projectID {
		response.ErrorWithCode(w, http.StatusNotFound, response.CodeTaskNotInProject, "Task not found in this project. The task ID may belong to a different project.")
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
		response.ErrorWithCode(w, http.StatusBadRequest, response.CodeInvalidRequest, "Invalid project ID in URL. Please use a positive number.")
		return
	}

	id, err := parseUintParam(chi.URLParam(r, "id"))
	if err != nil {
		response.ErrorWithCode(w, http.StatusBadRequest, response.CodeInvalidRequest, "Invalid task ID in URL. Please use a positive number.")
		return
	}

	var req updateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.ErrorWithCode(w, http.StatusBadRequest, response.CodeInvalidRequest, "Invalid JSON in request body. Send a valid JSON object with title, status, and optional description, priority, estimated_hours, deadline.")
		return
	}

	if err := h.validator.Struct(req); err != nil {
		msg := validationErrorMessage(err)
		response.ErrorWithCode(w, http.StatusBadRequest, response.CodeValidationError, msg)
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
		switch {
		case errors.Is(err, pkgErrors.ErrNotFound):
			response.ErrorWithCode(w, http.StatusNotFound, response.CodeNotFound, "Task not found. Check that the task ID exists.")
			return
		case errors.Is(err, pkgErrors.ErrInvalidStatusTransition):
			response.ErrorWithCode(w, http.StatusBadRequest, response.CodeInvalidStatusTransition, "Invalid status transition. Allowed: pending→in_progress or blocked, in_progress→completed, blocked→pending.")
			return
		case errors.Is(err, pkgErrors.ErrBlockedByDependencies):
			response.ErrorWithCode(w, http.StatusBadRequest, response.CodeBlockedByDependencies, "Cannot move task to in_progress: one or more dependency tasks are not completed. Complete all dependencies first.")
			return
		case errors.Is(err, pkgErrors.ErrDeadlineConstraint):
			response.ErrorWithCode(w, http.StatusBadRequest, response.CodeDeadlineConstraint, "Task deadline must be after the deadlines of all tasks it depends on. Adjust the deadline or dependency order.")
			return
		default:
			h.logger.Error("update task failed", zap.Error(err))
			response.ErrorWithCode(w, http.StatusInternalServerError, response.CodeInternalError, "Could not update task. Please try again later.")
			return
		}
	}

	updated, err := h.svc.GetTask(r.Context(), id)
	if err != nil {
		h.logger.Error("get updated task failed", zap.Error(err))
		response.ErrorWithCode(w, http.StatusInternalServerError, response.CodeInternalError, "Could not fetch updated task. Please try again later.")
		return
	}
	if updated.ProjectID != projectID {
		response.ErrorWithCode(w, http.StatusNotFound, response.CodeTaskNotInProject, "Task not found in this project. The task ID may belong to a different project.")
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
		response.ErrorWithCode(w, http.StatusBadRequest, response.CodeInvalidRequest, "Invalid project ID in URL. Please use a positive number.")
		return
	}

	id, err := parseUintParam(chi.URLParam(r, "id"))
	if err != nil {
		response.ErrorWithCode(w, http.StatusBadRequest, response.CodeInvalidRequest, "Invalid task ID in URL. Please use a positive number.")
		return
	}

	task, err := h.svc.GetTask(r.Context(), id)
	if err != nil {
		if errors.Is(err, pkgErrors.ErrNotFound) {
			response.ErrorWithCode(w, http.StatusNotFound, response.CodeNotFound, "Task not found. Check that the task ID exists.")
			return
		}
		h.logger.Error("get task before delete failed", zap.Error(err))
		response.ErrorWithCode(w, http.StatusInternalServerError, response.CodeInternalError, "Could not fetch task. Please try again later.")
		return
	}
	if task.ProjectID != projectID {
		response.ErrorWithCode(w, http.StatusNotFound, response.CodeTaskNotInProject, "Task not found in this project. The task ID may belong to a different project.")
		return
	}

	if err := h.svc.DeleteTask(r.Context(), id); err != nil {
		h.logger.Error("delete task failed", zap.Error(err))
		response.ErrorWithCode(w, http.StatusInternalServerError, response.CodeInternalError, "Could not delete task. Please try again later.")
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
		response.ErrorWithCode(w, http.StatusBadRequest, response.CodeInvalidRequest, "Invalid project ID in URL. Please use a positive number.")
		return
	}

	taskID, err := parseUintParam(chi.URLParam(r, "id"))
	if err != nil {
		response.ErrorWithCode(w, http.StatusBadRequest, response.CodeInvalidRequest, "Invalid task ID in URL. Please use a positive number.")
		return
	}

	var req addDependencyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.ErrorWithCode(w, http.StatusBadRequest, response.CodeInvalidRequest, "Invalid JSON in request body. Send a valid JSON object with 'depends_on_task_id' (the task ID this task depends on).")
		return
	}

	if err := h.validator.Struct(req); err != nil {
		msg := validationErrorMessage(err)
		response.ErrorWithCode(w, http.StatusBadRequest, response.CodeValidationError, msg)
		return
	}

	// ensure the task belongs to the given project
	task, err := h.svc.GetTask(r.Context(), taskID)
	if err != nil {
		if errors.Is(err, pkgErrors.ErrNotFound) {
			response.ErrorWithCode(w, http.StatusNotFound, response.CodeNotFound, "Task not found. Check that the task ID in the URL exists.")
			return
		}
		h.logger.Error("get task for dependency failed", zap.Error(err))
		response.ErrorWithCode(w, http.StatusInternalServerError, response.CodeInternalError, "Could not fetch task. Please try again later.")
		return
	}
	if task.ProjectID != projectID {
		response.ErrorWithCode(w, http.StatusBadRequest, response.CodeTaskNotInProject, "This task does not belong to the specified project. Use the correct project ID.")
		return
	}

	if err := h.svc.AddDependency(r.Context(), taskID, req.DependsOnTaskID); err != nil {
		switch {
		case errors.Is(err, pkgErrors.ErrNotFound):
			response.ErrorWithCode(w, http.StatusNotFound, response.CodeNotFound, "Dependency task not found. Check that 'depends_on_task_id' exists and belongs to the same project.")
			return
		case errors.Is(err, pkgErrors.ErrCircularDependency):
			response.ErrorWithCode(w, http.StatusBadRequest, response.CodeCircularDependency, "You cannot add this dependency: it would create a circular dependency (e.g. A→B→C→A). Ensure task dependencies do not form a cycle.")
			return
		case errors.Is(err, pkgErrors.ErrDependencyExists):
			response.ErrorWithCode(w, http.StatusBadRequest, response.CodeDependencyAlreadyExists, "This dependency already exists. The task already depends on the given task.")
			return
		default:
			h.logger.Error("add dependency failed", zap.Error(err))
			response.ErrorWithCode(w, http.StatusInternalServerError, response.CodeInternalError, "Could not add dependency. Please try again later.")
			return
		}
	}

	w.WriteHeader(http.StatusCreated)
}

