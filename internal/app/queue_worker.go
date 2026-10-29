package app

// Waker asks the job scheduler for another claim round; cmd/mdm passes the scheduler's Wake. A nil Waker drops
// wakeups.
type Waker func()

func (w Waker) Wake() {
	if w != nil {
		w()
	}
}
