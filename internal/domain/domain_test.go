package domain

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestValidateID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want error
	}{
		{name: "lowercase uuid", id: "0f8fad5b-d9cb-469f-a165-70867728950e", want: nil},
		{name: "uppercase uuid", id: "0F8FAD5B-D9CB-469F-A165-70867728950E", want: ErrInvalidID},
		{name: "empty", id: "", want: ErrInvalidID},
		{name: "32 chars without dashes", id: "0f8fad5bd9cb469fa16570867728950e", want: ErrInvalidID},
		{name: "35 chars", id: "0f8fad5b-d9cb-469f-a165-70867728950", want: ErrInvalidID},
		{name: "non-hex letter", id: "0f8fad5b-d9cb-469f-a165-70867728950g", want: ErrInvalidID},
		{name: "underscore instead of dash", id: "0f8fad5b_d9cb-469f-a165-70867728950e", want: ErrInvalidID},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ValidateID(tc.id); !errors.Is(got, tc.want) {
				t.Errorf("ValidateID(%q) = %v, want %v", tc.id, got, tc.want)
			}
		})
	}
}

func TestNormalizeName(t *testing.T) {
	// "é" is two bytes but one rune: counting bytes would reject a name that fits.
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr error
	}{
		{name: "trims surrounding spaces", input: "  Read  ", want: "Read", wantErr: nil},
		{name: "empty becomes default", input: "", want: "Untitled", wantErr: nil},
		{name: "blank becomes default", input: "   ", want: "Untitled", wantErr: nil},
		{name: "60 multibyte runes", input: strings.Repeat("é", 60), want: strings.Repeat("é", 60), wantErr: nil},
		{name: "61 multibyte runes", input: strings.Repeat("é", 61), want: "", wantErr: ErrNameTooLong},
		{name: "60 ascii runes", input: strings.Repeat("a", 60), want: strings.Repeat("a", 60), wantErr: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeName(tc.input)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("NormalizeName(%q) error = %v, want %v", tc.input, err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("NormalizeName(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestValidateDay(t *testing.T) {
	tests := []struct {
		name string
		day  string
		want error
	}{
		{name: "regular day", day: "2026-11-01", want: nil},
		{name: "leap day", day: "2028-02-29", want: nil},
		{name: "29 February in a common year", day: "2026-02-29", want: ErrInvalidDay},
		{name: "month 13", day: "2026-13-01", want: ErrInvalidDay},
		{name: "no zero padding", day: "2026-1-1", want: ErrInvalidDay},
		{name: "timestamp", day: "2026-11-01T00:00:00Z", want: ErrInvalidDay},
		{name: "empty", day: "", want: ErrInvalidDay},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ValidateDay(tc.day); !errors.Is(got, tc.want) {
				t.Errorf("ValidateDay(%q) = %v, want %v", tc.day, got, tc.want)
			}
		})
	}
}

func TestParseView(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    View
		wantErr error
	}{
		{name: "week", input: "week", want: "week", wantErr: nil},
		{name: "month", input: "month", want: "month", wantErr: nil},
		{name: "year", input: "year", want: "year", wantErr: nil},
		{name: "capitalized", input: "Week", want: "", wantErr: ErrInvalidView},
		{name: "empty", input: "", want: "", wantErr: ErrInvalidView},
		{name: "unknown", input: "decade", want: "", wantErr: ErrInvalidView},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseView(tc.input)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("ParseView(%q) error = %v, want %v", tc.input, err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("ParseView(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestValidateAccent(t *testing.T) {
	// The palette is spelled out so that a changed color fails:
	// ranging over Palette() would accept whatever it returns.
	tests := []struct {
		name   string
		accent string
		want   error
	}{
		{name: "palette 1", accent: "#4FA872", want: nil},
		{name: "palette 2", accent: "#8FBF4A", want: nil},
		{name: "palette 3", accent: "#3FB8B0", want: nil},
		{name: "palette 4", accent: "#5B8CEB", want: nil},
		{name: "palette 5", accent: "#B07FE0", want: nil},
		{name: "palette 6", accent: "#E07BB0", want: nil},
		{name: "palette 7", accent: "#D96A6A", want: nil},
		{name: "palette 8", accent: "#E08A45", want: nil},
		{name: "palette 9", accent: "#E3C24F", want: nil},
		{name: "palette 10", accent: "#ECE7DC", want: nil},
		{name: "lowercase palette color", accent: "#4fa872", want: ErrInvalidAccent},
		{name: "color outside the palette", accent: "#000000", want: ErrInvalidAccent},
		{name: "empty", accent: "", want: ErrInvalidAccent},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ValidateAccent(tc.accent); !errors.Is(got, tc.want) {
				t.Errorf("ValidateAccent(%q) = %v, want %v", tc.accent, got, tc.want)
			}
		})
	}
}

func TestNewSettingsUpdate(t *testing.T) {
	tests := []struct {
		name       string
		view       *string
		accent     *string
		wantView   *View
		wantAccent *string
		wantErr    error
	}{
		{name: "both nil", wantErr: ErrEmptySettings},
		{name: "view only", view: new("month"), wantView: new(View("month"))},
		{name: "accent only", accent: new("#5B8CEB"), wantAccent: new("#5B8CEB")},
		{name: "invalid view, valid accent", view: new("decade"), accent: new("#5B8CEB"), wantErr: ErrInvalidView},
		{name: "valid view, invalid accent", view: new("week"), accent: new("#000000"), wantErr: ErrInvalidAccent},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			call := fmt.Sprintf("NewSettingsUpdate(%s, %s)", show(tc.view), show(tc.accent))
			got, err := NewSettingsUpdate(tc.view, tc.accent)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("%s error = %v, want %v", call, err, tc.wantErr)
			}
			if gotView, wantView := show(got.View), show(tc.wantView); gotView != wantView {
				t.Errorf("%s.View = %s, want %s", call, gotView, wantView)
			}
			if gotAccent, wantAccent := show(got.Accent), show(tc.wantAccent); gotAccent != wantAccent {
				t.Errorf("%s.Accent = %s, want %s", call, gotAccent, wantAccent)
			}
		})
	}
}

func TestPaletteIsCopy(t *testing.T) {
	p := Palette()
	p[0] = "x"

	if got := Palette()[0]; got != "#4FA872" {
		t.Errorf("Palette()[0] = %q after writing to an earlier result, want %q", got, "#4FA872")
	}
	if got := len(Palette()); got != 10 {
		t.Errorf("len(Palette()) = %d, want 10", got)
	}
}

func TestErrorTexts(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "not found", err: ErrNotFound, want: "habit not found"},
		{name: "exists", err: ErrExists, want: "habit already exists"},
		{name: "invalid day", err: ErrInvalidDay, want: "invalid day: want YYYY-MM-DD"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.err.Error(); got != tc.want {
				t.Errorf("Error() = %q, want %q", got, tc.want)
			}
		})
	}
}

// show prints a pointer by value, so a failure names the text instead of an address
// and a nil pointer stays distinct from a pointer to an empty string.
func show[T ~string](p *T) string {
	if p == nil {
		return "nil"
	}
	return fmt.Sprintf("%q", string(*p))
}
