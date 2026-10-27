package jobs

import (
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
}

func newCapacity(cpu int) *capacity {
	return &capacity{cpu: cpu, readSlots: map[string]int{}, readUsed: map[string]int{}, foreground: map[string]int{}}
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
}

// readLimitLocked is never 0 while a foreground reader is open, so background work slows but does not stop.
func (c *capacity) readLimitLocked(disk string) int {
	if c.foreground[disk] > 0 {
		return min(c.readSlots[disk], 1)
	}
	return c.readSlots[disk]
}

// setDisks replaces each disk's read slots; slots already held on a disk stay counted until released.
func (c *capacity) setDisks(slots map[string]int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.readSlots = slots
}

// reserve takes demand on disk when it fits; a demand without reads ignores the disk.
func (c *capacity) reserve(disk string, demand domain.JobResources) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cpuUsed+demand.CPU > c.cpu || c.classifierUsed+demand.Classifier > domain.JobClassifierSlots {
		return false
	}
	if demand.Read > 0 && c.readUsed[disk]+demand.Read > c.readLimitLocked(disk) {
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
}

// inUse reports the slots in use, with the disk's read slots, for the debug log.
func (c *capacity) inUse(disk string) domain.JobResources {
	c.mu.Lock()
	defer c.mu.Unlock()
	return domain.JobResources{Read: c.readUsed[disk], CPU: c.cpuUsed, Classifier: c.classifierUsed}
}
