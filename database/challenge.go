package database

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
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

	// PubKeySize defines the length of the public key in bytes.
	PubKeySize = ed25519.PublicKeySize

	// challengeTTL defines how long we accept responses to this challenge.
	challengeTTL = 10 * time.Minute
)

var (
	// PortalName is the name this portal uses to announce itself to the world.
	// Its value is controlled by the PORTAL_DOMAIN environment variable.
	// This name does not have a protocol prefix.
	PortalName = "siasky.net"
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
func (db *DB) NewChallenge(ctx context.Context, pubKey PubKey, cType string) (*Challenge, error) {
	if cType != ChallengeTypeLogin && cType != ChallengeTypeRegister {
		return nil, errors.New(fmt.Sprintf("invalid challenge type '%s'", cType))
	}
	ch := &Challenge{
		Challenge: hex.EncodeToString(fastrand.Bytes(ChallengeSize)),
		Type:      cType,
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
func (db *DB) ValidateChallengeResponse(ctx context.Context, chr ChallengeResponse) (PubKey, error) {
	resp := chr.Response
	// Get the challenge type which sits right after the challenge in the
	// response.
	var cType string
	if strings.HasPrefix(string(resp[ChallengeSize:]), ChallengeTypeLogin) {
		cType = ChallengeTypeLogin
	} else if strings.HasPrefix(string(resp[ChallengeSize:]), ChallengeTypeRegister) {
		cType = ChallengeTypeRegister
	} else {
		return nil, errors.New("invalid challenge type")
	}
	// Now that we know the challenge type, we can get the recipient as well.
	recipientOffset := ChallengeSize + len([]byte(cType))
	recipient := string(resp[recipientOffset:])
	if recipient != PortalName {
		return nil, errors.New("invalid recipient " + recipient)
	}
	// Fetch the challenge from the DB.
	filter := bson.M{
		"challenge": hex.EncodeToString(resp[:ChallengeSize]),
		"type":      cType,
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
	return ch.PubKey, nil
}

// LoadFromRequest loads a ChallengeResponse by extracting its string form from
// http.Request.
func (cr *ChallengeResponse) LoadFromRequest(req *http.Request) error {
	resp, err := hex.DecodeString(req.PostFormValue("response"))
	if err != nil {
		return errors.AddContext(err, "failed to parse the response")
	}
	if len(resp) < ChallengeSize {
		return errors.New("invalid response")
	}
	sig, err := hex.DecodeString(req.PostFormValue("signature"))
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
	bytes, err := hex.DecodeString(s)
	if err != nil {
		return errors.AddContext(err, "invalid pubKey provided")
	}
	if len(bytes) != PubKeySize {
		return errors.New("invalid pubKey provided")
	}
	*pk = bytes[:]
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
