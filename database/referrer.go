package database

import (
	"encoding/base32"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"

	"gitlab.com/NebulousLabs/errors"
)

const (
	// ReferrerTypeHNS is for referrers that use handshake domains. The
	// canonical representation is the name of the domain, i.e.
	// `skygallery.hns.siasky.net` is normalised to `skygallery`.
	ReferrerTypeHNS = "hns"
	// ReferrerTypeSkylink is for referrers that are regular skapps, either base64
	// or base32 ones. The canonical representation is base64.
	ReferrerTypeSkylink = "skylink"
	// ReferrerTypeWeb is for regular websites. Their canonical representation
	// is their domain name, i.e. cnn.com.
	ReferrerTypeWeb = "web"
)

var (
	// ExtractHNSDomainRE matches an HNS domain used in a skynet domain name
	ExtractHNSDomainRE = regexp.MustCompile("^(http[s]?://)?([a-zA-Z0-9-_]+)\\.hns\\..*$")
	// ExtractHNSPathRE matches an HNS domain used in a skynet path
	ExtractHNSPathRE = regexp.MustCompile("^.*/hns/([a-zA-Z0-9-_]+).*$")
	// ExtractDomainRE matches a standard web domain name
	ExtractDomainRE = regexp.MustCompile("^(http[s]?://)?([a-zA-Z0-9-.]+)(/[a-zA-Z0-9-_!?&=.]*)*$")

	// ErrorReferrerEmpty is returned when there is no referrer given
	ErrorReferrerEmpty = errors.New("empty referrer")
)

type (
	// Referrer is a web application which creates or controls some Skynet
	// traffic.
	Referrer struct {
		CanonicalName string `bson:"canonical_name" json:"canonicalName"`
		Type          string `bson:"type" json:"type"`
	}
)

// FromString parses a given string and returns a Referrer.
func FromString(s string) (Referrer, error) {
	if len(s) == 0 {
		return Referrer{}, ErrorReferrerEmpty
	}
	// Detect HNS domain
	m := ExtractHNSDomainRE.FindStringSubmatch(s)
	if len(m) > 2 {
		r := Referrer{
			CanonicalName: m[2],
			Type:          ReferrerTypeHNS,
		}
		return r, nil
	}
	// Detect HNS path
	m = ExtractHNSPathRE.FindStringSubmatch(s)
	if len(m) > 1 {
		r := Referrer{
			CanonicalName: m[1],
			Type:          ReferrerTypeHNS,
		}
		return r, nil
	}
	// Detect base64 skylink
	m = ExtractSkylinkBase64RE.FindStringSubmatch(s)
	if len(m) > 1 {
		r := Referrer{
			CanonicalName: m[1],
			Type:          ReferrerTypeSkylink,
		}
		return r, nil
	}
	// Detect base32 skylink
	m = ExtractSkylinkBase32RE.FindStringSubmatch(strings.ToUpper(s))
	if len(m) > 1 {
		base32Name := m[1]
		name, err := base32.HexEncoding.WithPadding(base32.NoPadding).DecodeString(base32Name)
		if err != nil {
			return Referrer{}, errors.New(fmt.Sprintf("failed to parse base32 skylink: %v", err))
		}
		r := Referrer{
			CanonicalName: base64.RawURLEncoding.EncodeToString(name),
			Type:          ReferrerTypeSkylink,
		}
		return r, nil
	}
	// Detect web address
	m = ExtractDomainRE.FindStringSubmatch(s)
	if len(m) > 2 {
		r := Referrer{
			CanonicalName: m[2],
			Type:          ReferrerTypeWeb,
		}
		return r, nil
	}
	return Referrer{}, errors.New("failed to detect referrer type")
}
