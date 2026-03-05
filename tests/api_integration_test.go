package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"taskflow/internal/database"
	"taskflow/internal/handler"
	"taskflow/internal/middleware"
	"taskflow/internal/repository"
	"taskflow/internal/service"
	"taskflow/internal/validator"
)

func setupTestServer(t *testing.T) http.Handler {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite: %v", err)
	}

	logger := zap.NewNop()

	if err := database.AutoMigrate(db, logger); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	projectRepo := repository.NewProjectRepository(db)
	taskRepo := repository.NewTaskRepository(db)
	depRepo := repository.NewTaskDependencyRepository(db)

	projectService := service.NewProjectService(projectRepo, taskRepo, depRepo)
	taskService := service.NewTaskService(taskRepo, depRepo, projectRepo)

	v := validator.New()

	projectHandler := handler.NewProjectHandler(projectService, logger, v)
	taskHandler := handler.NewTaskHandler(taskService, logger, v)

	r := chi.NewRouter()
	r.Use(middleware.ContentTypeJSON)

	r.Route("/api/v1", func(api chi.Router) {
		api.Mount("/projects", projectHandler.Routes())

		api.Route("/projects/{project_id}/tasks", func(rt chi.Router) {
			rt.Post("/", taskHandler.CreateTaskForProject)
			rt.Get("/", taskHandler.ListTasksForProject)
			rt.Get("/{id}", taskHandler.GetTask)
			rt.Put("/{id}", taskHandler.UpdateTask)
			rt.Delete("/{id}", taskHandler.DeleteTask)
			rt.Post("/{id}/dependencies", taskHandler.AddDependencyForProject)
		})
	})

	return r
}

type projectResponse struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type taskResponse struct {
	ID        uint   `json:"id"`
	ProjectID uint   `json:"project_id"`
	Title     string `json:"title"`
}

func TestProjectCreation(t *testing.T) {
	server := setupTestServer(t)

	body := map[string]string{
		"name":        "Test Project",
		"description": "Integration test project",
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(b))
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var resp projectResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.ID == 0 {
		t.Fatalf("expected non-zero project id")
	}
}

func TestTaskCreationAndDependencyAndExecutionPlan(t *testing.T) {
	server := setupTestServer(t)

	// create project
	pBody := map[string]string{
		"name": "Project A",
	}
	pb, _ := json.Marshal(pBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(pb))
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected project create 201, got %d", rec.Code)
	}
	var pResp projectResponse
	_ = json.NewDecoder(rec.Body).Decode(&pResp)

	// create tasks
	createTask := func(title string) taskResponse {
		tb, _ := json.Marshal(map[string]interface{}{
			"title": title,
		})
		url := "/api/v1/projects/" + strconv.Itoa(int(pResp.ID)) + "/tasks"
		req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(tb))
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected task create 201, got %d, body=%s", rec.Code, rec.Body.String())
		}
		var tr taskResponse
		_ = json.NewDecoder(rec.Body).Decode(&tr)
		return tr
	}

	task1 := createTask("Setup DB")
	task2 := createTask("Create API")

	// add dependency: task2 depends on task1, scoped by project
	dbody, _ := json.Marshal(map[string]uint{
		"depends_on_task_id": task1.ID,
	})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+strconv.Itoa(int(pResp.ID))+"/tasks/"+strconv.Itoa(int(task2.ID))+"/dependencies", bytes.NewReader(dbody))
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected dependency create 201, got %d, body=%s", rec.Code, rec.Body.String())
	}

	// execution plan should list task1 before task2
	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+strconv.Itoa(int(pResp.ID))+"/execution-plan", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected execution plan 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var plan []map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&plan); err != nil {
		t.Fatalf("failed to decode execution plan: %v", err)
	}

	if len(plan) != 2 {
		t.Fatalf("expected 2 tasks in plan, got %d", len(plan))
	}

	if int(plan[0]["task_id"].(float64)) != int(task1.ID) {
		t.Fatalf("expected first task to be %d, got %v", task1.ID, plan[0]["task_id"])
	}
}

func TestCycleDetection(t *testing.T) {
	server := setupTestServer(t)

	// create project
	pBody := map[string]string{"name": "Project B"}
	pb, _ := json.Marshal(pBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(pb))
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected project create 201, got %d", rec.Code)
	}
	var pResp projectResponse
	_ = json.NewDecoder(rec.Body).Decode(&pResp)

	// create two tasks
	createTask := func(title string) taskResponse {
		tb, _ := json.Marshal(map[string]interface{}{"title": title})
		url := "/api/v1/projects/" + strconv.Itoa(int(pResp.ID)) + "/tasks"
		req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(tb))
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected task create 201, got %d, body=%s", rec.Code, rec.Body.String())
		}
		var tr taskResponse
		_ = json.NewDecoder(rec.Body).Decode(&tr)
		return tr
	}

	task1 := createTask("Task 1")
	task2 := createTask("Task 2")

	// create cycle: 1->2 and 2->1 within the same project
	createDep := func(taskID, dependsOn uint) {
		dbody, _ := json.Marshal(map[string]uint{"depends_on_task_id": dependsOn})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+strconv.Itoa(int(pResp.ID))+"/tasks/"+strconv.Itoa(int(taskID))+"/dependencies", bytes.NewReader(dbody))
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected dep create 201, got %d, body=%s", rec.Code, rec.Body.String())
		}
	}

	createDep(task2.ID, task1.ID)
	createDep(task1.ID, task2.ID)

	// execution plan should now fail with 400
	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+strconv.Itoa(int(pResp.ID))+"/execution-plan", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected execution plan 400 due to cycle, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

