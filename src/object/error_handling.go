// object/error_handling.go
package object

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/javanhut/Carrion/src/token"
)

const (
	CUSTOM_ERROR_OBJ = "USER DEFINED ERROR"
)

// ErrorType represents different categories of errors
type ErrorType string

const (
	// Error type constants
	RuntimeError      ErrorType = "RuntimeError"
	SyntaxError       ErrorType = "SyntaxError"
	TypeError         ErrorType = "TypeError"
	ReferenceError    ErrorType = "ReferenceError"
	ImportError       ErrorType = "ImportError"
	IndexError        ErrorType = "IndexError"
	AttributeError    ErrorType = "AttributeError"
	NameError         ErrorType = "NameError"
	ValueError        ErrorType = "ValueError"
	OverflowError     ErrorType = "OverflowError"
	AssertionError    ErrorType = "AssertionError"
	NotImplementedErr ErrorType = "NotImplementedError"
	DivisionByZeroErr ErrorType = "DivisionByZeroError"
)

// StackTraceEntry represents a single entry in the stack trace
type StackTraceEntry struct {
	Position token.Position // Position in source code
	Function string         // Function name
	Args     []string       // Function arguments (optional)
	Context  string         // Context code snippet (optional)
}

// Error represents a built-in error in the language with stack trace
type Error struct {
	ErrorKind   ErrorType
	Message     string
	StackTrace  []StackTraceEntry
	Position    token.Position
	Cause       *Error    // Optional cause of this error
	Suggestions []string  // Optional suggestions to fix the error
	Context     string    // Optional code context where error occurred
}

func (e *Error) Type() ObjectType { return ERROR_OBJ }

func (e *Error) Inspect() string {
	var sb strings.Builder

	// Create a header with error type and message
	sb.WriteString(fmt.Sprintf("\x1b[31m%s\x1b[0m: %s\n", e.ErrorKind, e.Message))

	// Add position information if available
	if e.Position.File != "" {
		// Get relative path for better readability
		relPath, _ := filepath.Rel(".", e.Position.File)
		if relPath == "" {
			relPath = e.Position.File
		}
		sb.WriteString(fmt.Sprintf("  at \x1b[36m%s\x1b[0m:\x1b[33m%d\x1b[0m:\x1b[33m%d\x1b[0m\n", 
			relPath, e.Position.Line, e.Position.Column))
	}

	// Add context if available
	if e.Context != "" {
		sb.WriteString("\nCode context:\n")
		lines := strings.Split(e.Context, "\n")
		for i, line := range lines {
			lineNum := e.Position.Line - (len(lines) - 1) + i
			if lineNum == e.Position.Line {
				// Highlight the error line
				sb.WriteString(fmt.Sprintf("  \x1b[33m%d\x1b[0m | \x1b[31m%s\x1b[0m\n", lineNum, line))
				// Add a caret pointing to the column
				sb.WriteString(fmt.Sprintf("     | %s\x1b[31m^\x1b[0m\n", strings.Repeat(" ", e.Position.Column-1)))
			} else {
				sb.WriteString(fmt.Sprintf("  \x1b[33m%d\x1b[0m | %s\n", lineNum, line))
			}
		}
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
			
			// Get relative path for better readability
			relPath, _ := filepath.Rel(".", entry.Position.File)
			if relPath == "" {
				relPath = entry.Position.File
			}
			
			// Format with arguments if available
			argStr := ""
			if len(entry.Args) > 0 {
				argStr = fmt.Sprintf("(%s)", strings.Join(entry.Args, ", "))
			}
			
			sb.WriteString(fmt.Sprintf("  at \x1b[36m%s%s\x1b[0m in \x1b[36m%s\x1b[0m:\x1b[33m%d\x1b[0m:\x1b[33m%d\x1b[0m\n", 
				funcName, argStr, relPath, entry.Position.Line, entry.Position.Column))
			
			// Add context code snippet if available
			if entry.Context != "" {
				contextLines := strings.Split(entry.Context, "\n")
				for _, line := range contextLines {
					sb.WriteString(fmt.Sprintf("    | %s\n", line))
				}
			}
		}
	}

	// Add suggestions if available
	if len(e.Suggestions) > 0 {
		sb.WriteString("\nSuggestions:\n")
		for _, suggestion := range e.Suggestions {
			sb.WriteString(fmt.Sprintf("  - %s\n", suggestion))
		}
	}

	// Add cause if available
	if e.Cause != nil {
		sb.WriteString("\nCaused by:\n")
		causeLines := strings.Split(e.Cause.Inspect(), "\n")
		for _, line := range causeLines {
			sb.WriteString(fmt.Sprintf("  %s\n", line))
		}
	}

	return sb.String()
}

// NewError creates a new Error object with specified error type
func NewError(errorType ErrorType, message string, positionOpt ...token.Position) *Error {
	pos := token.Position{}
	if len(positionOpt) > 0 {
		pos = positionOpt[0]
	}
	
	return &Error{
		ErrorKind:   errorType,
		Message:     message,
		Position:    pos,
		StackTrace:  []StackTraceEntry{},
		Suggestions: []string{},
	}
}

// AddStackEntry adds a stack trace entry
func (e *Error) AddStackEntry(position token.Position, function string) {
	e.AddDetailedStackEntry(position, function, nil, "")
}

// AddDetailedStackEntry adds a detailed stack trace entry with arguments and context
func (e *Error) AddDetailedStackEntry(position token.Position, function string, args []string, context string) {
	e.StackTrace = append(e.StackTrace, StackTraceEntry{
		Position: position,
		Function: function,
		Args:     args,
		Context:  context,
	})
}

// WithCause adds a cause to this error
func (e *Error) WithCause(cause *Error) *Error {
	e.Cause = cause
	return e
}

// WithSuggestion adds a suggestion to this error
func (e *Error) WithSuggestion(suggestion string) *Error {
	e.Suggestions = append(e.Suggestions, suggestion)
	return e
}

// WithSuggestions adds multiple suggestions to this error
func (e *Error) WithSuggestions(suggestions []string) *Error {
	e.Suggestions = append(e.Suggestions, suggestions...)
	return e
}

// WithContext adds source code context to this error
func (e *Error) WithContext(context string) *Error {
	e.Context = context
	return e
}

// CreateErrorWithContext creates an error with source code context extracted from the source
func CreateErrorWithContext(errorType ErrorType, message string, position token.Position, source string) *Error {
	err := NewError(errorType, message, position)
	
	// Extract context from source
	if source != "" && position.Line > 0 {
		lines := strings.Split(source, "\n")
		if position.Line <= len(lines) {
			// Get a few lines before and after the error
			startLine := position.Line - 2
			if startLine < 0 {
				startLine = 0
			}
			endLine := position.Line + 1
			if endLine > len(lines) {
				endLine = len(lines)
			}
			
			// Build context
			var contextLines []string
			for i := startLine; i < endLine && i < len(lines); i++ {
				if i < len(lines) {
					contextLines = append(contextLines, lines[i])
				}
			}
			
			err.Context = strings.Join(contextLines, "\n")
		}
	}
	
	return err
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
	Context    string             // Optional code context where error occurred
}

// Type returns the type of the object (implements the Object interface).
func (ce *CustomError) Type() ObjectType { return CUSTOM_ERROR_OBJ }

// Inspect returns a string representation of the error (implements the Object interface).
func (ce *CustomError) Inspect() string {
	var sb strings.Builder
	
	// Start with the error name and message
	sb.WriteString(fmt.Sprintf("\x1b[31m%s\x1b[0m: %s", ce.Name, ce.Message))
	
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
		// Get relative path for better readability
		relPath, _ := filepath.Rel(".", ce.Position.File)
		if relPath == "" {
			relPath = ce.Position.File
		}
		sb.WriteString(fmt.Sprintf("\n  at \x1b[36m%s\x1b[0m:\x1b[33m%d\x1b[0m:\x1b[33m%d\x1b[0m", 
			relPath, ce.Position.Line, ce.Position.Column))
	}
	
	// Add context if available
	if ce.Context != "" {
		sb.WriteString("\n\nCode context:\n")
		lines := strings.Split(ce.Context, "\n")
		for i, line := range lines {
			lineNum := ce.Position.Line - (len(lines) - 1) + i
			if lineNum == ce.Position.Line {
				// Highlight the error line
				sb.WriteString(fmt.Sprintf("  \x1b[33m%d\x1b[0m | \x1b[31m%s\x1b[0m\n", lineNum, line))
				// Add a caret pointing to the column
				sb.WriteString(fmt.Sprintf("     | %s\x1b[31m^\x1b[0m\n", strings.Repeat(" ", ce.Position.Column-1)))
			} else {
				sb.WriteString(fmt.Sprintf("  \x1b[33m%d\x1b[0m | %s\n", lineNum, line))
			}
		}
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
			
			// Get relative path for better readability
			relPath, _ := filepath.Rel(".", entry.Position.File)
			if relPath == "" {
				relPath = entry.Position.File
			}
			
			// Format with arguments if available
			argStr := ""
			if len(entry.Args) > 0 {
				argStr = fmt.Sprintf("(%s)", strings.Join(entry.Args, ", "))
			}
			
			sb.WriteString(fmt.Sprintf("  at \x1b[36m%s%s\x1b[0m in \x1b[36m%s\x1b[0m:\x1b[33m%d\x1b[0m:\x1b[33m%d\x1b[0m\n", 
				funcName, argStr, relPath, entry.Position.Line, entry.Position.Column))
			
			// Add context code snippet if available
			if entry.Context != "" {
				contextLines := strings.Split(entry.Context, "\n")
				for _, line := range contextLines {
					sb.WriteString(fmt.Sprintf("    | %s\n", line))
				}
			}
		}
	}
	
	return sb.String()
}

// NewCustomError creates a new CustomError object with optional position info.
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
	ce.AddDetailedStackEntry(position, function, nil, "")
}

// AddDetailedStackEntry adds a detailed stack trace entry with arguments and context
func (ce *CustomError) AddDetailedStackEntry(position token.Position, function string, args []string, context string) {
	ce.StackTrace = append(ce.StackTrace, StackTraceEntry{
		Position: position,
		Function: function,
		Args:     args,
		Context:  context,
	})
}

// WithContext adds source code context to this error
func (ce *CustomError) WithContext(context string) *CustomError {
	ce.Context = context
	return ce
}

// ParseError represents a syntax error detected during parsing
type ParseError struct {
	Message  string         // Error message
	Position token.Position // Position of the error in source
	Expected string         // What was expected (if applicable)
	Found    string         // What was found instead (if applicable)
	Context  string         // Source code context
}

// NewParseError creates a new parse error
func NewParseError(message string, position token.Position) *ParseError {
	return &ParseError{
		Message:  message,
		Position: position,
	}
}

// WithExpectation adds expectation information
func (pe *ParseError) WithExpectation(expected, found string) *ParseError {
	pe.Expected = expected
	pe.Found = found
	return pe
}

// WithContext adds source context
func (pe *ParseError) WithContext(context string) *ParseError {
	pe.Context = context
	return pe
}

// String formats the error message
func (pe *ParseError) String() string {
	var sb strings.Builder
	
	// Create the header with error location
	relPath, _ := filepath.Rel(".", pe.Position.File)
	if relPath == "" {
		relPath = pe.Position.File
	}
	
	sb.WriteString(fmt.Sprintf("\x1b[31mSyntaxError\x1b[0m: %s\n", pe.Message))
	sb.WriteString(fmt.Sprintf("  at \x1b[36m%s\x1b[0m:\x1b[33m%d\x1b[0m:\x1b[33m%d\x1b[0m\n", 
		relPath, pe.Position.Line, pe.Position.Column))
	
	// Add expectation information if available
	if pe.Expected != "" || pe.Found != "" {
		if pe.Expected != "" && pe.Found != "" {
			sb.WriteString(fmt.Sprintf("  expected %s, got %s\n", pe.Expected, pe.Found))
		} else if pe.Expected != "" {
			sb.WriteString(fmt.Sprintf("  expected %s\n", pe.Expected))
		} else {
			sb.WriteString(fmt.Sprintf("  got %s\n", pe.Found))
		}
	}
	
	// Add context if available
	if pe.Context != "" {
		sb.WriteString("\nCode context:\n")
		lines := strings.Split(pe.Context, "\n")
		for i, line := range lines {
			lineNum := pe.Position.Line - (len(lines) - 1) + i
			if lineNum == pe.Position.Line {
				// Highlight the error line
				sb.WriteString(fmt.Sprintf("  \x1b[33m%d\x1b[0m | \x1b[31m%s\x1b[0m\n", lineNum, line))
				// Add a caret pointing to the column
				sb.WriteString(fmt.Sprintf("     | %s\x1b[31m^\x1b[0m\n", strings.Repeat(" ", pe.Position.Column-1)))
			} else {
				sb.WriteString(fmt.Sprintf("  \x1b[33m%d\x1b[0m | %s\n", lineNum, line))
			}
		}
	}
	
	return sb.String()
}

// Creates a smart error message with suggestions based on error type
func CreateSmartError(errorType ErrorType, message string, position token.Position) *Error {
	err := NewError(errorType, message, position)
	
	// Add smart suggestions based on error type
	switch errorType {
	case SyntaxError:
		if strings.Contains(message, "unexpected token") {
			err.WithSuggestions([]string{
				"Check for missing or mismatched braces, parentheses, or colons",
				"Verify proper indentation at the beginning of each line",
				"Ensure statements are properly terminated"})
		} else if strings.Contains(message, "unterminated string") {
			err.WithSuggestion("Check for missing closing quote in string literal")
		}
	case TypeError:
		if strings.Contains(message, "cannot add") || strings.Contains(message, "cannot subtract") ||
		   strings.Contains(message, "cannot multiply") || strings.Contains(message, "cannot divide") {
			err.WithSuggestions([]string{
				"Verify that operands have compatible types",
				"Consider adding explicit type conversion"})
		}
	case NameError:
		err.WithSuggestions([]string{
			"Check if the variable is defined before use",
			"Verify the variable name is spelled correctly",
			"Make sure you're not using a variable outside its scope"})
	case ImportError:
		err.WithSuggestions([]string{
			"Verify the import path is correct",
			"Check that the imported file exists",
			"Ensure the imported file is accessible"})
	case AttributeError:
		err.WithSuggestion("Verify that the object has the attribute or method you're trying to access")
	case IndexError:
		err.WithSuggestions([]string{
			"Verify that the index is within the bounds of the collection",
			"Check for off-by-one errors in loop bounds"})
	}
	
	return err
}

// Get a snippet of source code around the error position
func GetContextFromSource(source string, position token.Position, contextLines int) string {
	if source == "" || position.Line <= 0 {
		return ""
	}
	
	lines := strings.Split(source, "\n")
	if position.Line > len(lines) {
		return ""
	}
	
	startLine := position.Line - contextLines
	if startLine < 0 {
		startLine = 0
	}
	
	endLine := position.Line + contextLines
	if endLine > len(lines) {
		endLine = len(lines)
	}
	
	var result []string
	for i := startLine; i < endLine; i++ {
		if i < len(lines) {
			result = append(result, lines[i])
		}
	}
	
	return strings.Join(result, "\n")
}