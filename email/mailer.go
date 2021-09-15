package email

import (
	"context"

	"github.com/NebulousLabs/skynet-accounts/database"

	"gitlab.com/NebulousLabs/errors"
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

// Mailer prepares messages for sending by adding them to the email queue.
type Mailer struct {
	staticDB *database.DB
}

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
