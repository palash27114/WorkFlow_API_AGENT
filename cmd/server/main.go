package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger"
	"go.uber.org/zap"

	"taskflow/docs"
	"taskflow/internal/config"
	"taskflow/internal/database"
	"taskflow/internal/handler"
	appMiddleware "taskflow/internal/middleware"
	"taskflow/internal/repository"
	"taskflow/internal/service"
	"taskflow/internal/validator"
)

// @title TaskFlow API
// @version 1.0
// @description TaskFlow is a workflow task management API where users create projects, define tasks with dependencies, and the system determines execution order.
// @BasePath /api/v1



func main() {

	// logger
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("cannot init logger: %v", err)
	}
	defer logger.Sync()





	// config
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}


	// database
	db, err := database.Connect(cfg, logger)
	if err != nil {
		logger.Fatal("failed to connect database", zap.Error(err))
	}

	if err := database.AutoMigrate(db, logger); err != nil {
		logger.Fatal("failed to run migrations", zap.Error(err))
	}



	// validator
	v := validator.New()


	
	// repositories
	projectRepo := repository.NewProjectRepository(db)
	taskRepo := repository.NewTaskRepository(db)
	depRepo := repository.NewTaskDependencyRepository(db)

	// services
	projectService := service.NewProjectService(projectRepo, taskRepo, depRepo)
	taskService := service.NewTaskService(taskRepo, depRepo, projectRepo)

	// handlers
	projectHandler := handler.NewProjectHandler(projectService, logger, v)
	taskHandler := handler.NewTaskHandler(taskService, logger, v)

	// router
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(appMiddleware.ZapLogger(logger))
	r.Use(appMiddleware.Recoverer(logger))
	r.Use(appMiddleware.ContentTypeJSON)
	r.Use(appMiddleware.ValidationErrorHandler)

	// swagger setup
	docs.SwaggerInfo.Title = "TaskFlow API"
	docs.SwaggerInfo.Version = "1.0"
	docs.SwaggerInfo.BasePath = "/api/v1"

	r.Get("/swagger/*", httpSwagger.WrapHandler)





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

	srv := &http.Server{
		Addr:         ":" + cfg.HTTPPort,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("starting http server", zap.String("port", cfg.HTTPPort))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server error", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server shutdown failed", zap.Error(err))
	} else {
		logger.Info("server exited gracefully")
	}
}

