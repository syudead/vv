package domain

import "time"

// WorkerWait is how long a background worker waits after one step before the next; a wakeup ends any wait early.
// Positive values wait that long.
type WorkerWait time.Duration

const (
	// WorkerNext steps again at once.
	WorkerNext WorkerWait = 0
	// WorkerIdle waits for a wakeup only, as after an empty queue or while the feature is off.
	WorkerIdle WorkerWait = -1
	// WorkerRetry waits out a failure that may pass, such as a database error.
	WorkerRetry = WorkerWait(5 * time.Second)
)
