package desktop

import (
	"fmt"
	"time"
)

// Keep acquisition ordering independent of instanceOS so process interleavings are testable on
// every OS.

type InstanceState int

const (
	InstanceAcquired InstanceState = iota
	// The existing same-session window was activated; exit silently.
	InstanceActivated
	InstanceOtherSession
	// The same-session process neither showed its window nor exited before the deadline.
	InstanceStillRunning
)

const instancePoll = 200 * time.Millisecond

type instanceOS interface {
	// CreateMutex does not grant ownership when the named mutex already exists.
	CreateMutex(name string, owner bool) (handle uintptr, existed bool, err error)
	MutexExists(name string) bool
	Close(handle uintptr)
	// Wait also succeeds when the previous owner exited without releasing the mutex.
	Wait(handle uintptr, d time.Duration) (acquired bool, err error)
	// ActivateWindow ignores hidden windows.
	ActivateWindow() bool
}

// Create local before global so any global owner already has a session marker; after closing our
// local handle, its continued existence distinguishes same-session owners without an interleaving
// race.
func acquireInstance(sys instanceOS, globalName, localName string, wait time.Duration) (InstanceState, []uintptr, error) {
	local, _, err := sys.CreateMutex(localName, false)
	if err != nil {
		return 0, nil, err
	}
	global, existed, err := sys.CreateMutex(globalName, true)
	if err != nil {
		sys.Close(local)
		return 0, nil, err
	}
	if !existed {
		return InstanceAcquired, []uintptr{local, global}, nil
	}

	sys.Close(local)
	if !sys.MutexExists(localName) {
		sys.Close(global)
		return InstanceOtherSession, nil, nil
	}

	// Keep local while waiting so acquiring global always implies owning local, and later same-session
	// contenders recognize this session.
	local, _, err = sys.CreateMutex(localName, false)
	if err != nil {
		sys.Close(global)
		return 0, nil, err
	}
	release := func() {
		sys.Close(global)
		sys.Close(local)
	}
	deadline := time.Now().Add(wait)
	for {
		if sys.ActivateWindow() {
			release()
			return InstanceActivated, nil, nil
		}
		acquired, err := sys.Wait(global, instancePoll)
		if err != nil {
			release()
			return 0, nil, fmt.Errorf("cannot wait for the running VVMDM: %w", err)
		}
		if acquired {
			return InstanceAcquired, []uintptr{local, global}, nil
		}
		if time.Now().After(deadline) {
			release()
			return InstanceStillRunning, nil, nil
		}
	}
}
