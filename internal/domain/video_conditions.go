package domain

import (
	"errors"
	"fmt"
	"time"
)

var ErrInvalidVideoConditions = errors.New("invalid video conditions")

const calendarDayLayout = "2006-01-02"

// CalendarDay is a YYYY-MM-DD day in the server's local time zone; empty means unset.
type CalendarDay string

func (d CalendarDay) parse() (time.Time, error) {
	return time.ParseInLocation(calendarDayLayout, string(d), time.Local)
}

// Start is the first instant of the day in the server's local time zone; callers validate the day first.
func (d CalendarDay) Start() time.Time {
	day, _ := d.parse()
	return day
}

// End is the first instant of the following day, so a range ends before it.
func (d CalendarDay) End() time.Time {
	return d.Start().AddDate(0, 0, 1)
}

// VideoConditions are the duration, added-day and watch-state filters shared by VideoQuery and SmartRule; nil and empty values do not filter.
type VideoConditions struct {
	// Duration bounds are inclusive and nil when unset, so an explicit 0 still filters; an unknown duration never matches a bound.
	MinDurationMs *int64      `json:"minDurationMs,omitempty"`
	MaxDurationMs *int64      `json:"maxDurationMs,omitempty"`
	AddedFrom     CalendarDay `json:"addedFrom,omitempty"`
	AddedTo       CalendarDay `json:"addedTo,omitempty"`
	// WatchState follows ClassifyWatch.
	WatchState WatchState `json:"watchState,omitempty"`
}

// HasCondition reports whether any condition is set.
func (c VideoConditions) HasCondition() bool {
	return c.MinDurationMs != nil || c.MaxDurationMs != nil || c.AddedFrom != "" || c.AddedTo != "" || c.WatchState != ""
}

// Validate rejects negative durations, days that do not parse, reversed bounds and unknown watch states with ErrInvalidVideoConditions.
func (c VideoConditions) Validate() error {
	if (c.MinDurationMs != nil && *c.MinDurationMs < 0) || (c.MaxDurationMs != nil && *c.MaxDurationMs < 0) {
		return fmt.Errorf("%w: a duration is negative", ErrInvalidVideoConditions)
	}
	if c.MinDurationMs != nil && c.MaxDurationMs != nil && *c.MaxDurationMs < *c.MinDurationMs {
		return fmt.Errorf("%w: maxDurationMs is below minDurationMs", ErrInvalidVideoConditions)
	}
	for name, day := range map[string]CalendarDay{"addedFrom": c.AddedFrom, "addedTo": c.AddedTo} {
		if day == "" {
			continue
		}
		if _, err := day.parse(); err != nil {
			return fmt.Errorf("%w: %s is not a YYYY-MM-DD day", ErrInvalidVideoConditions, name)
		}
	}
	if c.AddedFrom != "" && c.AddedTo != "" && c.AddedTo.Start().Before(c.AddedFrom.Start()) {
		return fmt.Errorf("%w: addedTo is before addedFrom", ErrInvalidVideoConditions)
	}
	switch c.WatchState {
	case "", WatchStateUnwatched, WatchStateInProgress, WatchStateWatched:
		return nil
	default:
		return fmt.Errorf("%w: watchState %q is unknown", ErrInvalidVideoConditions, c.WatchState)
	}
}
