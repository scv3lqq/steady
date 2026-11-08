package httpapi

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/scv3lqq/steady/internal/domain"
)

func TestNewHabitDTOCreatedAt(t *testing.T) {
	tests := []struct {
		name  string
		habit domain.Habit
		want  string
	}{
		{
			name:  "unknown",
			habit: domain.Habit{ID: "a", Name: "Read", Done: []string{}},
			want:  `{"id":"a","name":"Read","done":[],"createdAt":null}`,
		},
		{
			name:  "known",
			habit: domain.Habit{ID: "a", Name: "Read", CreatedAt: time.Unix(1793525400, 0), Done: []string{}},
			want:  `{"id":"a","name":"Read","done":[],"createdAt":1793525400}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(newHabitDTO(tc.habit))
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("newHabitDTO(%+v) = %s, want %s", tc.habit, got, tc.want)
			}
		})
	}
}
