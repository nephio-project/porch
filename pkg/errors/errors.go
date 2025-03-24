package errors

import (
	"fmt"
	"strings"

	pkgerrors "github.com/pkg/errors"
)

// ErrorCollector can collect then combine multiple errors into one.
type ErrorCollector struct {
	errors []error
	// Used in Join to join the error messages.
	Sep string
	// Used in Join to format the contained message.
	// Should be some simple wrapping like `{%s}`.
	Fmt string
}

func NewErrorCollector() *ErrorCollector {
	return &ErrorCollector{
		errors: []error{},
		Sep:    "; ",
		Fmt:    "%s",
	}
}

func (e *ErrorCollector) WithSeparator(sep string) *ErrorCollector {
	e.Sep = sep
	return e
}

func (e *ErrorCollector) WithFormat(format string) *ErrorCollector {
	e.Fmt = format
	return e
}

// Add adds an error to the collector if it is not nil.
func (e *ErrorCollector) Add(err error) {
	if err != nil {
		e.errors = append(e.errors, err)
	}
}

// IsEmpty returns true if the collector has no errors.
func (e *ErrorCollector) IsEmpty() bool {
	return e == nil || len(e.errors) == 0
}

func (e *ErrorCollector) join(format string) string {
	msgs := make([]string, 0, len(e.errors))
	for _, err := range e.errors {
		msg := fmt.Sprintf(format, err)
		msgs = append(msgs, msg)
	}

	return strings.Join(msgs, e.Sep)
}

// Join combines the errors formatted Fmt and separated by Sep.
// If the collector is empty, it returns nil.
func (e *ErrorCollector) Join() error {
	if e.IsEmpty() {
		return nil
	}
	return pkgerrors.New(e.join(e.Fmt))
}
