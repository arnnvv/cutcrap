package utils

import "fmt"

type ProcessingError struct {
	Step    string
	Message string
	Err     error
}

func (e *ProcessingError) Error() string {
	return fmt.Sprintf("%s error: %s (%v)", e.Step, e.Message, e.Err)
}

func WrapError(step, message string, err error) error {
	return &ProcessingError{
		Step:    step,
		Message: message,
		Err:     err,
	}
}
