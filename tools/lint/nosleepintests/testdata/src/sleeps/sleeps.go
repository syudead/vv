package sleeps

import "time"

// Production code may sleep; only tests are checked.
func Backoff() { time.Sleep(time.Millisecond) }
