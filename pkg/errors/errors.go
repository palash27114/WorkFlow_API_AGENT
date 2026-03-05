package errors

import "errors"

var (
	// Domain-level errors
	ErrNotFound                = errors.New("resource not found")
	ErrInvalidStatusTransition = errors.New("invalid status transition")
	ErrBlockedByDependencies   = errors.New("task is blocked by incomplete dependencies")
	ErrCircularDependency      = errors.New("circular dependency detected")
	ErrDeadlineConstraint      = errors.New("deadline must be after all dependency deadlines")
)

