package desktop

import (
	"fmt"
	"testing"
	"time"
)

// Model Local names per session and Global names across sessions.
type fakeKernel struct {
	objects map[string]*fakeMutex
	handles map[uintptr]*fakeMutex
	next    uintptr
}

type fakeMutex struct {
	key   string
	refs  int
	owner uintptr
}

func newFakeKernel() *fakeKernel {
	return &fakeKernel{objects: map[string]*fakeMutex{}, handles: map[uintptr]*fakeMutex{}}
}

// Call after following each simulated OS operation.
type fakeProcess struct {
	k       *fakeKernel
	session int
	after   func()
}

func (p *fakeProcess) key(name string) string {
	if len(name) > 6 && name[:6] == `Local\` {
		return fmt.Sprintf("session %d:%s", p.session, name)
	}
	return name
}

func (p *fakeProcess) step() {
	if p.after != nil {
		p.after()
	}
}

func (p *fakeProcess) CreateMutex(name string, owner bool) (uintptr, bool, error) {
	defer p.step()
	p.k.next++
	handle := p.k.next
	m, existed := p.k.objects[p.key(name)]
	if !existed {
		m = &fakeMutex{key: p.key(name)}
		p.k.objects[m.key] = m
		if owner {
			m.owner = handle
		}
	}
	m.refs++
	p.k.handles[handle] = m
	return handle, existed, nil
}

func (p *fakeProcess) MutexExists(name string) bool {
	defer p.step()
	_, ok := p.k.objects[p.key(name)]
	return ok
}

func (p *fakeProcess) Close(handle uintptr) {
	defer p.step()
	m := p.k.handles[handle]
	delete(p.k.handles, handle)
	if m.owner == handle {
		m.owner = 0
	}
	if m.refs--; m.refs == 0 {
		delete(p.k.objects, m.key)
	}
}

func (p *fakeProcess) Wait(handle uintptr, _ time.Duration) (bool, error) {
	defer p.step()
	m := p.k.handles[handle]
	if m.owner == 0 {
		m.owner = handle
		return true, nil
	}
	return false, nil
}

func (p *fakeProcess) ActivateWindow() bool {
	defer p.step()
	return false
}

// Exercise interleavings throughout acquisition so only one process starts and session identity is
// never misclassified.
func TestAcquireInstanceInterleaving(t *testing.T) {
	const globalName, localName = `Global\VVMDM-x`, `Local\VVMDM-x`
	for _, sameSession := range []bool{true, false} {
		for at := 1; at <= 10; at++ {
			t.Run(fmt.Sprintf("same session %v, after call %d", sameSession, at), func(t *testing.T) {
				k := newFakeKernel()
				second := &fakeProcess{k: k, session: 1}
				if !sameSession {
					second.session = 2
				}
				var secondState InstanceState
				var secondErr error
				ran := false
				calls := 0
				first := &fakeProcess{k: k, session: 1}
				first.after = func() {
					if calls++; calls == at && !ran {
						ran = true
						secondState, _, secondErr = acquireInstance(second, globalName, localName, 0)
					}
				}
				firstState, _, firstErr := acquireInstance(first, globalName, localName, 0)
				if !ran {
					secondState, _, secondErr = acquireInstance(second, globalName, localName, 0)
				}
				if firstErr != nil || secondErr != nil {
					t.Fatalf("errors: %v, %v", firstErr, secondErr)
				}
				states := []InstanceState{firstState, secondState}
				acquired := 0
				for _, s := range states {
					if s == InstanceAcquired {
						acquired++
					}
					if sameSession && s == InstanceOtherSession {
						t.Errorf("states %v: a launch in the same session was reported as another session", states)
					}
					if !sameSession && s != InstanceAcquired && s != InstanceOtherSession {
						t.Errorf("states %v: a launch in another session was not reported as such", states)
					}
				}
				if acquired != 1 {
					t.Errorf("states %v: %d launches acquired the instance", states, acquired)
				}
			})
		}
	}
}
