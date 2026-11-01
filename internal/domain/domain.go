// Package domain defines habits, day keys, settings and their validation.
// It imports only the standard library.
package domain

import (
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// Limits and defaults for habit names, day keys and the accent color.
const (
	// MaxNameRunes is the longest habit name, counted in runes.
	MaxNameRunes = 60
	// DefaultName replaces a blank habit name.
	DefaultName = "Untitled"
	// DayLayout is the time layout of a day key (YYYY-MM-DD).
	DayLayout = "2006-01-02"
	// DefaultAccent is the accent of a fresh database, the first palette color.
	DefaultAccent = "#4FA872"
)

// View is how the habit list is laid out.
type View string

// The supported views.
const (
	ViewWeek  View = "week"
	ViewMonth View = "month"
	ViewYear  View = "year"
	// DefaultView is the view of a fresh database.
	DefaultView = ViewWeek
)

// ValidationError is bad client input; handlers map it to 400 with its text.
type ValidationError string

// Error returns the text shown to the client.
func (e ValidationError) Error() string { return string(e) }

// Bad client input.
const (
	ErrInvalidID     ValidationError = "invalid habit id: want a lowercase UUID"
	ErrNameRequired  ValidationError = "name is required"
	ErrNameTooLong   ValidationError = "name is longer than 60 characters"
	ErrInvalidDay    ValidationError = "invalid day: want YYYY-MM-DD"
	ErrInvalidView   ValidationError = "invalid view: want week, month or year"
	ErrInvalidAccent ValidationError = "invalid accent: want one of the palette colors"
	ErrEmptySettings ValidationError = "settings: nothing to update"
)

// StateError is a conflict with stored state; handlers map ErrNotFound to 404, ErrExists to 409.
type StateError string

// Error returns the text shown to the client.
func (e StateError) Error() string { return string(e) }

// Conflicts with stored state.
const (
	ErrNotFound StateError = "habit not found"
	ErrExists   StateError = "habit already exists"
)

// Habit is a tracked habit with the days it was done.
type Habit struct {
	ID   string
	Name string
	Done []string // day keys, ascending; never nil in State
}

// State is everything the client renders.
type State struct {
	Habits []Habit // non-deleted, position order; never nil
	View   View
	Accent string
}

// SettingsUpdate is a validated partial settings change; a nil field is left as stored.
type SettingsUpdate struct {
	View   *View
	Accent *string
}

// Palette returns the accent colors in display order. Each call returns a fresh slice.
func Palette() []string {
	return []string{
		"#4FA872", "#8FBF4A", "#3FB8B0", "#5B8CEB", "#B07FE0",
		"#E07BB0", "#D96A6A", "#E08A45", "#E3C24F", "#ECE7DC",
	}
}

// SeedNames returns the habits of a first run. Each call returns a fresh slice.
func SeedNames() []string {
	return []string{"Drink 2 L of water", "Walk 30 minutes", "Read 10 pages"}
}

// ValidateID accepts a lowercase UUID of any version: 36 bytes, dashes at 8, 13, 18 and 23, lowercase hex elsewhere.
func ValidateID(id string) error {
	if len(id) != 36 {
		return ErrInvalidID
	}
	for i := range len(id) {
		switch i {
		case 8, 13, 18, 23:
			if id[i] != '-' {
				return ErrInvalidID
			}
		default:
			if !isLowerHex(id[i]) {
				return ErrInvalidID
			}
		}
	}
	return nil
}

func isLowerHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
}

// NormalizeName trims the name, maps a blank one to DefaultName and rejects one longer than MaxNameRunes.
func NormalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return DefaultName, nil
	}
	if utf8.RuneCountInString(name) > MaxNameRunes {
		return "", ErrNameTooLong
	}
	return name, nil
}

// ValidateDay accepts a real calendar day written as YYYY-MM-DD.
func ValidateDay(day string) error {
	t, err := time.Parse(DayLayout, day)
	if err != nil || t.Format(DayLayout) != day {
		return ErrInvalidDay
	}
	return nil
}

// ParseView accepts exactly "week", "month" or "year".
func ParseView(s string) (View, error) {
	switch v := View(s); v {
	case ViewWeek, ViewMonth, ViewYear:
		return v, nil
	default:
		return "", ErrInvalidView
	}
}

// ValidateAccent accepts exactly one of the Palette colors.
func ValidateAccent(accent string) error {
	if !slices.Contains(Palette(), accent) {
		return ErrInvalidAccent
	}
	return nil
}

// NewSettingsUpdate validates the non-nil fields, view first, and returns an update that holds copies of them.
// Both nil is ErrEmptySettings.
func NewSettingsUpdate(view, accent *string) (SettingsUpdate, error) {
	if view == nil && accent == nil {
		return SettingsUpdate{}, ErrEmptySettings
	}
	var u SettingsUpdate
	if view != nil {
		v, err := ParseView(*view)
		if err != nil {
			return SettingsUpdate{}, err
		}
		u.View = &v
	}
	if accent != nil {
		if err := ValidateAccent(*accent); err != nil {
			return SettingsUpdate{}, err
		}
		a := *accent
		u.Accent = &a
	}
	return u, nil
}
