package email

import (
	"context"
	"crypto/tls"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/NebulousLabs/skynet-accounts/build"
	"github.com/NebulousLabs/skynet-accounts/database"
	"github.com/sirupsen/logrus"
	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/SkynetLabs/skyd/skymodules"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"gopkg.in/mail.v2"
)

const (
	// batchSize defines the largest batch of emails we will try to send.
	batchSize = 10
)

var (
	// From is the address we send emails from. It defaults to the user
	// from DefaultConnectionURI but can be overridden by the ACCOUNTS_EMAIL_FROM
	// environment variable.
	From = "noreply@siasky.net"

	// PortalAddress defines the URI where we can access our portal. Its value
	// comes from the PORTAL_DOMAIN environment variable, preceded by the
	// appropriate schema.
	PortalAddress = "https://siasky.net"

	// ServerLockID holds the name of the name of this particular server. Its
	// value is controlled by the SERVER_DOMAIN entry in the .env file. If the
	// SERVER_DOMAIN entry is empty or missing, the PORTAL_DOMAIN (preceded by
	// schema) will be used instead. The only exception is testing where there's
	// nothing to set it, so we want to always have it set.
	ServerLockID = build.Select(
		build.Var{
			Dev:      "",
			Testing:  "siasky.test",
			Standard: "",
		},
	).(string)

	// matchPattern extracts all relevant configuration values from an email
	// connection URI
	matchPattern = regexp.MustCompile("smtps://(?P<user>.*):(?P<password>.*)@(?P<server>.*):(?P<port>\\d*)(/\\??skip_ssl_verify=(?P<skip_ssl_verify>\\w*))?")

	// sleepBetweenScans defines how long the sender should sleep between its
	// sweeps of the DB.
	sleepBetweenScans = build.Select(
		build.Var{
			Dev:      time.Second,
			Testing:  100 * time.Millisecond,
			Standard: 3 * time.Second,
		},
	).(time.Duration)
)

type (
	// Sender is a daemon that periodically checks the DB for emails waiting to
	// be sent and sends them.
	Sender struct {
		staticConfig emailConfig
		staticCtx    context.Context
		staticDB     *database.DB
		staticDeps   skymodules.SkydDependencies
		staticLogger *logrus.Logger
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

// NewSender returns a new Sender instance.
func NewSender(ctx context.Context, db *database.DB, logger *logrus.Logger, deps skymodules.SkydDependencies, emailConnURI string) (Sender, error) {
	c, err := config(emailConnURI)
	if err != nil {
		return Sender{}, errors.AddContext(err, "failed to parse email config")
	}
	return Sender{
		staticConfig: c,
		staticCtx:    ctx,
		staticDB:     db,
		staticDeps:   deps,
		staticLogger: logger,
	}, nil
}

// Start periodically scanning the database for email messages waiting to be
// sent and sending them.
func (s Sender) Start() {
	go func() {
		s.ScanAndSend(ServerLockID)
		for {
			select {
			case <-s.staticCtx.Done():
				return
			case <-time.After(sleepBetweenScans):
				s.ScanAndSend(ServerLockID)
			}
		}
	}()
}

// ScanAndSend scans the database for email messages waiting to be sent and
// sends them.
//
// We lock the messages before sending them and update their SentAt field after
// sending them. We also don't lock more than batchSize messages.
func (s Sender) ScanAndSend(lockID string) (int, int) {
	msgs, err := s.staticDB.EmailLockAndFetch(s.staticCtx, lockID, batchSize)
	if err != nil {
		s.staticLogger.Warningln(errors.AddContext(err, "failed to send email batch"))
		return 0, 0
	}
	if len(msgs) == 0 {
		return 0, 0
	}
	var sent []primitive.ObjectID
	var failed []*database.EmailMessage
	var errs []error
	for i, m := range msgs {
		err = s.send(m.From, m.To, m.Subject, m.Body, m.BodyMime)
		if err != nil {
			errs = append(errs, err)
			failed = append(failed, &msgs[i])
			continue
		}
		sent = append(sent, m.ID)
	}
	if len(errs) > 0 {
		err = errors.Compose(errs...)
		err = errors.AddContext(err, "failed to send some emails")
		s.staticLogger.Warningln(err)
	}

	err = s.staticDB.MarkAsSent(s.staticCtx, sent)
	if err != nil {
		err = errors.AddContext(err, "failed to mark emails as sent. they might get sent again")
		s.staticLogger.Warningln(err)
	}

	err = s.staticDB.MarkAsFailed(s.staticCtx, failed)
	if err != nil {
		err = errors.AddContext(err, "failed to mark emails as failed. we might attempt to send them one extra time")
		s.staticLogger.Debugln(err)
	}
	return len(sent), len(failed)
}

// send an email message.
//
// This function will not be called by Mailer but rather by Sender.
//
// bodyMime should be either "text/plain" or "text/html"
func (s Sender) send(from, to, subject, body, bodyMime string) error {
	m := mail.NewMessage()
	m.SetHeader("From", from)
	m.SetHeader("To", to)
	m.SetHeader("Subject", subject)
	m.SetBody(bodyMime, body)

	return s.sendMultiple(m)
}

// sendMultiple one or more email messages.
//
// This function will not be called by Mailer but rather by Sender.
func (s Sender) sendMultiple(m ...*mail.Message) error {
	d := mail.NewDialer(s.staticConfig.Server, s.staticConfig.Port, s.staticConfig.User, s.staticConfig.Pass)
	// This is only needed when SSL/TLS certificate is not valid on server.
	// In production this should be set to false.
	/* #nosec */
	d.TLSConfig = &tls.Config{
		InsecureSkipVerify: s.staticConfig.InsecureSkipVerify,
		ServerName:         s.staticConfig.Server,
	}
	if s.staticDeps.Disrupt("SkipSendingEmails") {
		return nil
	}
	return d.DialAndSend(m...)
}

// config parses the DefaultConnectionURI variable and extracts the configuration
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
