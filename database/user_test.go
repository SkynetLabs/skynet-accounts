package database

import (
	"testing"
	"time"
)

// TestMonthStart ensures we calculate the start of the subscription month
// correctly.
func TestMonthStart(t *testing.T) {
	// We test with first of March for a reason - it's preceded by February,
	// which is both shorter and it changes its length.
	firstMarch22 := time.Date(2022, 3, 1, 12, 13, 14, 15, time.UTC)
	tests := []struct {
		subUntil     time.Time
		checkedOn    time.Time
		startOfMonth time.Time
	}{
		{
			// The sub expiration day of month precedes the current day of month.
			// We expect the start of month to be the same day during the previous month.
			subUntil:     time.Date(2020, 1, 15, 3, 4, 5, 6, time.UTC),
			checkedOn:    firstMarch22,
			startOfMonth: time.Date(2022, 2, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			// The sub expiration day of month is after the current day of month.
			// We expect the start of month to be the same day during the current month.
			subUntil:     time.Date(2020, 1, 15, 3, 4, 5, 6, time.UTC),
			checkedOn:    time.Date(2022, 3, 18, 2, 3, 4, 5, time.UTC),
			startOfMonth: time.Date(2022, 3, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			// The sub expires at the last day of the month. But the preceding month doesn't have that day.
			// We expect the start of month to be the last day of the previous month, even if the day is different.
			subUntil:     time.Date(2020, 1, 31, 3, 4, 5, 6, time.UTC),
			checkedOn:    firstMarch22,
			startOfMonth: time.Date(2022, 2, 28, 0, 0, 0, 0, time.UTC),
		},
		{
			// The sub expires at the last day of the month. But the preceding month doesn't have that day.
			// We expect the start of month to be the last day of the previous month, even if the day is different.
			// This case is exactly like the one above but covers a leap year.
			subUntil:     time.Date(2020, 1, 31, 3, 4, 5, 6, time.UTC),
			checkedOn:    time.Date(2024, 3, 1, 2, 3, 4, 5, time.UTC),
			startOfMonth: time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC),
		},
		{
			subUntil:     time.Date(2020, 1, 1, 3, 4, 5, 6, time.UTC),
			checkedOn:    time.Date(2022, 1, 1, 2, 3, 4, 5, time.UTC),
			startOfMonth: time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	df := "2006-01-02"
	for _, tt := range tests {
		som := monthStartWithTime(tt.subUntil, tt.checkedOn)
		if som != tt.startOfMonth {
			t.Errorf("Expected a sub ending on %s when checked on %s to have startOfMonth on %s but got %s.",
				tt.subUntil.Format(df), tt.checkedOn.Format(df), tt.startOfMonth.Format(df), som.Format(df))
		}
	}
}
