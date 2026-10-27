package jobs

import (
	"context"
	"sync"

	"github.com/syudead/vv/internal/domain"
)

// capacity counts the slots in use against the CPU slots, the classifier slots and each disk's read slots; it is safe
// for concurrent use, because running jobs borrow and return slots while the scheduler grants.
type capacity struct {
	mu             sync.Mutex
	cpu            int
	cpuUsed        int
	classifierUsed int
	readSlots      map[string]int
	readUsed       map[string]int
	// foreground counts the open foreground readers per disk, which hold its background reads to one slot.
	foreground map[string]int
	// scanWaiting counts the scan reads waiting for each disk; while one waits, no job takes a read slot there.
	scanWaiting map[string]int
	// scanAhead counts the scans that read again on each disk, and scanHeld the read slots scans hold there; a scan
	// between two reads keeps one slot free from jobs.
	scanAhead map[string]int
	scanHeld  map[string]int
	// changed is closed and replaced whenever slots may have become free, to wake the waiting scan reads.
	changed chan struct{}
}

func newCapacity(cpu int) *capacity {
	return &capacity{
		cpu: cpu, readSlots: map[string]int{}, readUsed: map[string]int{}, foreground: map[string]int{},
		scanWaiting: map[string]int{}, scanAhead: map[string]int{}, scanHeld: map[string]int{},
		changed: make(chan struct{}),
	}
}

// holdForeground caps the disk's background reads at one slot until releaseForeground; slots already granted stay
// granted.
func (c *capacity) holdForeground(disk string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.foreground[disk]++
}

func (c *capacity) releaseForeground(disk string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.foreground[disk]--; c.foreground[disk] <= 0 {
		delete(c.foreground, disk)
	}
	c.broadcastLocked()
}

// readLimitLocked caps the disk's slots for background reads; it is never 0 while a foreground reader is open, so
// background work slows but does not stop.
func (c *capacity) readLimitLocked(disk string, slots int) int {
	if c.foreground[disk] > 0 {
		return min(slots, 1)
	}
	return slots
}

// setDisks replaces each disk's read slots; slots already held on a disk stay counted until released.
func (c *capacity) setDisks(slots map[string]int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.readSlots = slots
	c.broadcastLocked()
}

func (c *capacity) broadcastLocked() {
	close(c.changed)
	c.changed = make(chan struct{})
}

// acquireScanRead waits for one read slot on disk and takes it ahead of every job; it never takes a slot a job holds.
// A disk the scheduler has not served yet counts its slots from its type.
func (c *capacity) acquireScanRead(ctx context.Context, disk domain.Disk) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for {
		slots, ok := c.readSlots[disk.Key]
		if !ok {
			slots = disk.ReadSlots()
		}
		if c.readUsed[disk.Key] < c.readLimitLocked(disk.Key, slots) {
			c.readUsed[disk.Key]++
			c.scanHeld[disk.Key]++
			return nil
		}
		changed := c.changed
		c.scanWaiting[disk.Key]++
		c.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
		}
		c.mu.Lock()
		c.scanWaiting[disk.Key]--
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

func (c *capacity) releaseScanRead(disk string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.readUsed[disk]--
	c.scanHeld[disk]--
	c.broadcastLocked()
}

// keepScanAhead keeps one read slot of disk free from jobs while the scan is between its reads there, until the
// returned func is called.
func (c *capacity) keepScanAhead(disk string) (done func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.scanAhead[disk]++
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.scanAhead[disk]--
		c.broadcastLocked()
	}
}

// reserve takes demand on disk when it fits; a demand without reads ignores the disk. A waiting scan read keeps the
// disk's read slots from jobs, and a scan between its reads keeps one of them.
func (c *capacity) reserve(disk string, demand domain.JobResources) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cpuUsed+demand.CPU > c.cpu || c.classifierUsed+demand.Classifier > domain.JobClassifierSlots {
		return false
	}
	kept := max(c.scanAhead[disk]-c.scanHeld[disk], 0)
	limit := c.readLimitLocked(disk, c.readSlots[disk])
	if demand.Read > 0 && (c.scanWaiting[disk] > 0 || c.readUsed[disk]+demand.Read+kept > limit) {
		return false
	}
	c.cpuUsed += demand.CPU
	c.classifierUsed += demand.Classifier
	c.readUsed[disk] += demand.Read
	return true
}

func (c *capacity) release(disk string, demand domain.JobResources) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cpuUsed -= demand.CPU
	c.classifierUsed -= demand.Classifier
	c.readUsed[disk] -= demand.Read
	c.broadcastLocked()
}

// inUse reports the slots in use, with the disk's read slots, for the debug log.
func (c *capacity) inUse(disk string) domain.JobResources {
	c.mu.Lock()
	defer c.mu.Unlock()
	return domain.JobResources{Read: c.readUsed[disk], CPU: c.cpuUsed, Classifier: c.classifierUsed}
}
