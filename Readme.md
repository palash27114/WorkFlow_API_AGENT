## TaskFlow

TaskFlow is a workflow task management REST API written in Go. Users create projects, define tasks with dependencies, and TaskFlow computes a valid execution order using topological sorting, along with project statistics and risk analysis.

### Tech stack

- **Language**: Go 1.22+
- **Router**: `chi`
- **Database**: pluggable via GORM (PostgreSQL or SQLite)
- **ORM**: `gorm`
- **Docs**: Swagger via `swaggo`
- **Validation**: `go-playground/validator`
- **Logging**: `zap`
- **Testing**: `testing` + `httptest`

### Project layout

- `cmd/server/main.go` – HTTP server entrypoint
- `internal/config` – configuration loading
- `internal/database` – DB connection and migrations
- `internal/models` – GORM models (Project, Task, TaskDependency)
- `internal/repository` – repository interfaces & GORM implementations
- `internal/service` – business logic (state machine, toposort, stats, risks)
- `internal/handler` – HTTP handlers for projects, tasks, dependencies
- `internal/middleware` – logging, recovery, content-type, validation hook
- `internal/graph` – topological sort engine
- `internal/validator` – validation abstraction
- `pkg/response` – JSON and error helpers
- `pkg/errors` – domain error definitions
- `docs` – Swagger stub (for `swag` output)
- `tests` – integration tests with `httptest`

### Database setup

TaskFlow uses GORM and can work with PostgreSQL or SQLite.

#### PostgreSQL (recommended)

Create a database:

```sql
CREATE DATABASE taskflow;
```

Then configure connection via environment variables:

```bash
set TASKFLOW_DB_DRIVER=postgres
set TASKFLOW_DB_DSN=host=localhost user=postgres password=postgres dbname=taskflow port=5432 sslmode=disable TimeZone=UTC
set TASKFLOW_HTTP_PORT=8080
```

On Unix shells:

```bash
export TASKFLOW_DB_DRIVER=postgres
export TASKFLOW_DB_DSN="host=localhost user=postgres password=postgres dbname=taskflow port=5432 sslmode=disable TimeZone=UTC"
export TASKFLOW_HTTP_PORT=8080
```

#### SQLite (for local testing)

```bash
set TASKFLOW_DB_DRIVER=sqlite
set TASKFLOW_DB_DSN=taskflow.db
set TASKFLOW_HTTP_PORT=8080
```

On Unix:

```bash
export TASKFLOW_DB_DRIVER=sqlite
export TASKFLOW_DB_DSN=taskflow.db
export TASKFLOW_HTTP_PORT=8080
```

On startup the server runs GORM `AutoMigrate` for all models.

### Running the server

From the project root:

```bash
go run ./cmd/server
```

The API will listen on `http://localhost:8080` (or the value of `TASKFLOW_HTTP_PORT`).

### Swagger documentation

Swagger UI is served at:

- `http://localhost:8080/swagger/index.html`

This project includes basic annotations and a lightweight `docs` package so it compiles out of the box. For full swagger generation, install and run:

```bash
go install github.com/swaggo/swag/cmd/swag@latest
swag init -g cmd/server/main.go -o docs
```

Then restart the server and open `/swagger/index.html`.

### Core API endpoints

All endpoints are rooted at `/api/v1`.

- **Projects**
  - **POST** `/api/v1/projects` – create project
  - **GET** `/api/v1/projects` – list projects
  - **GET** `/api/v1/projects/{id}` – get project by id
  - **PUT** `/api/v1/projects/{id}` – update project
  - **DELETE** `/api/v1/projects/{id}` – delete project
  - **GET** `/api/v1/projects/{id}/execution-plan` – topological execution order for tasks
  - **GET** `/api/v1/projects/{id}/stats` – workload statistics (total hours, critical path, etc.)
  - **GET** `/api/v1/projects/{id}/risks` – risk analysis for tasks

- **Tasks**
  - **POST** `/api/v1/projects/{id}/tasks` – create task in project
  - **GET** `/api/v1/projects/{id}/tasks` – list project tasks
  - **GET** `/api/v1/tasks/{id}` – get task by id
  - **PUT** `/api/v1/tasks/{id}` – update task (enforces state machine and dependency rules)
  - **DELETE** `/api/v1/tasks/{id}` – delete task

- **Dependencies**
  - **POST** `/api/v1/projects/{project_id}/tasks/{id}/dependencies`

    Request body:

    ```json
    {
      "depends_on_task_id": 2
    }
    ```

### Example curl requests

Create a project:

```bash
curl -X POST http://localhost:8080/api/v1/projects \
  -H "Content-Type: application/json" \
  -d '{
    "name": "API Project",
    "description": "TaskFlow demo project"
  }'
```

Create a task in a project (project id 1):

```bash
curl -X POST http://localhost:8080/api/v1/projects/1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Setup DB",
    "description": "Create schema and migrations",
    "priority": "high",
    "estimated_hours": 8
  }'
```

Link a dependency (task 2 depends on task 1 in project 1):

```bash
curl -X POST http://localhost:8080/api/v1/projects/1/tasks/2/dependencies \
  -H "Content-Type: application/json" \
  -d '{ "depends_on_task_id": 1 }'
```

Get execution plan for a project:

```bash
curl http://localhost:8080/api/v1/projects/1/execution-plan
```

Get project statistics:

```bash
curl http://localhost:8080/api/v1/projects/1/stats
```

Get project risks:

```bash
curl http://localhost:8080/api/v1/projects/1/risks
```

### Testing

Run tests from the project root:

```bash
go test ./...
```

Integration tests in `tests` spin up an in-memory SQLite database and exercise:

- project creation
- task creation
- dependency linking
- execution plan
- cycle detection

