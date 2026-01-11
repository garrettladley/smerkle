package xerrors

import (
	"fmt"
	"strings"
)

type HashError struct {
	Path string
	Err  error
}

func (e HashError) Error() string {
	return fmt.Sprintf("%s: %v", e.Path, e.Err)
}

func (e HashError) Unwrap() error {
	return e.Err
}

type MultiError struct {
	Errors []HashError
}

func (e *MultiError) Error() string {
	if len(e.Errors) == 1 {
		return e.Errors[0].Error()
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d errors occurred:\n", len(e.Errors))
	for _, err := range e.Errors {
		fmt.Fprintf(&b, "  - %s\n", err.Error())
	}
	return b.String()
}

type ErrorCollector struct {
	errors []HashError
	mu     chan struct{}
}

func NewErrorCollector() *ErrorCollector {
	ec := &ErrorCollector{
		mu: make(chan struct{}, 1),
	}
	ec.mu <- struct{}{}
	return ec
}

func (c *ErrorCollector) Add(path string, err error) {
	<-c.mu
	c.errors = append(c.errors, HashError{Path: path, Err: err})
	c.mu <- struct{}{}
}

func (c *ErrorCollector) Errors() []HashError {
	<-c.mu
	result := make([]HashError, len(c.errors))
	copy(result, c.errors)
	c.mu <- struct{}{}
	return result
}

func (c *ErrorCollector) HasErrors() bool {
	<-c.mu
	has := len(c.errors) > 0
	c.mu <- struct{}{}
	return has
}
