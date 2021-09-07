package email

import (
	"html/template"
	"strings"

	"github.com/NebulousLabs/skynet-accounts/database"
)

/**
TODO
 - fix the links here
*/

const (
	confirmEmailSubject = "Please verify your email address"
	confirmEmailMime    = "multipart/alternative; boundary=e31b4aa4706e10c57d31a44da59281c216fb10992b0e5b512edea805408a"
	confirmEmailTempl   = `
--e31b4aa4706e10c57d31a44da59281c216fb10992b0e5b512edea805408a
Content-Transfer-Encoding: quoted-printable
Content-Type: text/plain; charset=UTF-8

Hi, please verify your account by clicking the following link:

<a href="https://siasky.net/confirmEmail?code={{.Token}}">https://siasky.net/confirmEmail?code={{.Token}}</a>

--e31b4aa4706e10c57d31a44da59281c216fb10992b0e5b512edea805408a
Content-Transfer-Encoding: quoted-printable
Content-Type: text/html; charset=UTF-8

Hi, please verify your account by clicking the following link:

<a href="https://siasky.net/confirmEmail?code={{.Token}}">https://siasky.net/confirmEmail?code={{.Token}}</a>

--e31b4aa4706e10c57d31a44da59281c216fb10992b0e5b512edea805408a
`

	recoverAccountSubject = "Recover access to your account"
	recoverAccountMime    = "multipart/alternative; boundary=9f0f6cc6978acbf34b218925c8b6be77292fcc0ec91a086b04045aafa8ca"
	recoverAccountTempl   = `
--9f0f6cc6978acbf34b218925c8b6be77292fcc0ec91a086b04045aafa8ca
Content-Transfer-Encoding: quoted-printable
Content-Type: text/plain; charset=UTF-8

Hi,

please recover access to your account by clicking the following link:

<a href="https://siasky.net/recoverAccount?code={{.Token}}">https://siasky.net/recoverAccount?code={{.Token}}</a>

--9f0f6cc6978acbf34b218925c8b6be77292fcc0ec91a086b04045aafa8ca
Content-Transfer-Encoding: quoted-printable
Content-Type: text/html; charset=UTF-8

Hi,

please recover access to your account by clicking the following link:

<a href="https://siasky.net/recoverAccount?code={{.Token}}">https://siasky.net/recoverAccount?code={{.Token}}</a>

--9f0f6cc6978acbf34b218925c8b6be77292fcc0ec91a086b04045aafa8ca--
`

	accountAccessAttemptedSubject = "Account access attempted"
	accountAccessAttemptedMime    = "multipart/alternative; boundary=f096ee1beed49f6757a41b4bf22d1ddc10cc9480a4df9376ebac4fe4f405"
	accountAccessAttemptedTempl   = `
--f096ee1beed49f6757a41b4bf22d1ddc10cc9480a4df9376ebac4fe4f405
Content-Transfer-Encoding: quoted-printable
Content-Type: text/plain; charset=UTF-8

Hi,

you (or someone else) entered this email address when trying to recover acc=
ess to an account.

However, this email address is not on our database of registered users and =
therefore the attempt has failed.

If this was you, check if you signed up using a different address.

If this was not you, please ignore this email.

--f096ee1beed49f6757a41b4bf22d1ddc10cc9480a4df9376ebac4fe4f405
Content-Transfer-Encoding: quoted-printable
Content-Type: text/html; charset=UTF-8

Hi,

you (or someone else) entered this email address when trying to recover acc=
ess to an account.

However, this email address is not on our database of registered users and =
therefore the attempt has failed.

If this was you, check if you signed up using a different address.

If this was not you, please ignore this email.

--f096ee1beed49f6757a41b4bf22d1ddc10cc9480a4df9376ebac4fe4f405--
`
)

type (
	confirmEmailData struct {
		Token string
	}
	recoverAccountData struct {
		Token string
	}
)

// confirmEmailEmail generates an email for confirming that the user owns the
// given email address.
func confirmEmailEmail(to string, data confirmEmailData) (*database.EmailMessage, error) {
	t, err := template.New("confirmEmail").Parse(confirmEmailTempl)
	if err != nil {
		return nil, err
	}
	var bodyBuilder strings.Builder
	err = t.Execute(&bodyBuilder, data)
	if err != nil {
		return nil, err
	}
	return &database.EmailMessage{
		From:     from,
		To:       to,
		Subject:  confirmEmailSubject,
		Body:     bodyBuilder.String(),
		BodyMime: confirmEmailMime,
	}, nil
}

// recoverAccountEmail generates an email for recovering an account.
func recoverAccountEmail(to string, data recoverAccountData) (*database.EmailMessage, error) {
	t, err := template.New("recoverAccount").Parse(recoverAccountTempl)
	if err != nil {
		return nil, err
	}
	var bodyBuilder strings.Builder
	err = t.Execute(&bodyBuilder, data)
	if err != nil {
		return nil, err
	}
	return &database.EmailMessage{
		From:     from,
		To:       to,
		Subject:  recoverAccountSubject,
		Body:     bodyBuilder.String(),
		BodyMime: recoverAccountMime,
	}, nil
}

// accountAccessAttemptedEmail generates an email for notifying a user that
// someone tried to use their email for recovering a Skynet account but their
// email is not in our system. The main reason to do that is because the user
// might have forgotten which email they used for signing up.
func accountAccessAttemptedEmail(to string) (*database.EmailMessage, error) {
	t, err := template.New("accountAccessAttempted").Parse(accountAccessAttemptedTempl)
	if err != nil {
		return nil, err
	}
	var bodyBuilder strings.Builder
	err = t.Execute(&bodyBuilder, nil)
	if err != nil {
		return nil, err
	}
	return &database.EmailMessage{
		From:     from,
		To:       to,
		Subject:  accountAccessAttemptedSubject,
		Body:     bodyBuilder.String(),
		BodyMime: accountAccessAttemptedMime,
	}, nil
}
