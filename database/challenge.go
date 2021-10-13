package database

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/crypto/ed25519"
)

const (
	// ChallengeTypeLogin is the type of the login challenge.
	ChallengeTypeLogin = "login"
	// ChallengeTypeRegister is the type of the registration challenge.
	ChallengeTypeRegister = "register"

	// PubKeyLen defines the length of the public key in bytes.
	PubKeyLen = 32

	// challengeSize defines the number of bytes of entropy to send as a
	// Challenge
	challengeSize = 32
	// challengeTTL defines how long we accept responses to this challenge.
	challengeTTL = 30 * time.Second
)

type (
	// Challenge defines the format in which we will deliver our login and
	// registration challenges to the caller.
	Challenge struct {
		ID primitive.ObjectID `bson:"_id,omitempty" json:"-"`
		// Challenge is a hex-encoded representation of the []byte challenge.
		Challenge string    `bson:"challenge" json:"challenge"`
		Type      string    `bson:"type" json:"-"`
		PubKey    PubKey    `bson:"pub_key" json:"-"`
		ExpiresAt time.Time `bson:"expires_at" json:"-"`
	}

	// ChallengeResponse defines the format in which the caller will deliver
	// its response to our login and register challenges.
	ChallengeResponse struct {
		Response  []byte `json:"response"`
		Signature []byte `json:"signature"`
	}

	// PubKey represents a public key. It's a helper type used to make function
	// signatures more readable.
	PubKey ed25519.PublicKey
)

// NewChallenge creates a new challenge with the given type and pubKey.
func (db *DB) NewChallenge(ctx context.Context, pubKey PubKey, typ string) (*Challenge, error) {
	if typ != ChallengeTypeLogin && typ != ChallengeTypeRegister {
		return nil, errors.New(fmt.Sprintf("invalid challenge type '%s'", typ))
	}
	ch := &Challenge{
		Challenge: hex.EncodeToString(fastrand.Bytes(challengeSize)),
		Type:      typ,
		PubKey:    pubKey,
		ExpiresAt: time.Now().UTC().Add(challengeTTL),
	}
	_, err := db.staticChallenges.InsertOne(ctx, ch)
	if err != nil {
		return nil, errors.AddContext(err, "failed to create challenge DB record")
	}
	return ch, nil
}

// ValidateChallengeResponse validates the challenge response against the
// database. It makes sure the challenge and type in the response match what's
// in the database and that the signature is valid.
//
// Challenge format: challenge + type + recipient
func (db *DB) ValidateChallengeResponse(ctx context.Context, chr *ChallengeResponse) (PubKey, error) {
	resp := chr.Response
	var typ string
	if strings.HasPrefix(string(resp[challengeSize:]), ChallengeTypeLogin) {
		typ = ChallengeTypeLogin
	} else if strings.HasPrefix(string(resp[challengeSize:]), ChallengeTypeRegister) {
		typ = ChallengeTypeRegister
	} else {
		return nil, errors.New("invalid challenge type")
	}
	filter := bson.M{
		"challenge": hex.EncodeToString(resp[:challengeSize]),
		"type":      typ,
	}
	sr := db.staticChallenges.FindOne(ctx, filter)
	var ch Challenge
	err := sr.Decode(&ch)
	if err != nil {
		return nil, errors.AddContext(err, "challenge not found")
	}
	if ch.ExpiresAt.Before(time.Now().UTC()) {
		return nil, errors.New("challenge expired")
	}
	if !verifySignature(ch.PubKey, resp, chr.Signature) {
		return nil, errors.New("invalid signature")
	}
	_, err = db.staticChallenges.DeleteOne(ctx, bson.M{"_id": ch.ID})
	if err != nil {
		db.staticLogger.Debugln("Failed to delete challenge from DB:", err)
	}
	return ch.PubKey, nil
}

// verifySignature is a helper method.
func verifySignature(pk PubKey, message []byte, sig []byte) bool {
	return ed25519.Verify(ed25519.PublicKey(pk[:]), message[:], sig[:])
}
