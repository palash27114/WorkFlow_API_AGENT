package graph

import (
	"taskflow/internal/models"
	"taskflow/pkg/errors"
)

// ExecutionNode represents a task in the execution plan.
type ExecutionNode struct {
	TaskID uint   `json:"task_id"`
	Title  string `json:"title"`
}

// GetExecutionPlan computes a valid execution order for the given project's tasks
// using Kahn's algorithm for topological sorting. If a cycle is detected, it
// returns ErrCircularDependency.
func GetExecutionPlan(projectID uint, tasks []models.Task, deps []models.TaskDependency) ([]ExecutionNode, error) {
	idToTask := make(map[uint]models.Task, len(tasks))
	for _, t := range tasks {
		idToTask[t.ID] = t
	}

	inDegree := make(map[uint]int, len(tasks))
	adj := make(map[uint][]uint, len(tasks))

	for _, t := range tasks {
		inDegree[t.ID] = 0
	}

	for _, d := range deps {
		// edge: DependsOnTaskID -> TaskID
		adj[d.DependsOnTaskID] = append(adj[d.DependsOnTaskID], d.TaskID)
		inDegree[d.TaskID]++
	}

	// queue of nodes with in-degree 0
	queue := make([]uint, 0, len(tasks))
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	result := make([]ExecutionNode, 0, len(tasks))

	for len(queue) > 0 {
		// pop front
		n := queue[0]
		queue = queue[1:]

		if task, ok := idToTask[n]; ok {
			result = append(result, ExecutionNode{
				TaskID: task.ID,
				Title:  task.Title,
			})
		}

		for _, m := range adj[n] {
			inDegree[m]--
			if inDegree[m] == 0 {
				queue = append(queue, m)
			}
		}
	}

	if len(result) != len(tasks) {
		return nil, errors.ErrCircularDependency
	}

	return result, nil
}

