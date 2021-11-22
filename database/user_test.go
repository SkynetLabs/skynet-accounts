package database

import (
	"encoding/json"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// TestMonthStart ensures we calculate the start of the subscription month
// correctly.
func TestMonthStart(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		subUntil     time.Time
		startOfMonth time.Time
	}{
		{
			// If the monthly sub cycle reset yesterday, expect the start of
			// month to also be yesterday.
			subUntil:     time.Date(2020, 1, now.Day()-1, 3, 4, 5, 6, time.UTC),
			startOfMonth: time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, time.UTC),
		},
		{
			// If the monthly sub cycle resets today, expect the start of
			// month to also be today.
			subUntil:     time.Date(2020, 1, now.Day(), 3, 4, 5, 6, time.UTC),
			startOfMonth: time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC),
		},
		{
			// If the monthly sub cycle resets tomorrow, expect the start of
			// month to be tomorrows date but a month back.
			subUntil:     time.Date(2020, 1, now.Day()+1, 3, 4, 5, 6, time.UTC),
			startOfMonth: time.Date(now.Year(), now.Month()-1, now.Day()+1, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		som := monthStart(tt.subUntil)
		if som != tt.startOfMonth {
			t.Errorf("Expected a sub ending on %v to have startOfMonth on %v but got %v.", tt.subUntil.String(), tt.startOfMonth.String(), som.String())
		}
	}
}

func TestUser_UnmarshalJSON(t *testing.T) {
	name := t.Name()
	u := &User{
		ID:              primitive.ObjectID{},
		Email:           name + "@siasky.net",
		SubscribedUntil: time.Now(),
		EmailConfirmationToken: "initial",
	}

	// We have a user with a confirmed email (no token).
	// Marshal and unmarshal the user.
	// We expect the EmailConfirmed field to be `true`.
	b, err := json.Marshal(u)
	if err != nil {
		t.Fatal(err)
	}
	err = json.Unmarshal(b, u)
	if err != nil {
		t.Fatal(err)
	}
	if u.EmailConfirmed {
		t.Fatal("Expected the email to be unconfirmed.")
	}

	// Set a confirmation token. Don't touch the EmailConfirmed field.
	u.EmailConfirmationToken = "set"
	// Marshal and unmarshal the user.
	// We expect the EmailConfirmed field to be `false`.
	b, err = json.Marshal(u)
	if err != nil {
		t.Fatal(err)
	}
	err = json.Unmarshal(b, u)
	if err != nil {
		t.Fatal(err)
	}
	if u.EmailConfirmed {
		t.Fatal("Expected the email to be unconfirmed.")
	}

	// Remove the confirmation token. Don't touch the EmailConfirmed field.
	u.EmailConfirmationToken = ""
	// Marshal and unmarshal the user.
	// We expect the EmailConfirmed field to be `true`.
	b, err = json.Marshal(u)
	if err != nil {
		t.Fatal(err)
	}
	err = json.Unmarshal(b, u)
	if err != nil {
		t.Fatal(err)
	}
	if !u.EmailConfirmed {
		t.Fatal("Expected the email to be confirmed.")
	}
}
