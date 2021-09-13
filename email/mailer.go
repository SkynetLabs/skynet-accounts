package email

import (
	"context"
	"crypto/tls"
	"net/url"
	"regexp"
	"strconv"

	"github.com/NebulousLabs/skynet-accounts/database"

	"gitlab.com/NebulousLabs/errors"
	"gopkg.in/mail.v2"
)

/**
This file contains the defining piece of logic for working with emails. The flow
is as follows:

A sender wants to send a message. In order to do that they use an instance of
`Mailer` to `Send` a message. `Mailer` doesn't actually send the message
but queues it up in the database for future processing. A background thread
running `Sender` is looping over the DB on a timer and taking care to send the
messages waiting there.

Since the DB is shared between all nodes in the portal cluster, the process of
sending a message can be initiated by any node, regardless of the origin of the
message. The way this works is that whenever `Sender` is triggered, it scans the
DB for waiting messages, locks up a batch of them by writing its identity in the
`LockedBy` field of each, waits for a short time, and then retrieves all
messages locked by its id. Then it proceeds to attempt to send them. Should it
succeed, the `SentAt` timestamp is set on each successfully sent message, thus
marking it as processed. Should it fail to send the message, `Sender` will
unlock it, so it can be retried later. If a message fails to get sent more than
`maxAttemptsToSend` times it is marked as `Failed`.
*/

const (
	// emailBatchSize defines the largest batch of emails we will try to send.
	emailBatchSize = 10
)

var (
	// ConnectionURI is the connection string used for sending emails. Its
	// value is controlled by the ACCOUNTS_EMAIL_URI environment variable.
	ConnectionURI = "smtps://test:test@mailslurper:1025/?skip_ssl_verify=true"

	// From is the address we send emails from. It defaults to the user
	// from ConnectionURI but can be overridden by the ACCOUNTS_EMAIL_FROM
	// environment variable.
	From = "noreply@siasky.net"

	// PortalAddress defines the URI where we can access our portal. Its value
	// comes from the PORTAL_DOMAIN environment variable, preceded by the
	// appropriate schema.
	PortalAddress = "https://siasky.net"

	// matchPattern extracts all relevant configuration values from an email
	// connection URI
	matchPattern = regexp.MustCompile("smtps://(?P<user>.*):(?P<password>.*)@(?P<server>.*):(?P<port>\\d*)(/\\??skip_ssl_verify=(?P<skip_ssl_verify>\\w*))?")
)

type (
	// Mailer is the struct that takes care of sending of emails.
	Mailer struct {
		staticDB *database.DB
	}

	// emailConfig contains all configuration options we need in order to send
	// an email
	emailConfig struct {
		User               string
		Pass               string
		Server             string
		Port               int
		InsecureSkipVerify bool
	}
)

// New creates a new instance of Mailer.
func New(db *database.DB) *Mailer {
	return &Mailer{db}
}

// Send queues an email message for sending. The message will be sent by Sender
// with the next batch of emails.
func (em Mailer) Send(ctx context.Context, m database.EmailMessage) error {
	return em.staticDB.EmailCreate(ctx, m)
}

// SendAddressConfirmationEmail sends a new email to the given email address
// with a link to confirm the ownership of the address.
func (em Mailer) SendAddressConfirmationEmail(ctx context.Context, email, token string) error {
	m, err := confirmEmailEmail(email, token)
	if err != nil {
		return errors.AddContext(err, "failed to generate email template")
	}
	return em.Send(ctx, *m)
}

// SendRecoverAccountEmail sends a new email to the given email address
// with a link to recover the account.
func (em Mailer) SendRecoverAccountEmail(ctx context.Context, email, token string) error {
	m, err := recoverAccountEmail(email, token)
	if err != nil {
		return errors.AddContext(err, "failed to generate email template")
	}
	return em.Send(ctx, *m)
}

// SendAccountAccessAttemptedEmail sends a new email to the given email address
// that notifies the user that someone used their email address in an attempt to
// recover a Skynet account but their email is not in our system. The main
// reason to do that is because the user might have forgotten which email they
// used for signing up.
func (em Mailer) SendAccountAccessAttemptedEmail(ctx context.Context, email string) error {
	m, err := accountAccessAttemptedEmail(email)
	if err != nil {
		return errors.AddContext(err, "failed to generate email template")
	}
	return em.Send(ctx, *m)
}

// send an email message.
//
// This function will not be called by Mailer but rather by Sender.
//
// bodyMime should be either "text/plain" or "text/html"
func send(from, to, subject, body, bodyMime string) error {
	m := mail.NewMessage()
	m.SetHeader("From", from)
	m.SetHeader("To", to)
	m.SetHeader("Subject", subject)
	m.SetBody(bodyMime, body)

	return sendMultiple(m)
}

// send one or more email messages.
//
// This function will not be called by Mailer but rather by Sender.
func sendMultiple(m ...*mail.Message) error {
	c, err := config(ConnectionURI)
	if err != nil {
		return errors.AddContext(err, "failed to parse email config")
	}
	d := mail.NewDialer(c.Server, c.Port, c.User, c.Pass)
	// This is only needed when SSL/TLS certificate is not valid on server.
	// In production this should be set to false.
	/* #nosec */
	d.TLSConfig = &tls.Config{
		InsecureSkipVerify: c.InsecureSkipVerify,
		ServerName:         c.Server,
	}
	return d.DialAndSend(m...)
}

// config parses the ConnectionURI variable and extracts the configuration
// values from it.
func config(connURI string) (emailConfig, error) {
	match := matchPattern.FindStringSubmatch(connURI)
	result := make(map[string]string)
	for i, name := range matchPattern.SubexpNames() {
		if i != 0 && name != "" {
			result[name] = match[i]
		}
	}
	server, e1 := result["server"]
	portStr, e2 := result["port"]
	user, e3 := result["user"]
	password, e4 := result["password"]

	// These fields are obligatory, so we return an error if any of them are
	// missing.
	if !(e1 && e2 && e3 && e4) {
		return emailConfig{}, errors.New("missing obligatory email configuration field. One of server, port, user, or password is missing")
	}
	user, err1 := url.QueryUnescape(user)
	password, err2 := url.QueryUnescape(password)
	port, err3 := strconv.Atoi(portStr)
	err := errors.Compose(err1, err2, err3)
	if err != nil {
		return emailConfig{}, errors.AddContext(err, "invalid value for username, password, or port in email connection string")
	}
	skip := result["skip_ssl_verify"]
	return emailConfig{
		User:               user,
		Pass:               password,
		Server:             server,
		Port:               port,
		InsecureSkipVerify: skip == "true",
	}, nil
}
