package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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

func TestExecutionPlanOrdersByDeadlineWhenMultipleReady(t *testing.T) {
	server := setupTestServer(t)

	// create project
	pBody := map[string]string{
		"name": "Deadline Ordering Project",
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

	createTask := func(title string, deadline string) taskResponse {
		tb, _ := json.Marshal(map[string]interface{}{
			"title":    title,
			"deadline": deadline,
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

	late := createTask("Late Task", "2026-04-01T10:00:00Z")
	early := createTask("Early Task", "2026-03-01T10:00:00Z")
	mid := createTask("Mid Task", "2026-03-15T10:00:00Z")

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
	if len(plan) != 3 {
		t.Fatalf("expected 3 tasks in plan, got %d", len(plan))
	}

	got0 := int(plan[0]["task_id"].(float64))
	got1 := int(plan[1]["task_id"].(float64))
	got2 := int(plan[2]["task_id"].(float64))

	if got0 != int(early.ID) || got1 != int(mid.ID) || got2 != int(late.ID) {
		t.Fatalf("expected deadline order [early, mid, late] = [%d, %d, %d], got [%d, %d, %d]",
			early.ID, mid.ID, late.ID,
			got0, got1, got2,
		)
	}
}

func TestExecutionPlanRespectsDependenciesOverDeadlines(t *testing.T) {
	server := setupTestServer(t)

	// create project
	pBody := map[string]string{
		"name": "Dependency Beats Deadline",
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

	createTask := func(title string, deadline string) taskResponse {
		tb, _ := json.Marshal(map[string]interface{}{
			"title":    title,
			"deadline": deadline,
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

	// Make dependent have an earlier deadline, but it must still come after its dependency.
	dep := createTask("Dependency Task", "2026-04-01T10:00:00Z")
	dependent := createTask("Dependent Task", "2026-03-01T10:00:00Z")

	// dependent depends on dep
	dbody, _ := json.Marshal(map[string]uint{
		"depends_on_task_id": dep.ID,
	})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+strconv.Itoa(int(pResp.ID))+"/tasks/"+strconv.Itoa(int(dependent.ID))+"/dependencies", bytes.NewReader(dbody))
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected dependency create 201, got %d, body=%s", rec.Code, rec.Body.String())
	}

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

	got0 := int(plan[0]["task_id"].(float64))
	got1 := int(plan[1]["task_id"].(float64))

	if got0 != int(dep.ID) || got1 != int(dependent.ID) {
		t.Fatalf("expected dependency order [dep, dependent] = [%d, %d], got [%d, %d]",
			dep.ID, dependent.ID,
			got0, got1,
		)
	}
}

func TestRisksFlagsDependencyWithLaterDeadline(t *testing.T) {
	server := setupTestServer(t)

	// create project
	pBody := map[string]string{
		"name": "Risk Later Deadline Dep",
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

	createTask := func(title string, deadline string, hours float64) taskResponse {
		tb, _ := json.Marshal(map[string]interface{}{
			"title":           title,
			"deadline":        deadline,
			"estimated_hours": hours,
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

	// A depends on B, but B has a later deadline than A.
	taskA := createTask("Task A", "2026-03-06T10:00:00Z", 2)
	taskB := createTask("Task B", "2026-03-12T10:00:00Z", 7)

	// add dependency: A depends on B
	dbody, _ := json.Marshal(map[string]uint{
		"depends_on_task_id": taskB.ID,
	})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+strconv.Itoa(int(pResp.ID))+"/tasks/"+strconv.Itoa(int(taskA.ID))+"/dependencies", bytes.NewReader(dbody))
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected dependency create 201, got %d, body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+strconv.Itoa(int(pResp.ID))+"/risks", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected risks 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var risks []map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&risks); err != nil {
		t.Fatalf("failed to decode risks: %v", err)
	}

	found := false
	for _, r := range risks {
		if int(r["task_id"].(float64)) == int(taskA.ID) {
			found = true
			reason, _ := r["reason"].(string)
			if !strings.Contains(reason, "later") && !strings.Contains(reason, "after this task's deadline") {
				t.Fatalf("expected risk reason to mention later dependency deadline, got %q", reason)
			}
		}
	}
	if !found {
		t.Fatalf("expected Task A (%d) to appear in risks, got %v", taskA.ID, risks)
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

	// first dependency: task2 depends on task1 -> allowed
	dbody, _ := json.Marshal(map[string]uint{"depends_on_task_id": task1.ID})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+strconv.Itoa(int(pResp.ID))+"/tasks/"+strconv.Itoa(int(task2.ID))+"/dependencies", bytes.NewReader(dbody))
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected first dep create 201, got %d, body=%s", rec.Code, rec.Body.String())
	}

	// second dependency: task1 depends on task2 -> creates cycle 1->2->1, must be rejected with 400
	dbody2, _ := json.Marshal(map[string]uint{"depends_on_task_id": task2.ID})
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+strconv.Itoa(int(pResp.ID))+"/tasks/"+strconv.Itoa(int(task1.ID))+"/dependencies", bytes.NewReader(dbody2))
	rec2 := httptest.NewRecorder()
	server.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("expected second dep create 400 (cycle detected), got %d, body=%s", rec2.Code, rec2.Body.String())
	}
}

