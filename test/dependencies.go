package test

import "gitlab.com/SkynetLabs/skyd/skymodules"

// DependencySkipSendingEmails is a test dependency that causes the email sender
// not to send the emails and to directly return a success instead.
type DependencySkipSendingEmails struct {
	skymodules.SkynetDependencies
}

// Disrupt will check for a specific disrupt and respond accordingly.
func (d *DependencySkipSendingEmails) Disrupt(s string) bool {
	return s == "SkipSendingEmails"
}
