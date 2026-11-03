package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateUsername(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"abc", true}, {"a_9-z", true}, {"a" + strings.Repeat("0", 31), true},
		{"", false}, {"ab", false}, {"a" + strings.Repeat("0", 32), false},
		{"Abc", false}, {" abc", false}, {"abc ", false}, {"9abc", false},
		{"_abc", false}, {"ab.c", false}, {"aéz", false}, {"abc\n", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateUsername(tc.name)
			if tc.valid && err != nil {
				t.Fatalf("valid username rejected: %v", err)
			}
			if !tc.valid && !errors.Is(err, ErrInvalidUsername) {
				t.Fatalf("error = %v, want ErrInvalidUsername", err)
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password []byte
		valid    bool
	}{
		{"minimum", []byte("abcdefghijkl"), true},
		{"maximum", []byte(strings.Repeat("x", 128)), true},
		{"spaces preserved", []byte("            "), true},
		{"UTF8 byte minimum", []byte(strings.Repeat("é", 6)), true},
		{"UTF8 byte maximum", []byte(strings.Repeat("é", 64)), true},
		{"empty", nil, false}, {"short", []byte("abcdefghijk"), false},
		{"long", []byte(strings.Repeat("x", 129)), false},
		{"UTF8 bytes not runes", []byte(strings.Repeat("é", 65)), false},
		{"malformed UTF8", append([]byte("abcdefghijkl"), 0xff), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			original := string(tc.password)
			err := ValidatePassword(tc.password)
			if tc.valid && err != nil {
				t.Fatalf("valid password rejected: %v", err)
			}
			if !tc.valid && !errors.Is(err, ErrInvalidPassword) {
				t.Fatalf("error = %v, want ErrInvalidPassword", err)
			}
			if string(tc.password) != original {
				t.Fatal("validation mutated password")
			}
		})
	}
}
