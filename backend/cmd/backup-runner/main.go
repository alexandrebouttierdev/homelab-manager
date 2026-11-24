// Command homelab-backup-runner runs one backup operation for
// homelab-backup@<operation>.service (SPEC_ADMINISTRATION.md § 7.1). It
// takes the operation as its only argument, the instance name of the unit;
// everything else comes from the job file, validated again.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"homelab-manager/backend/internal/backuprunner"
)

// version is set at build time by scripts/release/build.sh.
var version = "dev"

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "homelab-backup-runner %s\nusage: homelab-backup-runner <operation>\n", version)
		os.Exit(2)
	}
	// systemd stops the unit with SIGTERM at TimeoutStartSec=: restic is
	// stopped with the context and the failure is still reported.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	status := backuprunner.Run(ctx, backuprunner.Defaults(), os.Args[1], os.Stderr)
	stop()
	os.Exit(status)
}
