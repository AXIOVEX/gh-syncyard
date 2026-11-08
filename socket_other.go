//go:build !unix

package main

import "errors"

// The daemon socket channel is unix-only in this build — the
// same documented limitation as the tray's GatekeeperChannel
// (syncyard-tray control.rs). On Windows hosts the control
// surface reaches the daemon through the tray app instead.
func socketCallUnix(path string, payload []byte) (string, error) {
	return "", errors.New("daemon socket channel is unix-only in this build")
}
