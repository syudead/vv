package domain

import (
	"errors"
	"time"
	"unicode/utf8"
)

const WorkInfoPluginSettingValueMaxRunes = 2000

var (
	// Digest mismatches detect replacement after the owner's list read.
	ErrWorkInfoPluginChanged        = errors.New("the work-info plugin was replaced after it was listed")
	ErrWorkInfoPluginUnusable       = errors.New("the work-info plugin has a problem and cannot be used")
	ErrWorkInfoPluginSettingInvalid = errors.New("the work-info plugin setting is not declared or its value is too long")
)

// Invalid uploads do not persist Module and treat Operations/Settings as unreadable.
type WorkInfoPluginUpload struct {
	FileName string
	Module   []byte
	// Digest is lowercase SHA-256 hex of Module.
	Digest   string
	Manifest WorkInfoPluginManifest
	Problem  WorkInfoPluginProblem
}

// WorkInfoPlugin.Values may contain secrets; use Entry before crossing the internal/app boundary.
type WorkInfoPlugin struct {
	ID       int64
	FileName string
	// Invalid rows have empty Operations and Settings; empty Name means unreadable.
	Manifest       WorkInfoPluginManifest
	Digest         string
	Problem        WorkInfoPluginProblem
	ApprovedDigest string
	AddedAt        time.Time
	UpdatedAt      time.Time
	// Generation increments on replacement/revocation so apply can reject stale fetch results.
	Generation int64
	Values     map[string]string
	// LastRunAt is the last retained run's completion time, or zero.
	LastRunAt time.Time
}

func (p WorkInfoPlugin) State() WorkInfoPluginState {
	return WorkInfoPluginStateOf(p.Problem, p.Digest, p.ApprovedDigest)
}

// Secret settings expose only whether a value is set.
type WorkInfoPluginSettingEntry struct {
	WorkInfoPluginSetting
	Set bool
	// Value is always empty for secret settings.
	Value string
	// StaleValue exposes an out-of-options saved value; execution uses the default instead.
	StaleValue string
}

type WorkInfoPluginEntry struct {
	ID          int64
	Name        string
	FileName    string
	Version     string
	Description string
	Operations  []WorkInfoOperation
	Digest      string
	State       WorkInfoPluginState
	Problem     WorkInfoPluginProblem
	Settings    []WorkInfoPluginSettingEntry
	UpdatedAt   time.Time
	// LastRunAt is the last retained run's completion time, or zero.
	LastRunAt time.Time
}

// Entry preserves declaration order and removes secret values.
func (p WorkInfoPlugin) Entry() WorkInfoPluginEntry {
	settings := make([]WorkInfoPluginSettingEntry, 0, len(p.Manifest.Settings))
	for _, s := range p.Manifest.Settings {
		value, set := p.Values[s.Key]
		entry := WorkInfoPluginSettingEntry{WorkInfoPluginSetting: s, Set: set}
		if !s.Secret {
			entry.Value = value
		}
		if _, stale := EffectiveWorkInfoPluginValue(s, p.Values); stale {
			entry.StaleValue = value
		}
		settings = append(settings, entry)
	}
	operations := p.Manifest.Operations
	if operations == nil {
		operations = []WorkInfoOperation{}
	}
	return WorkInfoPluginEntry{
		ID:          p.ID,
		Name:        p.Manifest.Name,
		FileName:    p.FileName,
		Version:     p.Manifest.Version,
		Description: p.Manifest.Description,
		Operations:  operations,
		Digest:      p.Digest,
		State:       p.State(),
		Problem:     p.Problem,
		Settings:    settings,
		UpdatedAt:   p.UpdatedAt,
		LastRunAt:   p.LastRunAt,
	}
}

// CheckWorkInfoPluginValues requires declared keys and bounded valid UTF-8 values; nil deletes a saved value.
func CheckWorkInfoPluginValues(settings []WorkInfoPluginSetting, values map[string]*string) error {
	declared := make(map[string]bool, len(settings))
	for _, s := range settings {
		declared[s.Key] = true
	}
	for key, value := range values {
		if !declared[key] {
			return ErrWorkInfoPluginSettingInvalid
		}
		if value != nil && (!utf8.ValidString(*value) || utf8.RuneCountInString(*value) > WorkInfoPluginSettingValueMaxRunes) {
			return ErrWorkInfoPluginSettingInvalid
		}
	}
	return nil
}
