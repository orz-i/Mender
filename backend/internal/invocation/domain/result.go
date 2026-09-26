package domain

import "strings"

// ResultKind identifies a canonical successful invocation result variant.
type ResultKind string

const (
	ResultKindComplete      ResultKind = "complete"
	ResultKindInputRequired ResultKind = "input_required"
	ResultKindTask          ResultKind = "task"
)

// Result is one successful canonical execution outcome.
type Result struct {
	kind        ResultKind
	content     JSONValue
	requestID   string
	message     string
	inputSchema []byte
	taskID      string
	taskStatus  string
}

// Complete constructs an immediately completed result.
func Complete(content JSONValue) (Result, error) {
	if content.IsZero() {
		return Result{}, validationError("content", "must be provided")
	}
	return Result{kind: ResultKindComplete, content: content}, nil
}

// InputRequired constructs an outcome requesting additional caller/model input.
func InputRequired(requestID, message string, schema []byte) (Result, error) {
	requestID = strings.TrimSpace(requestID)
	message = strings.TrimSpace(message)
	if requestID == "" {
		return Result{}, validationError("request_id", "must not be empty")
	}
	if message == "" {
		return Result{}, validationError("message", "must not be empty")
	}
	if len(schema) > 0 {
		if _, err := validateSchema(schema); err != nil {
			return Result{}, err
		}
	}

	return Result{
		kind:        ResultKindInputRequired,
		requestID:   requestID,
		message:     message,
		inputSchema: cloneBytes(schema),
	}, nil
}

// Task constructs an asynchronous task handle result.
func Task(taskID, status, message string) (Result, error) {
	taskID = strings.TrimSpace(taskID)
	status = strings.TrimSpace(status)
	if taskID == "" {
		return Result{}, validationError("task_id", "must not be empty")
	}
	if status == "" {
		return Result{}, validationError("task_status", "must not be empty")
	}

	return Result{
		kind:       ResultKindTask,
		taskID:     taskID,
		taskStatus: status,
		message:    strings.TrimSpace(message),
	}, nil
}

func (r Result) Kind() ResultKind {
	return r.kind
}

func (r Result) Content() JSONValue {
	return r.content
}

func (r Result) RequestID() string {
	return r.requestID
}

func (r Result) Message() string {
	return r.message
}

func (r Result) InputSchema() []byte {
	return cloneBytes(r.inputSchema)
}

func (r Result) TaskID() string {
	return r.taskID
}

func (r Result) TaskStatus() string {
	return r.taskStatus
}
