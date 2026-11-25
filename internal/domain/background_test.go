package domain

import "testing"

// A kind without a group would be left out of every count and of the running list.
func TestEveryScheduledJobKindHasAGroup(t *testing.T) {
	for _, kind := range ScheduledJobKinds {
		if _, ok := JobGroup(kind); !ok {
			t.Errorf("kind %s has no background group", kind)
		}
	}
}
