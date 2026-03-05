package graph

import (
	"container/heap"
	"taskflow/internal/models"
	"taskflow/pkg/errors"
	"time"
)

// ExecutionNode represents a task in the execution plan.
type ExecutionNode struct {
	TaskID uint   `json:"task_id"`
	Title  string `json:"title"`
}

type readyTask struct {
	id       uint
	deadline *time.Time
	priority models.TaskPriority
}

type readyTaskHeap []readyTask

func (h readyTaskHeap) Len() int { return len(h) }
func (h readyTaskHeap) Less(i, j int) bool {
	di, dj := h[i].deadline, h[j].deadline

	// Earlier deadlines first; tasks with no deadline come last.
	if di == nil && dj != nil {
		return false
	}
	if di != nil && dj == nil {
		return true
	}
	if di != nil && dj != nil {
		if di.Before(*dj) {
			return true
		}
		if dj.Before(*di) {
			return false
		}
	}

	// Tie-breaker: higher priority first (high > medium > low).
	pi := priorityRank(h[i].priority)
	pj := priorityRank(h[j].priority)
	if pi != pj {
		return pi > pj
	}

	// Final tie-breaker for deterministic output.
	return h[i].id < h[j].id
}
func (h readyTaskHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *readyTaskHeap) Push(x any)   { *h = append(*h, x.(readyTask)) }
func (h *readyTaskHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

func priorityRank(p models.TaskPriority) int {
	switch p {
	case models.TaskPriorityHigh:
		return 3
	case models.TaskPriorityMedium:
		return 2
	case models.TaskPriorityLow:
		return 1
	default:
		return 0
	}
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

	// tasks with in-degree 0, ordered by earliest deadline first
	ready := make(readyTaskHeap, 0, len(tasks))
	for id, deg := range inDegree {
		if deg == 0 {
			t, ok := idToTask[id]
			if !ok {
				continue
			}
			ready = append(ready, readyTask{
				id:       id,
				deadline: t.Deadline,
				priority: t.Priority,
			})
		}
	}
	heap.Init(&ready)

	result := make([]ExecutionNode, 0, len(tasks))

	for ready.Len() > 0 {
		n := heap.Pop(&ready).(readyTask).id
		task, ok := idToTask[n]
		if !ok {
			continue
		}
		result = append(result, ExecutionNode{
			TaskID: task.ID,
			Title:  task.Title,
		})

		for _, m := range adj[n] {
			inDegree[m]--
			if inDegree[m] == 0 {
				mt, ok := idToTask[m]
				if !ok {
					continue
				}
				heap.Push(&ready, readyTask{
					id:       m,
					deadline: mt.Deadline,
					priority: mt.Priority,
				})
			}
		}
	}

	if len(result) != len(tasks) {
		return nil, errors.ErrCircularDependency
	}

	return result, nil
}

