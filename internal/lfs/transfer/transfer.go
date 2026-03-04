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

// dbStore implements Store using the global database handle.
type dbStore struct{}

// NewStore returns a new Store backed by the global database handle.
func NewStore() Store {
	return &dbStore{}
}

// CreateLFSObject inserts object metadata into the LFS table.
func (*dbStore) CreateLFSObject(ctx context.Context, repoID int64, oid lfsx.OID, size int64, storage lfsx.Storage) error {
	return database.Handle.LFS().CreateObject(ctx, repoID, oid, size, storage)
}

// GetLFSObjectByOID loads one LFS object by repository and object ID.
func (*dbStore) GetLFSObjectByOID(ctx context.Context, repoID int64, oid lfsx.OID) (*database.LFSObject, error) {
	return database.Handle.LFS().GetObjectByOID(ctx, repoID, oid)
}

// GetLFSObjectsByOIDs loads all matching LFS objects by repository and object IDs.
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

const supportedHashAlgorithm = "sha256"

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

// serve performs version negotiation and handles commands until the session ends.
func (h *handler) serve(ctx context.Context) error {
	if err := h.advertiseVersion(); err != nil {
		return err
	}

	clientVersion, err := h.readClientVersion()
	if err != nil {
		return err
	}
	if clientVersion != "version 1" {
		if err := h.writeStatus(400); err != nil {
			return err
		}
		return errors.Errorf("unsupported client version: %q", clientVersion)
	}
	if err := h.writeStatus(200); err != nil {
		return err
	}

	return h.commandLoop(ctx)
}

// advertiseVersion sends the server capability advertisement to the client.
func (h *handler) advertiseVersion() error {
	if err := h.writer.WritePacketText("version=1"); err != nil {
		return errors.Wrap(err, "advertise version")
	}
	if err := h.writer.WriteFlush(); err != nil {
		return errors.Wrap(err, "flush after version advertisement")
	}
	return nil
}

// readClientVersion reads the version line and consumes optional capability lines.
func (h *handler) readClientVersion() (string, error) {
	if !h.scanner.Scan() {
		if err := h.scanner.Err(); err != nil {
			return "", errors.Wrap(err, "read client version")
		}
		return "", errors.New("unexpected EOF reading client version")
	}
	clientVersion := h.scanner.Text()

	for h.scanner.Scan() && !h.scanner.IsFlush() {
	}
	if err := h.scanner.Err(); err != nil {
		return "", errors.Wrap(err, "read client version capabilities")
	}
	return clientVersion, nil
}

// commandLoop reads and dispatches commands until clean EOF or a quit command.
func (h *handler) commandLoop(ctx context.Context) error {
	for {
		line, ok, err := h.nextCommand()
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}

		quit, err := h.handleCommand(ctx, line)
		if err != nil {
			return err
		}
		if quit {
			return nil
		}
	}
}

// nextCommand reads the next non-flush command packet.
func (h *handler) nextCommand() (line string, ok bool, _ error) {
	for {
		if !h.scanner.Scan() {
			if err := h.scanner.Err(); err != nil {
				return "", false, errors.Wrap(err, "read command")
			}
			return "", false, nil
		}
		if h.scanner.IsFlush() {
			continue
		}
		return h.scanner.Text(), true, nil
	}
}

// handleCommand dispatches a single command line and reports whether to quit.
func (h *handler) handleCommand(ctx context.Context, line string) (bool, error) {
	switch {
	case line == "quit":
		h.consumeUntilFlush()
		return true, h.writeStatus(200)
	case line == "batch":
		return false, h.handleBatch(ctx)
	case strings.HasPrefix(line, "put-object "):
		oid := lfsx.OID(strings.TrimPrefix(line, "put-object "))
		return false, h.handlePutObject(ctx, oid)
	case strings.HasPrefix(line, "get-object "):
		oid := lfsx.OID(strings.TrimPrefix(line, "get-object "))
		return false, h.handleGetObject(ctx, oid)
	case strings.HasPrefix(line, "verify-object "):
		oid := lfsx.OID(strings.TrimPrefix(line, "verify-object "))
		return false, h.handleVerifyObject(ctx, oid)
	default:
		h.consumeUntilFlush()
		return false, h.writeStatusWithMessage(400, "unknown command")
	}
}

// batchItem is one object entry from a batch request payload.
type batchItem struct {
	oid  lfsx.OID
	size int64
}

// handleBatch handles the LFS "batch" command for upload and download operations.
func (h *handler) handleBatch(ctx context.Context) error {
	items, requestedHashAlgorithm, err := h.readBatchRequest()
	if err != nil {
		return err
	}

	existingSet, err := h.lookupBatchObjects(ctx, items)
	if err != nil {
		return err
	}

	return h.writeBatchResponse(items, existingSet, requestedHashAlgorithm)
}

// readBatchRequest parses batch arguments and object entries until flush.
func (h *handler) readBatchRequest() ([]batchItem, string, error) {
	var items []batchItem
	inObjectList := false
	requestedHashAlgorithm := ""
	for h.scanner.Scan() {
		if h.scanner.IsFlush() {
			break
		}
		if h.scanner.IsDelim() {
			inObjectList = true
			continue
		}

		line := h.scanner.Text()

		// Before delimiter, lines are command arguments.
		// For legacy clients without delimiter, skip argument-like lines.
		if !inObjectList && strings.Contains(line, "=") {
			if err := h.readBatchArgument(line, &requestedHashAlgorithm); err != nil {
				return nil, "", err
			}
			continue
		}

		oid, size, ok := parseBatchObjectLine(line)
		if !ok {
			continue
		}
		items = append(items, batchItem{oid: oid, size: size})
	}
	if err := h.scanner.Err(); err != nil {
		return nil, "", h.writeStatusWithMessage(400, "invalid batch payload")
	}
	return items, requestedHashAlgorithm, nil
}

// readBatchArgument parses supported key-value arguments in a batch preamble.
func (h *handler) readBatchArgument(line string, requestedHashAlgorithm *string) error {
	key, value, ok := strings.Cut(line, "=")
	if !ok || key != "hash-algo" {
		return nil
	}
	if value != supportedHashAlgorithm {
		return h.writeStatusWithMessage(400, "unsupported hash algorithm")
	}
	*requestedHashAlgorithm = value
	return nil
}

// lookupBatchObjects loads all existing objects referenced by the batch request.
func (h *handler) lookupBatchObjects(ctx context.Context, items []batchItem) (map[lfsx.OID]*database.LFSObject, error) {
	oids := make([]lfsx.OID, 0, len(items))
	for _, item := range items {
		oids = append(oids, item.oid)
	}

	existing, err := h.store.GetLFSObjectsByOIDs(ctx, h.repo.ID, oids...)
	if err != nil {
		return nil, h.writeStatusWithMessage(500, "internal error")
	}

	existingSet := make(map[lfsx.OID]*database.LFSObject, len(existing))
	for _, obj := range existing {
		existingSet[obj.OID] = obj
	}
	return existingSet, nil
}

// writeBatchResponse writes a full batch response including per-object actions.
func (h *handler) writeBatchResponse(items []batchItem, existingSet map[lfsx.OID]*database.LFSObject, requestedHashAlgorithm string) error {
	if err := h.writer.WritePacketText("status 200"); err != nil {
		return err
	}
	if requestedHashAlgorithm != "" {
		if err := h.writer.WritePacketText("hash-algo=" + requestedHashAlgorithm); err != nil {
			return err
		}
	}
	if err := h.writer.WriteDelim(); err != nil {
		return err
	}

	for _, item := range items {
		if err := h.writeBatchItem(item, existingSet[item.oid]); err != nil {
			return err
		}
	}
	return h.writer.WriteFlush()
}

// writeBatchItem determines and writes the action for a single batch object.
func (h *handler) writeBatchItem(item batchItem, obj *database.LFSObject) error {
	if h.operation == "upload" {
		if obj != nil {
			return h.writer.WritePacketText(string(item.oid) + " " + strconv.FormatInt(item.size, 10) + " noop")
		}
		return h.writer.WritePacketText(string(item.oid) + " " + strconv.FormatInt(item.size, 10) + " upload")
	}

	if obj != nil {
		return h.writer.WritePacketText(string(item.oid) + " " + strconv.FormatInt(obj.Size, 10) + " download")
	}
	return h.writer.WritePacketText(string(item.oid) + " " + strconv.FormatInt(item.size, 10) + " noop")
}

// parseBatchObjectLine parses either "<oid> <size>" or "oid=<oid> size=<size>".
func parseBatchObjectLine(line string) (oid lfsx.OID, size int64, ok bool) {
	parts := strings.Fields(line)
	if len(parts) == 2 {
		size, err := strconv.ParseInt(parts[1], 10, 64)
		if err == nil && size >= 0 {
			return lfsx.OID(parts[0]), size, true
		}
	}

	var oidValue string
	var sizeValue int64
	var hasOID bool
	var hasSize bool
	for _, part := range parts {
		entry := strings.SplitN(part, "=", 2)
		if len(entry) != 2 {
			continue
		}

		switch entry[0] {
		case "oid":
			oidValue = entry[1]
			hasOID = true
		case "size":
			parsed, err := strconv.ParseInt(entry[1], 10, 64)
			if err != nil || parsed < 0 {
				continue
			}
			sizeValue = parsed
			hasSize = true
		}
	}

	if hasOID && hasSize {
		return lfsx.OID(oidValue), sizeValue, true
	}
	return "", 0, false
}

// handlePutObject receives object data, stores it, and records metadata in the database.
func (h *handler) handlePutObject(ctx context.Context, oid lfsx.OID) error {
	if err := h.validatePutObjectRequest(oid); err != nil {
		return err
	}

	expectedSize, err := h.readPutObjectExpectedSize()
	if err != nil {
		return err
	}

	dataReader := newPktlineDataReader(h.scanner)
	written, err := h.uploadObjectData(oid, dataReader)
	if err != nil {
		return err
	}

	if expectedSize > 0 && written != expectedSize {
		return h.writeStatusWithMessage(400, "size mismatch")
	}

	if err := h.createUploadedObjectRecord(ctx, oid, written); err != nil {
		return err
	}

	return h.writeStatus(200)
}

// validatePutObjectRequest checks command-level constraints before reading upload data.
func (h *handler) validatePutObjectRequest(oid lfsx.OID) error {
	if h.operation != "upload" {
		h.consumeUntilFlush()
		return h.writeStatusWithMessage(403, "not allowed for download operation")
	}
	if !lfsx.ValidOID(oid) {
		h.consumeUntilFlush()
		return h.writeStatusWithMessage(400, "invalid oid")
	}
	return nil
}

// readPutObjectExpectedSize parses put-object arguments until the data delimiter.
func (h *handler) readPutObjectExpectedSize() (int64, error) {
	var expectedSize int64
	for h.scanner.Scan() {
		if h.scanner.IsDelim() {
			return expectedSize, nil
		}
		if h.scanner.IsFlush() {
			return 0, h.writeStatusWithMessage(400, "expected delimiter before object data")
		}

		line := h.scanner.Text()
		if !strings.HasPrefix(line, "size=") {
			continue
		}

		v, err := strconv.ParseInt(strings.TrimPrefix(line, "size="), 10, 64)
		if err != nil || v < 0 {
			h.consumeUntilFlush()
			return 0, h.writeStatusWithMessage(400, "invalid size")
		}
		expectedSize = v
	}
	return expectedSize, nil
}

// uploadObjectData streams packet data into the configured storage backend.
func (h *handler) uploadObjectData(oid lfsx.OID, dataReader io.Reader) (int64, error) {
	s := h.storagers[h.defaultStorage]
	if s == nil {
		_, _ = io.Copy(io.Discard, dataReader)
		return 0, h.writeStatusWithMessage(500, "storage backend not configured")
	}

	written, err := s.Upload(oid, io.NopCloser(dataReader))
	if err == nil {
		return written, nil
	}

	// Drain any remaining data so the protocol stays in sync.
	_, _ = io.Copy(io.Discard, dataReader)
	if errors.Is(err, lfsx.ErrOIDMismatch) || errors.Is(err, lfsx.ErrInvalidOID) {
		return 0, h.writeStatusWithMessage(400, err.Error())
	}

	log.Error("Failed to upload LFS object via SSH [oid: %s]: %v", oid, err)
	return 0, h.writeStatusWithMessage(500, "upload failed")
}

// createUploadedObjectRecord persists uploaded object metadata with duplicate fallback.
func (h *handler) createUploadedObjectRecord(ctx context.Context, oid lfsx.OID, size int64) error {
	// If the record already exists (for example from a concurrent upload), verify
	// with a follow-up query instead of failing the request.
	err := h.store.CreateLFSObject(ctx, h.repo.ID, oid, size, h.defaultStorage)
	if err == nil {
		log.Trace("[LFS SSH] Object created %q", oid)
		return nil
	}

	if _, lookupErr := h.store.GetLFSObjectByOID(ctx, h.repo.ID, oid); lookupErr != nil {
		log.Error("Failed to create LFS object record [repo_id: %d, oid: %s]: %v", h.repo.ID, oid, err)
		return h.writeStatusWithMessage(500, "failed to create object record")
	}
	log.Trace("[LFS SSH] Object already exists %q", oid)
	return nil
}

// handleGetObject resolves an object and streams its content back to the client.
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

// handleVerifyObject validates that the stored object exists and matches the requested size.
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

// writeStatus writes a status packet followed by a flush packet.
func (h *handler) writeStatus(code int) error {
	if err := h.writer.WritePacketText("status " + strconv.Itoa(code)); err != nil {
		return errors.Wrap(err, "write status")
	}
	return h.writer.WriteFlush()
}

// writeStatusWithMessage writes a status packet, optional message, and a flush packet.
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

// consumeUntilFlush discards packets until a flush packet is reached.
func (h *handler) consumeUntilFlush() {
	for h.scanner.Scan() {
		if h.scanner.IsFlush() {
			return
		}
	}
}
