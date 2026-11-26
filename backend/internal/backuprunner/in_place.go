package backuprunner

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"monoops/backend/internal/backupjob"
	"monoops/backend/internal/domain"
)

// safetyTag marks the backup an in-place restore makes of what it
// replaces (decision D-2).
const safetyTag = "before-restore"

// archiveMode lets the Manager's group read an archive to serve it.
const archiveMode = 0o640

var errArchiveTooLarge = fmt.Errorf("the archive exceeds %d GiB", domain.MaxBackupArchiveBytes>>30)

// restoreInPlace restores paths of a snapshot where they were (D-2). A
// simulation only reports what would change. A real restore first backs up
// the paths that exist now, tagged before-restore, and stops if that
// backup fails: what it overwrites can always be restored again. Files
// that are the same are left alone (--overwrite if-changed); files absent
// from the snapshot are never deleted.
func restoreInPlace(ctx context.Context, config Config, env []string, job backupjob.Job, result *backupjob.Result) error {
	request := job.Restore
	result.Details = &domain.BackupRunDetails{SnapshotID: request.SnapshotID, Target: "/", DryRun: request.DryRun, InPlace: true}
	if !request.DryRun {
		var existing []string
		for _, include := range request.Include {
			if _, err := os.Lstat(filepath.Join(config.InPlaceRoot, include)); err == nil {
				existing = append(existing, filepath.Join(config.InPlaceRoot, include))
			}
		}
		if len(existing) > 0 {
			args := append([]string{"backup", "--json", "--tag", domain.BackupTag, "--tag", safetyTag, "--"}, existing...)
			summary, output, err := runStreaming(ctx, config, env, args)
			if err != nil || summary == nil || summary.SnapshotID == "" {
				result.Message = domain.TruncateBackupMessage("the safety backup failed, nothing was restored: " + output.message())
				if err == nil {
					err = errors.New("the safety backup gave no snapshot")
				}
				return err
			}
			result.Details.SafetySnapshotID = summary.SnapshotID
		}
	}
	args := []string{"restore", "--json", "--target", config.InPlaceRoot, "--overwrite", "if-changed"}
	for _, include := range request.Include {
		args = append(args, "--include", include)
	}
	if request.DryRun {
		args = append(args, "--dry-run")
	}
	args = append(args, "--", request.SnapshotID)
	summary, output, err := runStreaming(ctx, config, env, args)
	if summary != nil {
		result.Details.TotalFiles, result.Details.FilesRestored = summary.TotalFiles, summary.FilesRestored
		result.Details.TotalBytes, result.Details.BytesRestored = summary.TotalBytes, summary.BytesRestored
	}
	if err != nil {
		result.Message = output.message()
		return err
	}
	result.Success = true
	if result.Details.SafetySnapshotID != "" {
		// The safety snapshot must be listed to restore it, which undoes the
		// restore; a listing failure does not undo the success.
		if snapshots, err := listSnapshots(ctx, config, env); err == nil {
			result.Snapshots = snapshots
		} else {
			result.Message = domain.TruncateBackupMessage("snapshot list unavailable: " + err.Error())
		}
	}
	return nil
}

// dumpArchive writes one path of a snapshot as a .tar.gz archive in the
// archives directory, for the Manager to serve (D-2). Only the archive of
// this run is kept; a larger one than domain.MaxBackupArchiveBytes fails
// and is removed.
func dumpArchive(ctx context.Context, config Config, env []string, job backupjob.Job, result *backupjob.Result) error {
	request := job.Dump
	result.Details = &domain.BackupRunDetails{SnapshotID: request.SnapshotID, ArchivePath: request.Path}
	if err := clearDumps(config.ArchiveRoot); err != nil {
		return fmt.Errorf("clear the archives: %w", err)
	}
	target := filepath.Join(config.ArchiveRoot, filepath.Base(request.Target))
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, archiveMode)
	if err != nil {
		return fmt.Errorf("create the archive: %w", err)
	}
	keep := false
	defer func() {
		_ = file.Close()
		if !keep {
			_ = os.Remove(target)
		}
	}()
	limited := &limitedWriter{writer: file, room: domain.MaxBackupArchiveBytes}
	compressed := gzip.NewWriter(limited)
	cmd := resticCommand(ctx, config, "dump", "--archive", "tar", "--", request.SnapshotID, request.Path)
	cmd.Env = env
	stderr := &boundedBuffer{limit: maxStderrBytes}
	cmd.Stdout, cmd.Stderr = compressed, stderr
	runErr := cmd.Run()
	closeErr := compressed.Close()
	output := resticOutput{}
	output.absorbAll(stderr.Bytes())
	switch {
	case limited.exceeded:
		result.Message = domain.TruncateBackupMessage(errArchiveTooLarge.Error() + ": choose a smaller path")
		return errArchiveTooLarge
	case runErr != nil:
		result.Message = output.message()
		return runErr
	case closeErr != nil:
		return fmt.Errorf("write the archive: %w", closeErr)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("write the archive: %w", err)
	}
	if err := os.Chmod(target, archiveMode); err != nil {
		return fmt.Errorf("protect the archive: %w", err)
	}
	result.Details.ArchiveBytes = domain.MaxBackupArchiveBytes - limited.room
	keep, result.Success = true, true
	return nil
}

// limitedWriter refuses to write more than room bytes.
type limitedWriter struct {
	writer   io.Writer
	room     int64
	exceeded bool
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.room {
		w.exceeded = true
		return 0, errArchiveTooLarge
	}
	n, err := w.writer.Write(p)
	w.room -= int64(n)
	return n, err
}
