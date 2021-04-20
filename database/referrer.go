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
	ExtractHNSDomainRE = regexp.MustCompile("^http[s]?://([a-zA-Z0-9-_]+)\\.hns\\..*$")
	ExtractHNSPathRE   = regexp.MustCompile("^.*/hns/([a-zA-Z0-9-_]*).*$")
)

type (
	// Referrer is a web application which creates or controls some Skynet
	// traffic.
	Referrer struct {
		CanonicalName string
		Type          string
	}
)

// FromString parses a given string and returns a Referrer.
func FromString(s string) (*Referrer, error) {
	if len(s) == 0 {
		return nil, errors.New("empty referrer")
	}
	// Detect HNS domain
	m := ExtractHNSDomainRE.FindStringSubmatch(s)
	if len(m) == 2 {
		r := &Referrer{
			CanonicalName: m[1],
			Type:          ReferrerTypeHNS,
		}
		return r, nil
	}
	// Detect HNS path
	m = ExtractHNSPathRE.FindStringSubmatch(s)
	if len(m) == 2 {
		r := &Referrer{
			CanonicalName: m[1],
			Type:          ReferrerTypeHNS,
		}
		return r, nil
	}

	// Detect base64 skylink
	m = ExtractSkylinkBase64RE.FindStringSubmatch(s)
	if len(m) == 2 {
		r := &Referrer{
			CanonicalName: m[1],
			Type:          ReferrerTypeSkylink,
		}
		return r, nil
	}
	// Detect base32 skylink
	m = ExtractSkylinkBase32RE.FindStringSubmatch(strings.ToLower(s))
	if len(m) == 2 {
		base32Name := m[1]
		name, err := base32.StdEncoding.DecodeString(base32Name)
		if err != nil {
			return nil, errors.New(fmt.Sprintf("failed to parse base32 skylink: %v", err))
		}
		r := &Referrer{
			CanonicalName: base64.StdEncoding.EncodeToString(name),
			Type:          ReferrerTypeSkylink,
		}
		return r, nil
	}

	// TODO Detect web address

	return nil, errors.New("failed to detect referrer type")
}
