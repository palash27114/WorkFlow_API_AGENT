package validator

import (
	"github.com/go-playground/validator/v10"
)

// Validator defines an interface to decouple concrete validation library from handlers/services.
type Validator interface {
	Struct(s interface{}) error
}

type validateWrapper struct {
	v *validator.Validate
}

// New creates a new validator instance.
func New() Validator {
	v := validator.New()
	return &validateWrapper{v: v}
}

func (w *validateWrapper) Struct(s interface{}) error {
	return w.v.Struct(s)
}

