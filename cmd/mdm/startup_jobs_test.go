package main

import (
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// startupJobLog keeps the log lines in order and reports when every startup job has been released.
type startupJobLog struct {
	mu       sync.Mutex
	lines    []string
	released map[domain.JobKind]bool
	finished bool
	done     chan struct{}
}

func (l *startupJobLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	line := string(p)
	l.lines = append(l.lines, line)
	if strings.Contains(line, `"msg":"released a job"`) {
		for _, kind := range domain.StartupJobKinds {
			if strings.Contains(line, `"stage":"`+string(kind)+`"`) {
				l.released[kind] = true
			}
		}
		if len(l.released) == len(domain.StartupJobKinds) && !l.finished {
			l.finished = true
			close(l.done)
		}
	}
	return len(p), nil
}

// firstGrantAfterListening reports whether the listener opened before the first backfill job was granted.
func (l *startupJobLog) firstGrantAfterListening() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	listening := false
	for _, line := range l.lines {
		if strings.Contains(line, `"msg":"listening"`) {
			listening = true
		}
		if strings.Contains(line, `"msg":"granted a job"`) && !listening {
			for _, kind := range domain.BackfillJobKinds {
				if strings.Contains(line, `"stage":"`+string(kind)+`"`) {
					return false
				}
			}
		}
	}
	return listening
}

func TestRunQueuesBackfillJobsOnlyAfterListening(t *testing.T) {
	stubMediaCommands(t)
	logs := &startupJobLog{released: map[domain.JobKind]bool{}, done: make(chan struct{})}
	stop := make(chan struct{})
	server := startServer(func() error {
		return run(runOptions{
			Config: Config{
				Addr: net.JoinHostPort("127.0.0.1", freePort(t)), DataDir: t.TempDir(), LogLevel: "debug",
			},
			LogOutput:  logs,
			Listener:   newReopenableListener(),
			NotifyStop: func() (<-chan struct{}, func()) { return stop, nil },
		})
	})

	select {
	case <-logs.done:
	case <-server.Done():
		t.Fatalf("run returned before the startup jobs ran: %v", server.Err())
	case <-time.After(10 * time.Second):
		t.Fatal("the startup jobs did not run")
	}
	close(stop)
	if err := server.Err(); err != nil {
		t.Fatalf("run = %v", err)
	}
	if !logs.firstGrantAfterListening() {
		t.Fatal("a backfill job was granted before the listener opened")
	}
}
