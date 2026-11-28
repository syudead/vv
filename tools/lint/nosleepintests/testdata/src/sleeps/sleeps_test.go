package sleeps

import (
	"testing"
	"testing/synctest"
	"time"
	clock "time"
)

func TestWallClock(t *testing.T) {
	time.Sleep(time.Millisecond) // want "time.Sleep in a test waits on the wall clock"
	defer clock.Sleep(1)         // want "time.Sleep in a test waits on the wall clock"
	wait := time.Sleep           // want "time.Sleep in a test waits on the wall clock"
	wait(1)
	t.Run("subtest", func(t *testing.T) {
		time.Sleep(1) // want "time.Sleep in a test waits on the wall clock"
	})
}

// A helper is checked where it is written, even when only a bubble calls it.
func sleepFor(d time.Duration) {
	time.Sleep(d) // want "time.Sleep in a test waits on the wall clock"
}

func TestBubble(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		time.Sleep(time.Hour)
		go func() { time.Sleep(time.Minute) }()
		t.Run("nested", func(t *testing.T) { time.Sleep(1) })
		inject := time.Sleep
		inject(1)
	})
	sleepFor(0)
}

// A literal that is not the bubble's argument does not count.
func TestCalledByTheBubble(t *testing.T) {
	body := func(t *testing.T) {
		time.Sleep(1) // want "time.Sleep in a test waits on the wall clock"
	}
	synctest.Test(t, body)
}
