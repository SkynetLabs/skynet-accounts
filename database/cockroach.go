package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/mail"
	"net/url"
	"os"
	"time"

	_ "github.com/lib/pq" // Import postgres driver.
	"gitlab.com/NebulousLabs/errors"
)

var (
	// ErrCockroachDBNotConfigured is returned when the configuration for
	// CockroachDB is missing.
	ErrCockroachDBNotConfigured = errors.New("cockroachdb not configured")
)

// CockroachUser represents a user in CockroachDB
type CockroachUser struct {
	Sub       string    `json:"sub"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	PassHash  string    `json:"pass_hash"`
}

// NewCockroachDB returns a connection CockroachDB.
func NewCockroachDB() (*sql.DB, error) {
	user := os.Getenv("COCKROACH_DB_USER")
	password := os.Getenv("COCKROACH_DB_PASS")
	host := os.Getenv("COCKROACH_DB_HOST")
	port := os.Getenv("COCKROACH_DB_PORT")
	if user == "" || password == "" || host == "" || port == "" {
		return nil, ErrCockroachDBNotConfigured
	}
	dbURL := fmt.Sprintf("postgres://%s:%s@%s:%s/defaultdb", user, url.QueryEscape(password), host, port)
	parsedURL, err := url.Parse(dbURL)
	if err != nil {
		return nil, err
	}
	return sql.Open("postgres", parsedURL.String())
}

// CockroachUserByEmail fetches a user from CockroachDB.
func CockroachUserByEmail(db *sql.DB, email string) (*CockroachUser, error) {
	if db == nil {
		return nil, errors.New("missing cockroachdb connection")
	}
	// Input validation.
	e, err := mail.ParseAddress(email)
	if err != nil {
		return nil, errors.AddContext(err, "invalid email address")
	}
	query := `
		SELECT i.id as sub, traits as email, i.created_at, config as pass_hash
		FROM identities AS i LEFT JOIN identity_credentials AS ic ON i.id = ic.identity_id
		WHERE traits @> '{"email":"%s"}';`
	row := db.QueryRow(fmt.Sprintf(query, e.Address))
	if row.Err() != nil {
		return nil, row.Err()
	}
	var sub, emailstr, passhash string
	var createdAt time.Time
	err = row.Scan(&sub, &emailstr, &createdAt, &passhash)
	if err != nil {
		return nil, err
	}
	// Clean up the email
	var getEmail struct {
		Email string `json:"email"`
	}
	err = json.Unmarshal([]byte(emailstr), &getEmail)
	if err != nil {
		return nil, err
	}
	// Clean up the password hash
	var ph struct {
		HashedPassword string `json:"hashed_password"`
	}
	err = json.Unmarshal([]byte(passhash), &ph)
	if err != nil {
		return nil, err
	}
	return &CockroachUser{
		Sub:       sub,
		Email:     getEmail.Email,
		CreatedAt: createdAt,
		PassHash:  ph.HashedPassword,
	}, nil
}
