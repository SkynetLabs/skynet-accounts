package email

import (
	"crypto/tls"
	"fmt"
	"net/url"
	"regexp"
	"strconv"

	"gitlab.com/NebulousLabs/errors"
	"gopkg.in/mail.v2"
)

var (
	// ConnectionURI is the connection string used for sending emails.
	ConnectionURI = "smtps://test:test@mailslurper:1025/?skip_ssl_verify=true"

	// matchPattern extracts all relevant configuration values from an email
	// connection URI
	matchPattern = regexp.MustCompile("smtps://(?P<user>.*):(?P<password>.*)@(?P<server>.*):(?P<port>\\d*)(/\\??skip_ssl_verify=(?P<skip_ssl_verify>\\w*))?")
)

// emailConfig contains all configuration options we need in order to send
// an email
type emailConfig struct {
	User               string
	Pass               string
	Server             string
	Port               int
	InsecureSkipVerify bool
}

// Send an email message.
//
// bodyMime should be either "text/plain" or "text/html"
func Send(from, to, subject, body, bodyMime string) error {
	m := mail.NewMessage()
	m.SetHeader("From", from)
	m.SetHeader("To", to)
	m.SetHeader("Subject", subject)
	m.SetBody(bodyMime, body)

	return SendMultiple(m)
}

// Send one or more email messages.
func SendMultiple(m ...*mail.Message) error {
	c, err := config(ConnectionURI)
	if err != nil {
		return errors.AddContext(err, "failed to parse email config")
	}
	d := mail.NewDialer(c.Server, c.Port, c.User, c.Pass)
	fmt.Println(c)
	fmt.Println(d.TLSConfig)
	// This is only needed when SSL/TLS certificate is not valid on server.
	// In production this should be set to false.
	d.TLSConfig = &tls.Config{
		InsecureSkipVerify: c.InsecureSkipVerify,
		ServerName:         c.Server,
	}
	// Send!
	return d.DialAndSend(m...)
}

// config parses the ConnectionURI variable and extracts the configuration
// values from it.
func config(connUri string) (emailConfig, error) {
	match := matchPattern.FindStringSubmatch(connUri)
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
