package transfer

import (
	"context"
	"io"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/lfsx"
)

// Store is the data layer for LFS SSH transfer operations.
type Store interface {
	CreateLFSObject(ctx context.Context, repoID int64, oid lfsx.OID, size int64, storage lfsx.Storage) error
	GetLFSObjectByOID(ctx context.Context, repoID int64, oid lfsx.OID) (*database.LFSObject, error)
	GetLFSObjectsByOIDs(ctx context.Context, repoID int64, oids ...lfsx.OID) ([]*database.LFSObject, error)
}

type dbStore struct{}

// NewStore returns a new Store backed by the global database handle.
func NewStore() Store {
	return &dbStore{}
}

func (*dbStore) CreateLFSObject(ctx context.Context, repoID int64, oid lfsx.OID, size int64, storage lfsx.Storage) error {
	return database.Handle.LFS().CreateObject(ctx, repoID, oid, size, storage)
}

func (*dbStore) GetLFSObjectByOID(ctx context.Context, repoID int64, oid lfsx.OID) (*database.LFSObject, error) {
	return database.Handle.LFS().GetObjectByOID(ctx, repoID, oid)
}

func (*dbStore) GetLFSObjectsByOIDs(ctx context.Context, repoID int64, oids ...lfsx.OID) ([]*database.LFSObject, error) {
	return database.Handle.LFS().GetObjectsByOIDs(ctx, repoID, oids...)
}

// handler manages a single LFS SSH transfer session.
type handler struct {
	scanner        *PktlineScanner
	writer         *PktlineWriter
	operation      string
	repo           *database.Repository
	store          Store
	defaultStorage lfsx.Storage
	storagers      map[lfsx.Storage]lfsx.Storager
}

// Serve runs the LFS SSH transfer protocol over the given reader and writer.
// It performs capability advertisement, version negotiation, and enters the
// command loop. It blocks until the client sends "quit" or the connection
// closes.
func Serve(
	ctx context.Context,
	r io.Reader,
	w io.Writer,
	operation string,
	repo *database.Repository,
	store Store,
	defaultStorage lfsx.Storage,
	storagers map[lfsx.Storage]lfsx.Storager,
) error {
	h := &handler{
		scanner:        NewPktlineScanner(r),
		writer:         NewPktlineWriter(w),
		operation:      operation,
		repo:           repo,
		store:          store,
		defaultStorage: defaultStorage,
		storagers:      storagers,
	}
	return h.serve(ctx)
}

func (h *handler) serve(ctx context.Context) error {
	// Advertise capabilities.
	if err := h.writer.WritePacketText("version=1"); err != nil {
		return errors.Wrap(err, "advertise version")
	}
	if err := h.writer.WriteFlush(); err != nil {
		return errors.Wrap(err, "flush after version advertisement")
	}

	// Read client version.
	if !h.scanner.Scan() {
		if err := h.scanner.Err(); err != nil {
			return errors.Wrap(err, "read client version")
		}
		return errors.New("unexpected EOF reading client version")
	}
	clientVersion := h.scanner.Text()
	// Consume remaining lines until flush.
	for h.scanner.Scan() && !h.scanner.IsFlush() {
	}
	if err := h.scanner.Err(); err != nil {
		return errors.Wrap(err, "read client version capabilities")
	}

	if clientVersion != "version 1" {
		if err := h.writeStatus(400); err != nil {
			return err
		}
		return errors.Errorf("unsupported client version: %q", clientVersion)
	}

	// Acknowledge version.
	if err := h.writeStatus(200); err != nil {
		return err
	}

	// Command loop.
	for {
		if !h.scanner.Scan() {
			if err := h.scanner.Err(); err != nil {
				return errors.Wrap(err, "read command")
			}
			return nil // Clean EOF.
		}

		if h.scanner.IsFlush() {
			continue
		}

		line := h.scanner.Text()
		var err error
		switch {
		case line == "quit":
			h.consumeUntilFlush()
			return h.writeStatus(200)

		case line == "batch":
			err = h.handleBatch(ctx)

		case strings.HasPrefix(line, "put-object "):
			oid := lfsx.OID(strings.TrimPrefix(line, "put-object "))
			err = h.handlePutObject(ctx, oid)

		case strings.HasPrefix(line, "get-object "):
			oid := lfsx.OID(strings.TrimPrefix(line, "get-object "))
			err = h.handleGetObject(ctx, oid)

		case strings.HasPrefix(line, "verify-object "):
			oid := lfsx.OID(strings.TrimPrefix(line, "verify-object "))
			err = h.handleVerifyObject(ctx, oid)

		default:
			h.consumeUntilFlush()
			err = h.writeStatusWithMessage(400, "unknown command")
		}
		if err != nil {
			return err
		}
	}
}

func (h *handler) handleBatch(ctx context.Context) error {
	type oidSize struct {
		oid  lfsx.OID
		size int64
	}

	var items []oidSize
	for h.scanner.Scan() {
		if h.scanner.IsFlush() {
			break
		}
		line := h.scanner.Text()

		// Skip known arguments like hash-algo, refname, transfer.
		if strings.Contains(line, "=") {
			continue
		}

		// Parse "<oid> <size>".
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}
		size, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}
		items = append(items, oidSize{oid: lfsx.OID(parts[0]), size: size})
	}

	// Look up existing objects.
	oids := make([]lfsx.OID, 0, len(items))
	for _, item := range items {
		oids = append(oids, item.oid)
	}
	existing, err := h.store.GetLFSObjectsByOIDs(ctx, h.repo.ID, oids...)
	if err != nil {
		return h.writeStatusWithMessage(500, "internal error")
	}
	existingSet := make(map[lfsx.OID]*database.LFSObject, len(existing))
	for _, obj := range existing {
		existingSet[obj.OID] = obj
	}

	// Write response.
	if err := h.writer.WritePacketText("status 200"); err != nil {
		return err
	}
	if err := h.writer.WriteFlush(); err != nil {
		return err
	}

	for _, item := range items {
		obj, exists := existingSet[item.oid]
		sizeStr := strconv.FormatInt(item.size, 10)

		if h.operation == "upload" {
			if exists {
				if err := h.writer.WritePacketText(string(item.oid) + " " + sizeStr + " noop"); err != nil {
					return err
				}
			} else {
				if err := h.writer.WritePacketText(string(item.oid) + " " + sizeStr); err != nil {
					return err
				}
			}
		} else {
			if exists {
				actualSize := strconv.FormatInt(obj.Size, 10)
				if err := h.writer.WritePacketText(string(item.oid) + " " + actualSize); err != nil {
					return err
				}
			} else {
				if err := h.writer.WritePacketText(string(item.oid) + " " + sizeStr + " noop"); err != nil {
					return err
				}
			}
		}
	}
	return h.writer.WriteFlush()
}

func (h *handler) handlePutObject(ctx context.Context, oid lfsx.OID) error {
	if h.operation != "upload" {
		h.consumeUntilFlush()
		return h.writeStatusWithMessage(403, "not allowed for download operation")
	}

	if !lfsx.ValidOID(oid) {
		h.consumeUntilFlush()
		return h.writeStatusWithMessage(400, "invalid oid")
	}

	// Read arguments until delim, then binary data until flush.
	var expectedSize int64
	for h.scanner.Scan() {
		if h.scanner.IsDelim() {
			break
		}
		if h.scanner.IsFlush() {
			return h.writeStatusWithMessage(400, "expected delimiter before object data")
		}
		line := h.scanner.Text()
		if strings.HasPrefix(line, "size=") {
			v, err := strconv.ParseInt(strings.TrimPrefix(line, "size="), 10, 64)
			if err != nil || v < 0 {
				// Consume remaining input so the protocol stays in sync.
				for h.scanner.Scan() && !h.scanner.IsFlush() {
				}
				return h.writeStatusWithMessage(400, "invalid size")
			}
			expectedSize = v
		}
	}

	// Read binary data from pkt-line packets until flush.
	dataReader := newPktlineDataReader(h.scanner)

	s := h.storagers[h.defaultStorage]
	if s == nil {
		_, _ = io.Copy(io.Discard, dataReader)
		return h.writeStatusWithMessage(500, "storage backend not configured")
	}

	written, err := s.Upload(oid, io.NopCloser(dataReader))
	if err != nil {
		// Drain any remaining data so the protocol stays in sync.
		_, _ = io.Copy(io.Discard, dataReader)
		if errors.Is(err, lfsx.ErrOIDMismatch) || errors.Is(err, lfsx.ErrInvalidOID) {
			return h.writeStatusWithMessage(400, err.Error())
		}
		log.Error("Failed to upload LFS object via SSH [oid: %s]: %v", oid, err)
		return h.writeStatusWithMessage(500, "upload failed")
	}

	if expectedSize > 0 && written != expectedSize {
		return h.writeStatusWithMessage(400, "size mismatch")
	}

	// Create the database record. If the record already exists (e.g. from a
	// concurrent upload), verify it with a follow-up query instead of failing.
	err = h.store.CreateLFSObject(ctx, h.repo.ID, oid, written, h.defaultStorage)
	if err != nil {
		if _, lookupErr := h.store.GetLFSObjectByOID(ctx, h.repo.ID, oid); lookupErr != nil {
			log.Error("Failed to create LFS object record [repo_id: %d, oid: %s]: %v", h.repo.ID, oid, err)
			return h.writeStatusWithMessage(500, "failed to create object record")
		}
		log.Trace("[LFS SSH] Object already exists %q", oid)
	} else {
		log.Trace("[LFS SSH] Object created %q", oid)
	}
	return h.writeStatus(200)
}

func (h *handler) handleGetObject(ctx context.Context, oid lfsx.OID) error {
	// Read remaining arguments until flush.
	h.consumeUntilFlush()

	if !lfsx.ValidOID(oid) {
		return h.writeStatusWithMessage(400, "invalid oid")
	}

	object, err := h.store.GetLFSObjectByOID(ctx, h.repo.ID, oid)
	if err != nil {
		if database.IsErrLFSObjectNotExist(err) {
			return h.writeStatusWithMessage(404, "object does not exist")
		}
		log.Error("Failed to get LFS object [repo_id: %d, oid: %s]: %v", h.repo.ID, oid, err)
		return h.writeStatusWithMessage(500, "internal error")
	}

	s := h.storagers[object.Storage]
	if s == nil {
		log.Error("Storage backend %q not found for LFS object %s", object.Storage, oid)
		return h.writeStatusWithMessage(500, "storage backend not found")
	}

	// Respond with status, size, delim, then stream binary data.
	if err := h.writer.WritePacketText("status 200"); err != nil {
		return err
	}
	if err := h.writer.WritePacketText("size=" + strconv.FormatInt(object.Size, 10)); err != nil {
		return err
	}
	if err := h.writer.WriteDelim(); err != nil {
		return err
	}

	// Use a pipe to stream from Storager.Download into pkt-line framed output.
	pr, pw := io.Pipe()
	downloadErr := make(chan error, 1)
	go func() {
		err := s.Download(object.OID, pw)
		pw.CloseWithError(err)
		downloadErr <- err
	}()

	if err := h.writer.WriteData(pr); err != nil {
		pr.Close()
		return errors.Wrap(err, "write object data")
	}
	pr.Close()

	if err := <-downloadErr; err != nil {
		return errors.Wrap(err, "download object from storage")
	}

	return h.writer.WriteFlush()
}

func (h *handler) handleVerifyObject(ctx context.Context, oid lfsx.OID) error {
	var expectedSize int64
	var sizeErr bool
	for h.scanner.Scan() {
		if h.scanner.IsFlush() {
			break
		}
		line := h.scanner.Text()
		if strings.HasPrefix(line, "size=") {
			v, err := strconv.ParseInt(strings.TrimPrefix(line, "size="), 10, 64)
			if err != nil || v < 0 {
				sizeErr = true
			} else {
				expectedSize = v
			}
		}
	}

	if sizeErr {
		return h.writeStatusWithMessage(400, "invalid size")
	}

	if !lfsx.ValidOID(oid) {
		return h.writeStatusWithMessage(400, "invalid oid")
	}

	object, err := h.store.GetLFSObjectByOID(ctx, h.repo.ID, oid)
	if err != nil {
		if database.IsErrLFSObjectNotExist(err) {
			return h.writeStatusWithMessage(404, "object does not exist")
		}
		log.Error("Failed to get LFS object [repo_id: %d, oid: %s]: %v", h.repo.ID, oid, err)
		return h.writeStatusWithMessage(500, "internal error")
	}

	if object.Size != expectedSize {
		return h.writeStatusWithMessage(409, "size mismatch")
	}

	return h.writeStatus(200)
}

func (h *handler) writeStatus(code int) error {
	if err := h.writer.WritePacketText("status " + strconv.Itoa(code)); err != nil {
		return errors.Wrap(err, "write status")
	}
	return h.writer.WriteFlush()
}

func (h *handler) writeStatusWithMessage(code int, message string) error {
	if err := h.writer.WritePacketText("status " + strconv.Itoa(code)); err != nil {
		return errors.Wrap(err, "write status")
	}
	if message != "" {
		if err := h.writer.WritePacketText(message); err != nil {
			return errors.Wrap(err, "write status message")
		}
	}
	return h.writer.WriteFlush()
}

func (h *handler) consumeUntilFlush() {
	for h.scanner.Scan() {
		if h.scanner.IsFlush() {
			return
		}
	}
}
