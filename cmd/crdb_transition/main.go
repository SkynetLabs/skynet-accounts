package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/NebulousLabs/skynet-accounts/database"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq" // Import postgres driver.
	"github.com/sirupsen/logrus"
	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// cru represents a user in CockroachDB
type cru struct {
	Sub       string    `json:"sub"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	PassHash  string    `json:"pass_hash"`
}

func connCockroachDB(creds database.DBCredentials) (*sql.DB, error) {
	dbURL := fmt.Sprintf("postgres://%s:%s@%s:%s/defaultdb", creds.User, url.QueryEscape(creds.Password), creds.Host, creds.Port)
	parsedURL, err := url.Parse(dbURL)
	if err != nil {
		return nil, err
	}
	return sql.Open("postgres", parsedURL.String())
}

func closeCockroachDB(conn *sql.DB) error {
	return conn.Close()
}

func connMongoDB(ctx context.Context, mongoCreds database.DBCredentials) (*database.DB, error) {
	return database.New(ctx, mongoCreds, &logrus.Logger{})
}

func rawConnMongoDB(ctx context.Context, mongoCreds database.DBCredentials) (*mongo.Database, error) {
	connStr := fmt.Sprintf(
		"mongodb://%s:%s@%s:%s/?compressors=%s&readPreference=%s&w=%s&wtimeoutMS=%s",
		url.QueryEscape(mongoCreds.User),
		url.QueryEscape(mongoCreds.Password),
		mongoCreds.Host,
		mongoCreds.Port,
		"zstd,zlib,snappy",
		"primary",
		"majority",
		"1000",
	)
	c, err := mongo.NewClient(options.Client().ApplyURI(connStr))
	if err != nil {
		return nil, errors.AddContext(err, "failed to create a new DB client")
	}
	err = c.Connect(ctx)
	if err != nil {
		return nil, errors.AddContext(err, "failed to connect to DB")
	}
	return c.Database("skynet"), nil
}

func listAllUsersCockroachDB(creds database.DBCredentials) map[string]cru {
	cr, err := connCockroachDB(creds)
	if err != nil {
		panic(err)
	}
	defer closeCockroachDB(cr)

	query := `
	SELECT i.id as sub, traits as email, i.created_at, config as pass_hash FROM identities AS i
	LEFT JOIN identity_credentials AS ic
	ON i.id = ic.identity_id
	WHERE 1=1`
	rows, err := cr.Query(string(query))
	if err != nil {
		panic(err)
	}
	users := map[string]cru{}
	var u cru
	for rows.Next() {
		err = rows.Scan(&u.Sub, &u.Email, &u.CreatedAt, &u.PassHash)
		if err != nil {
			panic(err)
		}
		var getEmail struct {
			Email string `json:"email"`
		}
		// Clean up the email
		err = json.Unmarshal([]byte(u.Email), &getEmail)
		if err != nil {
			panic(err)
		}
		u.Email = getEmail.Email
		// Clean up the password hash
		var ph struct {
			HashedPassword string `json:"hashed_password"`
		}
		err = json.Unmarshal([]byte(u.PassHash), &ph)
		if err != nil {
			panic(err)
		}
		u.PassHash = ph.HashedPassword
		users[u.Sub] = u
	}
	return users
}

func cleanMongoDB(creds database.DBCredentials) {
	ctx := context.Background()
	mgr, err := rawConnMongoDB(ctx, creds)
	if err != nil {
		panic(err)
	}
	deleteUserDataByID(ctx, mgr, "605325e9afc2f60129d1d109")
	deleteUserDataByID(ctx, mgr, "605334c89dfb881b847bf289")
}

func deleteUserDataByID(ctx context.Context, db *mongo.Database, id string) {
	uid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		panic(err)
	}
	filter := bson.M{"_id": uid}
	if _, err = db.Collection("users").DeleteOne(ctx, filter); err != nil {
		panic(err)
	}
	filter = bson.M{"user_id": uid}
	if _, err = db.Collection("skylinks").DeleteOne(ctx, filter); err != nil {
		panic(err)
	}
	if _, err = db.Collection("uploads").DeleteOne(ctx, filter); err != nil {
		panic(err)
	}
	if _, err = db.Collection("downloads").DeleteOne(ctx, filter); err != nil {
		panic(err)
	}
	if _, err = db.Collection("registry_reads").DeleteOne(ctx, filter); err != nil {
		panic(err)
	}
	if _, err = db.Collection("registry_writes").DeleteOne(ctx, filter); err != nil {
		panic(err)
	}
}

func updateUsersMongoDB(users map[string]cru, creds database.DBCredentials) {
	ctx := context.Background()
	mg, err := connMongoDB(ctx, creds)
	if err != nil {
		panic(err)
	}
	for sub, ucr := range users {
		u, err := mg.UserBySub(ctx, sub, false)
		if err != nil {
			panic(err)
		}
		u.Email = ucr.Email
		u.PasswordHash = ucr.PassHash
		u.CreatedAt = ucr.CreatedAt
		err = mg.UserSave(ctx, u)
		if err != nil {
			panic(err)
		}
	}
}

func dbCredsMongoDB() database.DBCredentials {
	return database.DBCredentials{
		User:     os.Getenv("SKYNET_DB_USER"),
		Password: os.Getenv("SKYNET_DB_PASS"),
		Host:     os.Getenv("SKYNET_DB_HOST"),
		Port:     os.Getenv("SKYNET_DB_PORT"),
	}
}

func dbCredsCockroachDB() database.DBCredentials {
	return database.DBCredentials{
		User:     os.Getenv("COCKROACH_DB_USER"),
		Password: os.Getenv("COCKROACH_DB_PASS"),
		Host:     os.Getenv("COCKROACH_DB_HOST"),
		Port:     os.Getenv("COCKROACH_DB_PORT"),
	}
}

func main() {
	_ = godotenv.Load()

	mongoCreds := dbCredsMongoDB()
	cleanMongoDB(mongoCreds)
	println("MongoDB cleaned.")

	// Get all users from CockroachDB:
	users := listAllUsersCockroachDB(dbCredsCockroachDB())
	println("Got users from cockroach: " + strconv.Itoa(len(users)))

	// Update all users:
	updateUsersMongoDB(users, mongoCreds)
	println("Moved data from CockroachDB to MongoDB.")
}
