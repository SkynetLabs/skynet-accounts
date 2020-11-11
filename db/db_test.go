package db

import (
	"context"
	"os"
	"testing"

	"gitlab.com/NebulousLabs/errors"

	"github.com/NebulousLabs/skynet-accounts/user"
)

func initEnv() {
	os.Setenv("SKYNET_DB_HOST", "localhost")
	os.Setenv("SKYNET_DB_PORT", "27017")
	os.Setenv("SKYNET_DB_USER", "admin")
}

func TestDB_UserFindByID(t *testing.T) {
	initEnv()
	ctx := context.Background()

	db, err := New(ctx)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.UserFindByID(ctx, "5fac362ea361afa5def1bcb8")
	if !errors.Contains(err, ErrUserNotFound) {
		t.Fatalf("expected error '%s', got '%s'\n", ErrUserNotFound.Error(), err.Error())
	}
}

func TestDB(t *testing.T) {
	initEnv()
	ctx := context.Background()

	db, err := New(ctx)
	if err != nil {
		t.Fatal(err)
	}

	u := user.User{
		FirstName: "Ivaylo",
		LastName:  "Novakov",
		Email:     "ivaylo@nebulous.tech",
	}

	created, err := db.UserSave(ctx, &u)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("User with id %s was created: %t\n", u.ID, created)

	fu, err := db.UserFindByID(ctx, u.ID.Hex())
	if err != nil {
		t.Fatal(err)
	}
	if fu == nil {
		t.Fatal("expected to find a user but didn't")
	}

	//ok, err := db.UserDeleteByID(ctx, u.ID)
	//if err != nil {
	//	t.Fatal(err)
	//}
	//if !ok {
	//	t.Fatal("expected to be able to delete user but it was not found")
	//}
}
