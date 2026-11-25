package domain

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Bounds of the container logs and actions (SPEC_ADMINISTRATION.md, D-5).
const (
	// DefaultContainerLogLines is the number of last lines read when the
	// request does not say.
	DefaultContainerLogLines = 200
	// MaxContainerLogLines bounds the lines read in one request.
	MaxContainerLogLines = 2000
	// MaxContainerLogLineBytes cuts a longer line; the line is marked.
	MaxContainerLogLineBytes = 16 << 10
	// ContainerActionTimeout bounds an action, from the request to the
	// answer of Docker.
	ContainerActionTimeout = 90 * time.Second
	// ContainerActionsPerMinute bounds the actions requested by one user.
	ContainerActionsPerMinute = 30
)

var (
	ErrInvalidContainerVerb = errors.New("container action must be start, stop or restart")
	// ErrContainerActionsDisabled: docker_actions_enabled is false.
	ErrContainerActionsDisabled = errors.New("container actions are disabled in the service configuration")
	// ErrContainerRemoved: the container is no longer on the Docker host.
	ErrContainerRemoved = errors.New("container was removed")
	// ErrContainerActionsLimited: too many actions in the last minute.
	ErrContainerActionsLimited = errors.New("too many container actions in the last minute")
	// ErrContainerActionsUnavailable: the action worker is not running.
	ErrContainerActionsUnavailable = errors.New("container actions are unavailable")
	// ErrInvalidLogQuery: a number of lines or a start out of bounds.
	ErrInvalidLogQuery = errors.New("container log query is invalid")
	// ErrDockerUnavailable: the restricted proxy or Docker cannot be reached.
	ErrDockerUnavailable = errors.New("Docker is unavailable")
)

// ContainerVerb is one of the three changes the Manager may ask Docker for.
// Removing a container, exec and prune stay excluded (rules 1 and 3).
type ContainerVerb string

const (
	ContainerStart   ContainerVerb = "start"
	ContainerStop    ContainerVerb = "stop"
	ContainerRestart ContainerVerb = "restart"
)

// ContainerVerbs lists the verbs in the order shown by the interface.
var ContainerVerbs = []ContainerVerb{ContainerStart, ContainerStop, ContainerRestart}

func (v ContainerVerb) Valid() bool {
	switch v {
	case ContainerStart, ContainerStop, ContainerRestart:
		return true
	}
	return false
}

// ContainerActionFailure tells why a container action failed.
type ContainerActionFailure string

const (
	// ContainerActionNotFound: Docker knows no such container.
	ContainerActionNotFound ContainerActionFailure = "not_found"
	// ContainerActionRejected: Docker refused or failed the action.
	ContainerActionRejected ContainerActionFailure = "rejected"
	// ContainerActionNotAllowed: the proxy refuses actions (its own
	// setting is off).
	ContainerActionNotAllowed ContainerActionFailure = "not_allowed"
	// ContainerActionTimedOut: no answer within ContainerActionTimeout.
	ContainerActionTimedOut ContainerActionFailure = "timeout"
	// ContainerActionUnavailable: the proxy or Docker cannot be reached.
	ContainerActionUnavailable ContainerActionFailure = "unavailable"
	// ContainerActionInterrupted: the Manager stopped during the action.
	ContainerActionInterrupted ContainerActionFailure = "interrupted"
)

// ContainerActionError is a failure reported by the container controller.
type ContainerActionError struct {
	Failure ContainerActionFailure
	Err     error
}

func (e *ContainerActionError) Error() string {
	return fmt.Sprintf("container action %s: %v", e.Failure, e.Err)
}

func (e *ContainerActionError) Unwrap() error { return e.Err }

// ContainerAction is the last action requested on a container since the
// Manager started.
type ContainerAction struct {
	ContainerID uuid.UUID
	Name        string
	Verb        ContainerVerb
	State       UnitActionState
	Failure     ContainerActionFailure
	RequestedBy string
	RequestedAt time.Time
	FinishedAt  *time.Time
}

// ContainerLogStream is the stream a log line was written to.
type ContainerLogStream string

const (
	ContainerStdout ContainerLogStream = "stdout"
	ContainerStderr ContainerLogStream = "stderr"
)

// ContainerLogLine is one line of a container log. Time is nil when Docker
// gave no readable time; Cut reports a line longer than
// MaxContainerLogLineBytes.
type ContainerLogLine struct {
	Stream  ContainerLogStream
	Time    *time.Time
	Message string
	Cut     bool
}

// ContainerLogs is the end of a container log. Truncated reports an answer
// of the proxy cut at its size bound: the first lines may be missing.
type ContainerLogs struct {
	Lines     []ContainerLogLine
	Truncated bool
}

// ContainerLogQuery selects the last Tail lines, written at Since or later
// when Since is not nil.
type ContainerLogQuery struct {
	Tail  int
	Since *time.Time
}

// Validate checks the bounds of the query.
func (q ContainerLogQuery) Validate() error {
	if q.Tail < 1 || q.Tail > MaxContainerLogLines {
		return ErrInvalidLogQuery
	}
	if q.Since != nil && (q.Since.Unix() < 1 || q.Since.After(time.Now().Add(time.Minute))) {
		return ErrInvalidLogQuery
	}
	return nil
}
