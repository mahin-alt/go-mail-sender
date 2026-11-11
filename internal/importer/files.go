// Package importer turns an uploaded .xlsx into email_recipients rows in the
// background.
//
// The upload request only streams the file to disk and creates the batch; it
// answers "202 Accepted" straight away. The import Worker then reads the file
// row by row, checks every row (format and, optionally, the domain), and
// saves the rows in chunks. Each chunk is saved together with a checkpoint
// ("rows up to N are saved"), so if the server stops halfway, the next run
// continues from the checkpoint instead of starting over or losing rows.
package importer

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ErrNotXLSX: the upload does not start like an .xlsx (a ZIP) file.
var ErrNotXLSX = errors.New("the file is not a valid .xlsx spreadsheet")

// ErrTooLarge: the upload is bigger than the size limit.
var ErrTooLarge = errors.New("file too large")

// zipMagic is how every ZIP file, and so every .xlsx, begins.
var zipMagic = []byte("PK\x03\x04")

// Files keeps uploaded spreadsheets in one folder on disk. A batch's file is
// <dir>/batch-<id>.xlsx; the name comes from the ID, never from the user's
// filename. Files are kept after the import (for now; see the ROADMAP item
// "Temporary upload files").
type Files struct {
	Dir string
}

// Path is where a batch's spreadsheet lives.
func (f Files) Path(batchID int64) string {
	return filepath.Join(f.Dir, "batch-"+strconv.FormatInt(batchID, 10)+".xlsx")
}

const incomingPrefix = ".incoming-"

// Receive streams r into a new temporary file in the folder, without ever
// holding the whole file in memory, and returns its path. It stops with
// ErrTooLarge after limit bytes and with ErrNotXLSX if the content is not a
// ZIP file (the extension and the browser's content type are not trusted).
// On any error the temporary file is removed.
func (f Files) Receive(r io.Reader, limit int64) (string, error) {
	if err := os.MkdirAll(f.Dir, 0o750); err != nil {
		return "", fmt.Errorf("create upload folder: %w", err)
	}
	tmp, err := os.CreateTemp(f.Dir, incomingPrefix+"*.xlsx")
	if err != nil {
		return "", fmt.Errorf("create upload file: %w", err)
	}
	ok := false
	defer func() {
		if !ok {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()

	head := make([]byte, len(zipMagic))
	n, err := io.ReadFull(r, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read upload: %w", err)
	}
	if !bytes.Equal(head[:n], zipMagic) {
		return "", ErrNotXLSX
	}
	if _, err := tmp.Write(head); err != nil {
		return "", fmt.Errorf("write upload: %w", err)
	}
	// Copy the rest in small buffers; read one byte past the limit to notice
	// a file that is too big.
	written, err := io.Copy(tmp, io.LimitReader(r, limit-int64(n)+1))
	if err != nil {
		return "", fmt.Errorf("write upload: %w", err)
	}
	if int64(n)+written > limit {
		return "", ErrTooLarge
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("save upload: %w", err)
	}
	ok = true
	return tmp.Name(), nil
}

// Promote gives a received file its final name, batch-<id>.xlsx.
func (f Files) Promote(tmpPath string, batchID int64) error {
	if err := os.Rename(tmpPath, f.Path(batchID)); err != nil {
		return fmt.Errorf("move upload into place: %w", err)
	}
	return nil
}

// BatchChecker tells whether a batch still exists.
type BatchChecker interface {
	BatchExists(ctx context.Context, id int64) (bool, error)
}

// CleanupOrphans removes files that no batch will ever read: temporary
// uploads that were never finished (the request died) and batch files whose
// batch row does not exist (the database insert failed after the file was
// moved, or the batch was deleted). Only files older than minAge are touched,
// so an upload in progress is never removed. Returns how many were deleted.
func (f Files) CleanupOrphans(ctx context.Context, batches BatchChecker, minAge time.Duration) (int, error) {
	entries, err := os.ReadDir(f.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil || time.Since(info.ModTime()) < minAge {
			continue
		}
		name := e.Name()
		orphan := strings.HasPrefix(name, incomingPrefix)
		if idStr, ok := strings.CutPrefix(name, "batch-"); ok && !orphan {
			id, err := strconv.ParseInt(strings.TrimSuffix(idStr, ".xlsx"), 10, 64)
			if err != nil {
				continue
			}
			exists, err := batches.BatchExists(ctx, id)
			if err != nil {
				return removed, err
			}
			orphan = !exists
		}
		if !orphan {
			continue
		}
		if err := os.Remove(filepath.Join(f.Dir, name)); err != nil {
			slog.Warn("could not remove orphan upload", "file", name, "err", err)
			continue
		}
		removed++
	}
	return removed, nil
}

// randomID is a short random hex string (worker names).
func randomID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}
