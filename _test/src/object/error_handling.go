// object/error_handling.go
package object

import (
	"fmt"
	"strings"

	"github.com/javanhut/Carrion/src/token"
)

const (
	CUSTOM_ERROR_OBJ = "USER DEFINED ERROR"
)

// StackTraceEntry represents a single entry in the stack trace
type StackTraceEntry struct {
	Position token.Position
	Function string
}

// Error represents a built-in error in the language with stack trace
type Error struct {
	Message    string
	StackTrace []StackTraceEntry
	Position   token.Position
}

func (e *Error) Type() ObjectType { return ERROR_OBJ }

func (e *Error) Inspect() string {
	var sb strings.Builder

	// Create a header with the error position
	if e.Position.File != "" {
		sb.WriteString(fmt.Sprintf("Error: %s at %s\n", e.Message, e.Position.String()))
	} else {
		sb.WriteString(fmt.Sprintf("Error: %s\n", e.Message))
	}

	// Add stack trace if available
	if len(e.StackTrace) > 0 {
		sb.WriteString("\nStack trace (most recent call last):\n")
		for i := len(e.StackTrace) - 1; i >= 0; i-- {
			entry := e.StackTrace[i]
			funcName := entry.Function
			if funcName == "" {
				funcName = "<module>"
			}
			sb.WriteString(fmt.Sprintf("  at %s in %s\n", funcName, entry.Position.String()))
		}
	}

	return sb.String()
}

// NewError creates a new Error object with optional position information
// If no position is provided, it uses a default empty position.
func NewError(message string, positionOpt ...token.Position) *Error {
	pos := token.Position{}
	if len(positionOpt) > 0 {
		pos = positionOpt[0]
	}
	
	return &Error{
		Message:    message,
		Position:   pos,
		StackTrace: []StackTraceEntry{},
	}
}

// AddStackEntry adds a stack trace entry
func (e *Error) AddStackEntry(position token.Position, function string) {
	e.StackTrace = append(e.StackTrace, StackTraceEntry{
		Position: position,
		Function: function,
	})
}

// CustomError represents a user-defined error in the language.
type CustomError struct {
	Name       string             // Name of the error type (e.g., "ValueError")
	Message    string             // Error message
	Details    map[string]Object  // Additional details (optional)
	ErrorType  *Grimoire          // The grimoire (class) this error belongs to
	Instance   *Instance          // Instance of the error (if applicable)
	Position   token.Position     // Position where the error occurred
	StackTrace []StackTraceEntry  // Stack trace for the error
}

// Type returns the type of the object (implements the Object interface).
func (ce *CustomError) Type() ObjectType { return CUSTOM_ERROR_OBJ }

// Inspect returns a string representation of the error (implements the Object interface).
func (ce *CustomError) Inspect() string {
	var sb strings.Builder
	
	// Start with the error name and message
	sb.WriteString(fmt.Sprintf("%s: %s", ce.Name, ce.Message))
	
	// Add details if any
	if len(ce.Details) > 0 {
		var details []string
		for key, value := range ce.Details {
			details = append(details, fmt.Sprintf("%s: %s", key, value.Inspect()))
		}
		sb.WriteString(fmt.Sprintf(" (%s)", strings.Join(details, ", ")))
	}
	
	// Add position information
	if ce.Position.File != "" {
		sb.WriteString(fmt.Sprintf(" at %s", ce.Position.String()))
	}
	
	// Add stack trace if available
	if len(ce.StackTrace) > 0 {
		sb.WriteString("\n\nStack trace (most recent call last):\n")
		for i := len(ce.StackTrace) - 1; i >= 0; i-- {
			entry := ce.StackTrace[i]
			funcName := entry.Function
			if funcName == "" {
				funcName = "<module>"
			}
			sb.WriteString(fmt.Sprintf("  at %s in %s\n", funcName, entry.Position.String()))
		}
	}
	
	return sb.String()
}

// NewCustomError creates a new CustomError object with optional position info.
// If no position is provided, it uses a default empty position.
func NewCustomError(name, message string, positionOpt ...token.Position) *CustomError {
	pos := token.Position{}
	if len(positionOpt) > 0 {
		pos = positionOpt[0]
	}
	
	return &CustomError{
		Name:       name,
		Message:    message,
		Details:    make(map[string]Object),
		Position:   pos,
		StackTrace: []StackTraceEntry{},
	}
}

// AddDetail adds a key-value pair to the error's details.
func (ce *CustomError) AddDetail(key string, value Object) {
	ce.Details[key] = value
}

// AddStackEntry adds a stack trace entry
func (ce *CustomError) AddStackEntry(position token.Position, function string) {
	ce.StackTrace = append(ce.StackTrace, StackTraceEntry{
		Position: position,
		Function: function,
	})
}
