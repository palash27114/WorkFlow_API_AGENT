package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	goValidator "github.com/go-playground/validator/v10"
	"go.uber.org/zap"

	"taskflow/internal/models"
	"taskflow/internal/service"
	"taskflow/internal/validator"
	pkgErrors "taskflow/pkg/errors"
	"taskflow/pkg/response"
)

type ProjectHandler struct {
	svc       service.ProjectService
	logger    *zap.Logger
	validator validator.Validator
}

func NewProjectHandler(svc service.ProjectService, logger *zap.Logger, v validator.Validator) *ProjectHandler {
	return &ProjectHandler{
		svc:       svc,
		logger:    logger,
		validator: v,
	}
}

func (h *ProjectHandler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Post("/", h.CreateProject)
	r.Get("/", h.ListProjects)
	r.Get("/{id}", h.GetProject)
	r.Put("/{id}", h.UpdateProject)
	r.Delete("/{id}", h.DeleteProject)
	r.Get("/{id}/execution-plan", h.ExecutionPlan)
	r.Get("/{id}/stats", h.Stats)
	r.Get("/{id}/risks", h.Risks)

	return r
}

type createProjectRequest struct {
	Name        string `json:"name" validate:"required"`
	Description string `json:"description"`
}

// CreateProject godoc
// @Summary Create project
// @Description Create a new project
// @Tags projects
// @Accept json
// @Produce json
// @Param project body createProjectRequest true "Project"
// @Success 201 {object} models.Project
// @Failure 400 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /projects [post]
func (h *ProjectHandler) CreateProject(w http.ResponseWriter, r *http.Request) {
	var req createProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.ErrorWithCode(w, http.StatusBadRequest, response.CodeInvalidRequest, "Invalid JSON in request body. Please send a valid JSON object with 'name' and optional 'description'.")
		return
	}

	if err := h.validator.Struct(req); err != nil {
		msg := validationErrorMessage(err)
		response.ErrorWithCode(w, http.StatusBadRequest, response.CodeValidationError, msg)
		return
	}

	project := &models.Project{
		Name:        req.Name,
		Description: req.Description,
	}

	if err := h.svc.CreateProject(r.Context(), project); err != nil {
		h.logger.Error("create project failed", zap.Error(err))
		response.ErrorWithCode(w, http.StatusInternalServerError, response.CodeInternalError, "Could not create project. Please try again later.")
		return
	}

	response.JSON(w, http.StatusCreated, project)
}

// ListProjects godoc
// @Summary List projects
// @Tags projects
// @Produce json
// @Success 200 {array} models.Project
// @Router /projects [get]
func (h *ProjectHandler) ListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := h.svc.ListProjects(r.Context())
	if err != nil {
		h.logger.Error("list projects failed", zap.Error(err))
		response.ErrorWithCode(w, http.StatusInternalServerError, response.CodeInternalError, "Could not list projects. Please try again later.")
		return
	}
	if len(projects) == 0 {
		response.ErrorWithCode(w, http.StatusNotFound, response.CodeEmptyResult, "There are no projects. Create a project first.")
		return
	}
	response.JSON(w, http.StatusOK, projects)
}

// GetProject godoc
// @Summary Get project
// @Tags projects
// @Produce json
// @Param id path int true "Project ID"
// @Success 200 {object} models.Project
// @Failure 404 {object} response.ErrorResponse
// @Router /projects/{id} [get]

func (h *ProjectHandler) GetProject(w http.ResponseWriter, r *http.Request) {
	id, err := parseUintParam(chi.URLParam(r, "id"))
	if err != nil {
		response.ErrorWithCode(w, http.StatusBadRequest, response.CodeInvalidRequest, "Invalid project ID in URL. Please use a positive number.")
		return
	}

	project, err := h.svc.GetProject(r.Context(), id)
	if err != nil {
		if errors.Is(err, pkgErrors.ErrNotFound) {
			response.ErrorWithCode(w, http.StatusNotFound, response.CodeNotFound, "Project not found. Check that the project ID exists.")
			return
		}
		h.logger.Error("get project failed", zap.Error(err))
		response.ErrorWithCode(w, http.StatusInternalServerError, response.CodeInternalError, "Could not fetch project. Please try again later.")
		return
	}

	response.JSON(w, http.StatusOK, project)
}

type updateProjectRequest struct {
	Name        string `json:"name" validate:"required"`
	Description string `json:"description"`
}

// UpdateProject godoc
// @Summary Update project
// @Tags projects
// @Accept json
// @Produce json
// @Param id path int true "Project ID"
// @Param project body updateProjectRequest true "Project"
// @Success 200 {object} models.Project
// @Failure 400 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /projects/{id} [put]
func (h *ProjectHandler) UpdateProject(w http.ResponseWriter, r *http.Request) {
	id, err := parseUintParam(chi.URLParam(r, "id"))
	if err != nil {
		response.ErrorWithCode(w, http.StatusBadRequest, response.CodeInvalidRequest, "Invalid project ID in URL. Please use a positive number.")
		return
	}

	var req updateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.ErrorWithCode(w, http.StatusBadRequest, response.CodeInvalidRequest, "Invalid JSON in request body. Please send a valid JSON object with 'name' and optional 'description'.")
		return
	}

	if err := h.validator.Struct(req); err != nil {
		msg := validationErrorMessage(err)
		response.ErrorWithCode(w, http.StatusBadRequest, response.CodeValidationError, msg)
		return
	}

	project := &models.Project{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
	}

	if err := h.svc.UpdateProject(r.Context(), project); err != nil {
		if errors.Is(err, pkgErrors.ErrNotFound) {
			response.ErrorWithCode(w, http.StatusNotFound, response.CodeNotFound, "Project not found. Check that the project ID exists.")
			return
		}
		h.logger.Error("update project failed", zap.Error(err))
		response.ErrorWithCode(w, http.StatusInternalServerError, response.CodeInternalError, "Could not update project. Please try again later.")
		return
	}

	response.JSON(w, http.StatusOK, project)
}

// DeleteProject godoc
// @Summary Delete project
// @Tags projects
// @Param id path int true "Project ID"
// @Success 204 "No Content"
// @Failure 404 {object} response.ErrorResponse
// @Router /projects/{id} [delete]
func (h *ProjectHandler) DeleteProject(w http.ResponseWriter, r *http.Request) {
	id, err := parseUintParam(chi.URLParam(r, "id"))
	if err != nil {
		response.ErrorWithCode(w, http.StatusBadRequest, response.CodeInvalidRequest, "Invalid project ID in URL. Please use a positive number.")
		return
	}

	if err := h.svc.DeleteProject(r.Context(), id); err != nil {
		h.logger.Error("delete project failed", zap.Error(err))
		response.ErrorWithCode(w, http.StatusInternalServerError, response.CodeInternalError, "Could not delete project. Please try again later.")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ExecutionPlan godoc
// @Summary Get execution plan
// @Tags projects
// @Produce json
// @Param id path int true "Project ID"
// @Success 200 {array} graph.ExecutionNode
// @Failure 400 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /projects/{id}/execution-plan [get]
func (h *ProjectHandler) ExecutionPlan(w http.ResponseWriter, r *http.Request) {
	id, err := parseUintParam(chi.URLParam(r, "id"))
	if err != nil {
		response.ErrorWithCode(w, http.StatusBadRequest, response.CodeInvalidRequest, "Invalid project ID in URL. Please use a positive number.")
		return
	}

	plan, err := h.svc.GetExecutionPlan(r.Context(), id)
	if err != nil {
		if errors.Is(err, pkgErrors.ErrCircularDependency) {
			response.ErrorWithCode(w, http.StatusBadRequest, response.CodeCircularDependency, "Cannot compute execution plan: circular dependency detected. Ensure task dependencies do not form a cycle (e.g. A depends on B, B depends on C, C depends on A).")
			return
		}
		h.logger.Error("get execution plan failed", zap.Error(err))
		response.ErrorWithCode(w, http.StatusInternalServerError, response.CodeInternalError, "Could not compute execution plan. Please try again later.")
		return
	}
	if len(plan) == 0 {
		response.ErrorWithCode(w, http.StatusNotFound, response.CodeEmptyResult, "There is nothing to show. This project has no tasks yet. Add tasks first to get an execution plan.")
		return
	}
	response.JSON(w, http.StatusOK, plan)
}

// Stats godoc
// @Summary Get project stats
// @Tags projects
// @Produce json
// @Param id path int true "Project ID"
// @Success 200 {object} service.ProjectStats
// @Failure 500 {object} response.ErrorResponse
// @Router /projects/{id}/stats [get]
func (h *ProjectHandler) Stats(w http.ResponseWriter, r *http.Request) {
	id, err := parseUintParam(chi.URLParam(r, "id"))
	if err != nil {
		response.ErrorWithCode(w, http.StatusBadRequest, response.CodeInvalidRequest, "Invalid project ID in URL. Please use a positive number.")
		return
	}

	stats, err := h.svc.GetStats(r.Context(), id)
	if err != nil {
		h.logger.Error("get stats failed", zap.Error(err))
		response.ErrorWithCode(w, http.StatusInternalServerError, response.CodeInternalError, "Could not compute project stats. Please try again later.")
		return
	}

	response.JSON(w, http.StatusOK, stats)
}

// Risks godoc
// @Summary Get project risks
// @Tags projects
// @Produce json
// @Param id path int true "Project ID"
// @Success 200 {array} service.RiskyTask
// @Failure 500 {object} response.ErrorResponse
// @Router /projects/{id}/risks [get]
func (h *ProjectHandler) Risks(w http.ResponseWriter, r *http.Request) {
	id, err := parseUintParam(chi.URLParam(r, "id"))
	if err != nil {
		response.ErrorWithCode(w, http.StatusBadRequest, response.CodeInvalidRequest, "Invalid project ID in URL. Please use a positive number.")
		return
	}

	risks, err := h.svc.GetRisks(r.Context(), id)
	if err != nil {
		h.logger.Error("get risks failed", zap.Error(err))
		response.ErrorWithCode(w, http.StatusInternalServerError, response.CodeInternalError, "Could not compute project risks. Please try again later.")
		return
	}
	if len(risks) == 0 {
		response.ErrorWithCode(w, http.StatusNotFound, response.CodeEmptyResult, "There is nothing to show. No risks detected for this project.")
		return
	}
	response.JSON(w, http.StatusOK, risks)
}

func parseUintParam(value string) (uint, error) {
	id64, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, err
	}
	return uint(id64), nil
}

// validationErrorMessage builds a user-friendly message from validator errors (e.g. "Field 'name' is required; Field 'title' must be a valid value.").
func validationErrorMessage(err error) string {
	var valErr goValidator.ValidationErrors
	if !errors.As(err, &valErr) {
		return "Validation failed. Check that all required fields are present and valid."
	}
	var parts []string
	for _, e := range valErr {
		field := e.Field()
		tag := e.Tag()
		switch tag {
		case "required":
			parts = append(parts, "Field '"+field+"' is required")
		default:
			parts = append(parts, "Field '"+field+"': "+tag)
		}
	}
	return "Validation failed. " + strings.Join(parts, "; ") + "."
}

