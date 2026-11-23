package domain

import (
	"errors"
	"strings"
)

// PowerAction is a request to reboot or power off the machine
// (SPEC_ADMINISTRATION.md § 8 bis).
type PowerAction string

const (
	PowerReboot   PowerAction = "reboot"
	PowerPowerOff PowerAction = "poweroff"
)

// MaxHostnameBytes bounds the typed confirmation (longest DNS host name).
const MaxHostnameBytes = 253

var (
	ErrInvalidPowerAction   = errors.New("power action must be reboot or poweroff")
	ErrHostnameMismatch     = errors.New("the confirmation does not match the host name")
	ErrPowerActionsDisabled = errors.New("power actions are disabled in the service configuration")
	ErrActionInProgress     = errors.New("an action is already in progress")
	ErrPowerActionRefused   = errors.New("the system refused the power action")
)

func (a PowerAction) Valid() bool {
	return a == PowerReboot || a == PowerPowerOff
}

// ConfirmsHostname reports whether the confirmation typed by the user names
// the machine exactly; only surrounding spaces are ignored.
func ConfirmsHostname(confirmation, hostname string) bool {
	return hostname != "" && strings.TrimSpace(confirmation) == hostname
}
