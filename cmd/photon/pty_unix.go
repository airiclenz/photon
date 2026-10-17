//go:build !windows

package main

// refuseWithoutPTY is nil here: a unix pty is there to record in.
func refuseWithoutPTY() error { return nil }
