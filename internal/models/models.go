package models

import (
	"time"

	"gorm.io/gorm"
)

type Project struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	Name        string         `gorm:"size:255;not null" json:"name"`
	Description string         `gorm:"type:text" json:"description"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	Tasks []Task `json:"tasks"`
}

type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusBlocked    TaskStatus = "blocked"
)

type TaskPriority string

const (
	TaskPriorityLow    TaskPriority = "low"
	TaskPriorityMedium TaskPriority = "medium"
	TaskPriorityHigh   TaskPriority = "high"
)

type Task struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	ProjectID      uint           `gorm:"index;not null" json:"project_id"`
	Title          string         `gorm:"size:255;not null" json:"title"`
	Description    string         `gorm:"type:text" json:"description"`
	Status         TaskStatus     `gorm:"size:32;not null;default:'pending'" json:"status"`
	Priority       TaskPriority   `gorm:"size:32;not null;default:'medium'" json:"priority"`
	EstimatedHours float64        `json:"estimated_hours"`
	Deadline       *time.Time     `json:"deadline,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`

	Dependencies      []TaskDependency `gorm:"foreignKey:TaskID" json:"dependencies,omitempty"`
	DependentOnTasks  []TaskDependency `gorm:"foreignKey:DependsOnTaskID" json:"dependent_on_tasks,omitempty"`
	Project           Project          `gorm:"foreignKey:ProjectID" json:"-"`
}

type TaskDependency struct {
	ID              uint `gorm:"primaryKey" json:"id"`
	TaskID          uint `gorm:"index;not null" json:"task_id"`
	DependsOnTaskID uint `gorm:"index;not null" json:"depends_on_task_id"`
}

