package database

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/crypto/ed25519"
)

const (
	// ChallengeSignatureSize is the size of the expected signature.
	ChallengeSignatureSize = ed25519.SignatureSize
	// ChallengeSize defines the number of bytes of entropy to send as a
	// challenge
	ChallengeSize = 32
	// ChallengeTypeLogin is the type of the login challenge.
	ChallengeTypeLogin = "skynet-portal-login"
	// ChallengeTypeRegister is the type of the registration challenge.
	ChallengeTypeRegister = "skynet-portal-register"
	// ChallengeTypeUpdate is the type of the update challenge which we use when
	// we register a new pubkey for the user.
	ChallengeTypeUpdate = "skynet-portal-update"

	// PubKeySize defines the length of the public key in bytes.
	PubKeySize = ed25519.PublicKeySize

	// challengeTTL defines how long we accept responses to this challenge.
	challengeTTL = 10 * time.Minute
)

var (
	// ErrInvalidChallengeResponse is returned when the received challenge
	// response object is not valid, i.e. it's either missing one of its
	// required fields or those do not follow the expected format.
	ErrInvalidChallengeResponse = errors.New("invalid response")
	// ErrInvalidPublicKey is returned when the provided public key is invalid.
	ErrInvalidPublicKey = errors.New("invalid pubKey provided")
	// PortalName is the name this portal uses to announce itself to the world.
	// Its value is controlled by the PORTAL_DOMAIN environment variable.
	// This name does not have a protocol prefix.
	PortalName = "https://siasky.net"
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

	// ChallengeResponse defines the format of a fully parsed and validated
	// response to a challenge.
	ChallengeResponse struct {
		Response  []byte `json:"response"`
		Signature []byte `json:"signature"`
	}

	// PubKey represents a public key. It's a helper type used to make function
	// signatures more readable.
	PubKey ed25519.PublicKey

	// UnconfirmedUserUpdate contains a user update that should be applied once
	// the respective challenge has been successfully responded to.
	UnconfirmedUserUpdate struct {
		Sub         string             `bson:"sub"`
		ChallengeID primitive.ObjectID `bson:"challenge_id"`
		ExpiresAt   time.Time          `bson:"expires_at"`
	}

	// ChallengeResponseRequest defines the format in which the caller will deliver
	// its response to a challenge.
	ChallengeResponseRequest struct {
		Response  string `json:"response"`
		Signature string `json:"signature"`
	}
)

// NewChallenge creates a new challenge with the given type and pubKey.
func (db *DB) NewChallenge(ctx context.Context, pubKey PubKey, cType string) (*Challenge, error) {
	if cType != ChallengeTypeLogin && cType != ChallengeTypeRegister && cType != ChallengeTypeUpdate {
		return nil, fmt.Errorf("invalid challenge type '%s'", cType)
	}
	ch := &Challenge{
		Challenge: hex.EncodeToString(fastrand.Bytes(ChallengeSize)),
		Type:      cType,
		PubKey:    pubKey,
		ExpiresAt: time.Now().UTC().Add(challengeTTL),
	}
	ior, err := db.staticChallenges.InsertOne(ctx, ch)
	if err != nil {
		return nil, errors.AddContext(err, "failed to create challenge DB record")
	}
	ch.ID = ior.InsertedID.(primitive.ObjectID)
	return ch, nil
}

// ValidateChallengeResponse validates the challenge response against the
// database. It makes sure the challenge and type in the response match what's
// in the database and that the signature is valid.
//
// Challenge format: challenge + type + recipient
func (db *DB) ValidateChallengeResponse(ctx context.Context, chr ChallengeResponse, expType string) (PubKey, primitive.ObjectID, error) {
	resp := chr.Response
	// Get the challenge type which sits right after the challenge in the
	// response.
	var cType string
	if strings.HasPrefix(string(resp[ChallengeSize:]), ChallengeTypeLogin) {
		cType = ChallengeTypeLogin
	} else if strings.HasPrefix(string(resp[ChallengeSize:]), ChallengeTypeRegister) {
		cType = ChallengeTypeRegister
	} else if strings.HasPrefix(string(resp[ChallengeSize:]), ChallengeTypeUpdate) {
		cType = ChallengeTypeUpdate
	} else {
		return nil, primitive.ObjectID{}, errors.New("invalid challenge type")
	}
	if cType != expType {
		return nil, primitive.ObjectID{}, errors.New("unexpected challenge type")
	}
	// Now that we know the challenge type, we can get the recipient as well.
	recipientOffset := ChallengeSize + len([]byte(cType))
	// Extract recipient from response.
	recipient := string(resp[recipientOffset:])
	// Check if the recipient is the current portal or any of its subdomains.
	recipientURL, err := url.Parse(recipient)
	if err != nil {
		return nil, primitive.ObjectID{}, errors.AddContext(err, "failed to parse recipient")
	}
	// The recipient should match the portal name.
	if fmt.Sprintf("%s://%s", recipientURL.Scheme, recipientURL.Host) != PortalName {
		return nil, primitive.ObjectID{}, fmt.Errorf("invalid recipient host %v != %v", recipientURL.Host, PortalName)
	}
	// Require HTTPS
	if recipientURL.Scheme != "https" {
		return nil, primitive.ObjectID{}, fmt.Errorf("invalid scheme %v, should be https", recipientURL.Scheme)
	}
	// Fetch the challenge from the DB.
	filter := bson.M{
		"challenge": hex.EncodeToString(resp[:ChallengeSize]),
		"type":      cType,
	}
	sr := db.staticChallenges.FindOne(ctx, filter)
	var ch Challenge
	err = sr.Decode(&ch)
	if err != nil {
		return nil, primitive.ObjectID{}, errors.AddContext(err, "challenge not found")
	}
	if ch.ExpiresAt.Before(time.Now().UTC()) {
		return nil, primitive.ObjectID{}, errors.New("challenge expired")
	}
	if !verifySignature(ch.PubKey, resp, chr.Signature) {
		return nil, primitive.ObjectID{}, errors.New("invalid signature")
	}
	// Now that the challenge has been used, we delete it from the DB. If this
	// errors out we'll log the error but we will still return success to the
	// caller.
	_, err = db.staticChallenges.DeleteOne(ctx, bson.M{"_id": ch.ID})
	if err != nil {
		db.staticLogger.Debugln("Failed to delete challenge from DB:", err)
	}
	// Clean up all expired challenges as well.
	_, err = db.staticChallenges.DeleteMany(ctx, bson.M{"expires_at": bson.M{"$lt": time.Now().UTC()}})
	if err != nil {
		db.staticLogger.Debugln("Failed to delete expired challenges from DB:", err)
	}
	return ch.PubKey, ch.ID, nil
}

// StoreUnconfirmedUserUpdate stores an UnconfirmedUserUpdate in the DB.
func (db *DB) StoreUnconfirmedUserUpdate(ctx context.Context, uu *UnconfirmedUserUpdate) error {
	_, err := db.staticUnconfirmedUserUpdates.InsertOne(ctx, uu)
	return err
}

// FetchUnconfirmedUserUpdate fetches an UnconfirmedUserUpdate from the DB.
func (db *DB) FetchUnconfirmedUserUpdate(ctx context.Context, chID primitive.ObjectID) (*UnconfirmedUserUpdate, error) {
	sr := db.staticUnconfirmedUserUpdates.FindOne(ctx, bson.M{"challenge_id": chID})
	if sr.Err() != nil {
		return nil, sr.Err()
	}
	uu := &UnconfirmedUserUpdate{}
	err := sr.Decode(uu)
	if err != nil {
		return nil, err
	}
	return uu, nil
}

// DeleteUnconfirmedUserUpdate deletes an UnconfirmedUserUpdate from the DB.
func (db *DB) DeleteUnconfirmedUserUpdate(ctx context.Context, chID primitive.ObjectID) error {
	_, err := db.staticUnconfirmedUserUpdates.DeleteOne(ctx, bson.M{"challenge_id": chID})
	// Do some cleanup while we're here and remove all expired updates.
	_, _ = db.staticUnconfirmedUserUpdates.DeleteMany(ctx, bson.M{"expires_at": bson.M{"$lt": time.Now().UTC()}})
	return err
}

// LoadFromBytes loads a ChallengeResponse from a []byte.
func (cr *ChallengeResponse) LoadFromBytes(b []byte) error {
	if b == nil {
		return errors.New("invalid input")
	}
	return cr.LoadFromReader(bytes.NewBuffer(b))
}

// LoadFromReader loads a ChallengeResponse from the given io.Reader.
//
// Typically, this reader will be a request.Body.
func (cr *ChallengeResponse) LoadFromReader(r io.Reader) error {
	var payload ChallengeResponseRequest
	err := json.NewDecoder(r).Decode(&payload)
	if err != nil {
		return errors.AddContext(err, ErrInvalidChallengeResponse.Error())
	}
	resp, err := hex.DecodeString(payload.Response)
	if err != nil {
		return errors.AddContext(err, "failed to parse the response")
	}
	if len(resp) < ChallengeSize {
		return ErrInvalidChallengeResponse
	}
	sig, err := hex.DecodeString(payload.Signature)
	if err != nil {
		return errors.AddContext(err, "failed to parse the response signature")
	}
	if len(sig) != ChallengeSignatureSize {
		return errors.New("invalid signature")
	}
	cr.Response = resp
	cr.Signature = sig
	return nil
}

// LoadString loads a PubKey from its hex-encoded string form.
func (pk *PubKey) LoadString(s string) error {
	b, err := hex.DecodeString(s)
	if err != nil {
		return errors.AddContext(err, ErrInvalidPublicKey.Error())
	}
	if len(b) != PubKeySize {
		return ErrInvalidPublicKey
	}
	*pk = b[:]
	return nil
}

// String converts a PubKey to its hex-encoded string form.
func (pk PubKey) String() string {
	return hex.EncodeToString(pk)
}

// verifySignature is a helper method.
func verifySignature(pk PubKey, message []byte, sig []byte) bool {
	return ed25519.Verify(ed25519.PublicKey(pk[:]), message[:], sig[:])
}
