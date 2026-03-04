package transfer

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/lfsx"
)

var _ Store = (*mockStore)(nil)

type mockStore struct {
	objects   map[lfsx.OID]*database.LFSObject
	created   []*database.LFSObject
	createErr error // If set, CreateLFSObject returns this error.
}

func newMockStore() *mockStore {
	return &mockStore{
		objects: make(map[lfsx.OID]*database.LFSObject),
	}
}

func (s *mockStore) CreateLFSObject(_ context.Context, repoID int64, oid lfsx.OID, size int64, storage lfsx.Storage) error {
	if s.createErr != nil {
		return s.createErr
	}
	obj := &database.LFSObject{
		RepoID:  repoID,
		OID:     oid,
		Size:    size,
		Storage: storage,
	}
	s.objects[oid] = obj
	s.created = append(s.created, obj)
	return nil
}

func (s *mockStore) GetLFSObjectByOID(_ context.Context, repoID int64, oid lfsx.OID) (*database.LFSObject, error) {
	obj, ok := s.objects[oid]
	if !ok {
		return nil, database.ErrLFSObjectNotExist{}
	}
	return obj, nil
}

func (s *mockStore) GetLFSObjectsByOIDs(_ context.Context, repoID int64, oids ...lfsx.OID) ([]*database.LFSObject, error) {
	var result []*database.LFSObject
	for _, oid := range oids {
		if obj, ok := s.objects[oid]; ok {
			result = append(result, obj)
		}
	}
	return result, nil
}

var _ lfsx.Storager = (*mockStorage)(nil)

type mockStorage struct {
	objects map[lfsx.OID][]byte
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		objects: make(map[lfsx.OID][]byte),
	}
}

func (*mockStorage) Storage() lfsx.Storage {
	return "memory"
}

func (s *mockStorage) Upload(oid lfsx.OID, rc io.ReadCloser) (int64, error) {
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return 0, err
	}
	s.objects[oid] = data
	return int64(len(data)), nil
}

func (s *mockStorage) Download(oid lfsx.OID, w io.Writer) error {
	data, ok := s.objects[oid]
	if !ok {
		return lfsx.ErrObjectNotExist
	}
	_, err := w.Write(data)
	return err
}

// clientWriter builds pkt-line input simulating a Git LFS client.
type clientWriter struct {
	buf bytes.Buffer
	pw  *PktlineWriter
}

func newClientWriter() *clientWriter {
	cw := &clientWriter{}
	cw.pw = NewPktlineWriter(&cw.buf)
	return cw
}

func (cw *clientWriter) text(line string)  { _ = cw.pw.WritePacketText(line) }
func (cw *clientWriter) flush()            { _ = cw.pw.WriteFlush() }
func (cw *clientWriter) delim()            { _ = cw.pw.WriteDelim() }
func (cw *clientWriter) data(d []byte)     { _ = cw.pw.WritePacket(d) }
func (cw *clientWriter) reader() io.Reader { return &cw.buf }

// readStatus reads a status response (status line + optional message lines + flush).
func readStatus(t *testing.T, s *PktlineScanner) (code string, messages []string) {
	t.Helper()
	require.True(t, s.Scan(), "expected status line, got EOF or error: %v", s.Err())
	code = s.Text()

	for s.Scan() {
		if s.IsFlush() {
			return code, messages
		}
		messages = append(messages, s.Text())
	}
	t.Fatal("expected flush after status")
	return "", nil
}

const testOID = "ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f"

func TestServe_VersionNegotiation(t *testing.T) {
	cw := newClientWriter()
	cw.text("version 1")
	cw.flush()
	cw.text("quit")
	cw.flush()

	var out bytes.Buffer
	err := Serve(
		context.Background(),
		cw.reader(),
		&out,
		"download",
		&database.Repository{ID: 1},
		newMockStore(),
		"memory",
		nil,
	)
	require.NoError(t, err)

	s := NewPktlineScanner(&out)

	// Server capability advertisement.
	require.True(t, s.Scan())
	assert.Equal(t, "version=1", s.Text())
	require.True(t, s.Scan())
	assert.True(t, s.IsFlush())

	// Version acknowledgement.
	code, _ := readStatus(t, s)
	assert.Equal(t, "status 200", code)

	// Quit response.
	code, _ = readStatus(t, s)
	assert.Equal(t, "status 200", code)
}

func TestServe_UnsupportedVersion(t *testing.T) {
	cw := newClientWriter()
	cw.text("version 99")
	cw.flush()

	var out bytes.Buffer
	err := Serve(
		context.Background(),
		cw.reader(),
		&out,
		"download",
		&database.Repository{ID: 1},
		newMockStore(),
		"memory",
		nil,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported client version")
}

func TestServe_BatchUpload(t *testing.T) {
	store := newMockStore()
	store.objects[lfsx.OID(testOID)] = &database.LFSObject{
		OID:  lfsx.OID(testOID),
		Size: 42,
	}
	newOID := lfsx.OID("aabbccdd18f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f")

	cw := newClientWriter()
	cw.text("version 1")
	cw.flush()
	// Batch command.
	cw.text("batch")
	cw.text("hash-algo=sha256")
	cw.text(testOID + " 42")
	cw.text(string(newOID) + " 100")
	cw.flush()
	cw.text("quit")
	cw.flush()

	var out bytes.Buffer
	err := Serve(
		context.Background(),
		cw.reader(),
		&out,
		"upload",
		&database.Repository{ID: 1},
		store,
		"memory",
		nil,
	)
	require.NoError(t, err)

	s := NewPktlineScanner(&out)

	// Skip version advertisement + ack.
	for i := 0; i < 4; i++ {
		require.True(t, s.Scan())
	}

	// Batch response: status 200 + flush.
	require.True(t, s.Scan())
	assert.Equal(t, "status 200", s.Text())
	require.True(t, s.Scan())
	assert.True(t, s.IsFlush())

	// Existing object → noop.
	require.True(t, s.Scan())
	assert.Equal(t, testOID+" 42 noop", s.Text())

	// New object → upload needed (no noop).
	require.True(t, s.Scan())
	assert.Equal(t, string(newOID)+" 100", s.Text())

	require.True(t, s.Scan())
	assert.True(t, s.IsFlush())
}

func TestServe_BatchDownload(t *testing.T) {
	store := newMockStore()
	store.objects[lfsx.OID(testOID)] = &database.LFSObject{
		OID:  lfsx.OID(testOID),
		Size: 42,
	}
	missingOID := lfsx.OID("aabbccdd18f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f")

	cw := newClientWriter()
	cw.text("version 1")
	cw.flush()
	cw.text("batch")
	cw.text(testOID + " 42")
	cw.text(string(missingOID) + " 100")
	cw.flush()
	cw.text("quit")
	cw.flush()

	var out bytes.Buffer
	err := Serve(
		context.Background(),
		cw.reader(),
		&out,
		"download",
		&database.Repository{ID: 1},
		store,
		"memory",
		nil,
	)
	require.NoError(t, err)

	s := NewPktlineScanner(&out)

	// Skip version advertisement + ack.
	for i := 0; i < 4; i++ {
		require.True(t, s.Scan())
	}

	// Batch response.
	require.True(t, s.Scan())
	assert.Equal(t, "status 200", s.Text())
	require.True(t, s.Scan())
	assert.True(t, s.IsFlush())

	// Existing object → available for download (actual size from DB).
	require.True(t, s.Scan())
	assert.Equal(t, testOID+" 42", s.Text())

	// Missing object → noop.
	require.True(t, s.Scan())
	assert.Equal(t, string(missingOID)+" 100 noop", s.Text())

	require.True(t, s.Scan())
	assert.True(t, s.IsFlush())
}

func TestServe_PutObject(t *testing.T) {
	store := newMockStore()
	storage := newMockStorage()

	cw := newClientWriter()
	cw.text("version 1")
	cw.flush()
	cw.text("put-object " + testOID)
	cw.text("size=13")
	cw.delim()
	cw.data([]byte("hello, world!"))
	cw.flush()
	cw.text("quit")
	cw.flush()

	var out bytes.Buffer
	err := Serve(
		context.Background(),
		cw.reader(),
		&out,
		"upload",
		&database.Repository{ID: 1},
		store,
		storage.Storage(),
		map[lfsx.Storage]lfsx.Storager{storage.Storage(): storage},
	)
	require.NoError(t, err)

	s := NewPktlineScanner(&out)

	// Skip version advertisement + ack.
	for i := 0; i < 4; i++ {
		require.True(t, s.Scan())
	}

	// Put-object response.
	code, _ := readStatus(t, s)
	assert.Equal(t, "status 200", code)

	// Verify the object was stored.
	assert.Equal(t, []byte("hello, world!"), storage.objects[lfsx.OID(testOID)])

	// Verify the DB record was created.
	assert.Len(t, store.created, 1)
	assert.Equal(t, lfsx.OID(testOID), store.created[0].OID)
	assert.Equal(t, int64(13), store.created[0].Size)
}

func TestServe_PutObjectForbiddenOnDownload(t *testing.T) {
	cw := newClientWriter()
	cw.text("version 1")
	cw.flush()
	cw.text("put-object " + testOID)
	cw.text("size=13")
	cw.delim()
	cw.data([]byte("hello, world!"))
	cw.flush()
	cw.text("quit")
	cw.flush()

	var out bytes.Buffer
	err := Serve(
		context.Background(),
		cw.reader(),
		&out,
		"download",
		&database.Repository{ID: 1},
		newMockStore(),
		"memory",
		nil,
	)
	require.NoError(t, err)

	s := NewPktlineScanner(&out)

	// Skip version advertisement + ack.
	for i := 0; i < 4; i++ {
		require.True(t, s.Scan())
	}

	// Put-object should be rejected with 403.
	code, msgs := readStatus(t, s)
	assert.Equal(t, "status 403", code)
	assert.Contains(t, msgs, "not allowed for download operation")
}

func TestServe_GetObject(t *testing.T) {
	store := newMockStore()
	storage := newMockStorage()
	storage.objects[lfsx.OID(testOID)] = []byte("file content here")
	store.objects[lfsx.OID(testOID)] = &database.LFSObject{
		OID:     lfsx.OID(testOID),
		Size:    17,
		Storage: storage.Storage(),
	}

	cw := newClientWriter()
	cw.text("version 1")
	cw.flush()
	cw.text("get-object " + testOID)
	cw.flush()
	cw.text("quit")
	cw.flush()

	var out bytes.Buffer
	err := Serve(
		context.Background(),
		cw.reader(),
		&out,
		"download",
		&database.Repository{ID: 1},
		store,
		storage.Storage(),
		map[lfsx.Storage]lfsx.Storager{storage.Storage(): storage},
	)
	require.NoError(t, err)

	s := NewPktlineScanner(&out)

	// Skip version advertisement + ack.
	for i := 0; i < 4; i++ {
		require.True(t, s.Scan())
	}

	// Get-object response: status 200 + size + delim + data + flush.
	require.True(t, s.Scan())
	assert.Equal(t, "status 200", s.Text())

	require.True(t, s.Scan())
	assert.Equal(t, "size=17", s.Text())

	require.True(t, s.Scan())
	assert.True(t, s.IsDelim())

	// Read binary data.
	dr := newPktlineDataReader(s)
	data, err := io.ReadAll(dr)
	require.NoError(t, err)
	assert.Equal(t, []byte("file content here"), data)

	// After data, the reader should have consumed the flush.
	// Quit response.
	code, _ := readStatus(t, s)
	assert.Equal(t, "status 200", code)
}

func TestServe_GetObjectNotFound(t *testing.T) {
	cw := newClientWriter()
	cw.text("version 1")
	cw.flush()
	cw.text("get-object " + testOID)
	cw.flush()
	cw.text("quit")
	cw.flush()

	var out bytes.Buffer
	err := Serve(
		context.Background(),
		cw.reader(),
		&out,
		"download",
		&database.Repository{ID: 1},
		newMockStore(),
		"memory",
		nil,
	)
	require.NoError(t, err)

	s := NewPktlineScanner(&out)
	for i := 0; i < 4; i++ {
		require.True(t, s.Scan())
	}

	code, msgs := readStatus(t, s)
	assert.Equal(t, "status 404", code)
	assert.Contains(t, msgs, "object does not exist")
}

func TestServe_VerifyObject(t *testing.T) {
	store := newMockStore()
	store.objects[lfsx.OID(testOID)] = &database.LFSObject{
		OID:  lfsx.OID(testOID),
		Size: 42,
	}

	t.Run("size match", func(t *testing.T) {
		cw := newClientWriter()
		cw.text("version 1")
		cw.flush()
		cw.text("verify-object " + testOID)
		cw.text("size=42")
		cw.flush()
		cw.text("quit")
		cw.flush()

		var out bytes.Buffer
		err := Serve(
			context.Background(),
			cw.reader(),
			&out,
			"upload",
			&database.Repository{ID: 1},
			store,
			"memory",
			nil,
		)
		require.NoError(t, err)

		s := NewPktlineScanner(&out)
		for i := 0; i < 4; i++ {
			require.True(t, s.Scan())
		}

		code, _ := readStatus(t, s)
		assert.Equal(t, "status 200", code)
	})

	t.Run("size mismatch", func(t *testing.T) {
		cw := newClientWriter()
		cw.text("version 1")
		cw.flush()
		cw.text("verify-object " + testOID)
		cw.text("size=99")
		cw.flush()
		cw.text("quit")
		cw.flush()

		var out bytes.Buffer
		err := Serve(
			context.Background(),
			cw.reader(),
			&out,
			"upload",
			&database.Repository{ID: 1},
			store,
			"memory",
			nil,
		)
		require.NoError(t, err)

		s := NewPktlineScanner(&out)
		for i := 0; i < 4; i++ {
			require.True(t, s.Scan())
		}

		code, msgs := readStatus(t, s)
		assert.Equal(t, "status 409", code)
		assert.Contains(t, msgs, "size mismatch")
	})
}

func TestServe_UnknownCommand(t *testing.T) {
	cw := newClientWriter()
	cw.text("version 1")
	cw.flush()
	cw.text("foobar")
	cw.flush()
	cw.text("quit")
	cw.flush()

	var out bytes.Buffer
	err := Serve(
		context.Background(),
		cw.reader(),
		&out,
		"download",
		&database.Repository{ID: 1},
		newMockStore(),
		"memory",
		nil,
	)
	require.NoError(t, err)

	s := NewPktlineScanner(&out)
	for i := 0; i < 4; i++ {
		require.True(t, s.Scan())
	}

	code, msgs := readStatus(t, s)
	assert.Equal(t, "status 400", code)
	assert.Contains(t, msgs, "unknown command")
}

func TestServe_PutObjectDuplicateRecord(t *testing.T) {
	store := newMockStore()
	storage := newMockStorage()

	// Pre-populate the object so GetLFSObjectByOID succeeds on fallback.
	store.objects[lfsx.OID(testOID)] = &database.LFSObject{
		OID:  lfsx.OID(testOID),
		Size: 13,
	}
	// Make CreateLFSObject fail to simulate a duplicate key error.
	store.createErr = errors.New("duplicate key")

	cw := newClientWriter()
	cw.text("version 1")
	cw.flush()
	cw.text("put-object " + testOID)
	cw.text("size=13")
	cw.delim()
	cw.data([]byte("hello, world!"))
	cw.flush()
	cw.text("quit")
	cw.flush()

	var out bytes.Buffer
	err := Serve(
		context.Background(),
		cw.reader(),
		&out,
		"upload",
		&database.Repository{ID: 1},
		store,
		storage.Storage(),
		map[lfsx.Storage]lfsx.Storager{storage.Storage(): storage},
	)
	require.NoError(t, err)

	s := NewPktlineScanner(&out)
	for i := 0; i < 4; i++ {
		require.True(t, s.Scan())
	}

	// Should succeed because the object exists on follow-up query.
	code, _ := readStatus(t, s)
	assert.Equal(t, "status 200", code)
}

func TestServe_PutObjectInvalidSize(t *testing.T) {
	storage := newMockStorage()

	cw := newClientWriter()
	cw.text("version 1")
	cw.flush()
	cw.text("put-object " + testOID)
	cw.text("size=abc")
	cw.delim()
	cw.data([]byte("hello, world!"))
	cw.flush()
	cw.text("quit")
	cw.flush()

	var out bytes.Buffer
	err := Serve(
		context.Background(),
		cw.reader(),
		&out,
		"upload",
		&database.Repository{ID: 1},
		newMockStore(),
		storage.Storage(),
		map[lfsx.Storage]lfsx.Storager{storage.Storage(): storage},
	)
	require.NoError(t, err)

	s := NewPktlineScanner(&out)
	for i := 0; i < 4; i++ {
		require.True(t, s.Scan())
	}

	code, msgs := readStatus(t, s)
	assert.Equal(t, "status 400", code)
	assert.Contains(t, msgs, "invalid size")
}

func TestServe_VerifyObjectInvalidSize(t *testing.T) {
	store := newMockStore()
	store.objects[lfsx.OID(testOID)] = &database.LFSObject{
		OID:  lfsx.OID(testOID),
		Size: 42,
	}

	t.Run("non-numeric", func(t *testing.T) {
		cw := newClientWriter()
		cw.text("version 1")
		cw.flush()
		cw.text("verify-object " + testOID)
		cw.text("size=notanumber")
		cw.flush()
		cw.text("quit")
		cw.flush()

		var out bytes.Buffer
		err := Serve(
			context.Background(),
			cw.reader(),
			&out,
			"upload",
			&database.Repository{ID: 1},
			store,
			"memory",
			nil,
		)
		require.NoError(t, err)

		s := NewPktlineScanner(&out)
		for i := 0; i < 4; i++ {
			require.True(t, s.Scan())
		}

		code, msgs := readStatus(t, s)
		assert.Equal(t, "status 400", code)
		assert.Contains(t, msgs, "invalid size")

		// Verify the session continues — quit should get a clean response.
		code, _ = readStatus(t, s)
		assert.Equal(t, "status 200", code)
	})

	t.Run("negative", func(t *testing.T) {
		cw := newClientWriter()
		cw.text("version 1")
		cw.flush()
		cw.text("verify-object " + testOID)
		cw.text("size=-1")
		cw.flush()
		cw.text("quit")
		cw.flush()

		var out bytes.Buffer
		err := Serve(
			context.Background(),
			cw.reader(),
			&out,
			"upload",
			&database.Repository{ID: 1},
			store,
			"memory",
			nil,
		)
		require.NoError(t, err)

		s := NewPktlineScanner(&out)
		for i := 0; i < 4; i++ {
			require.True(t, s.Scan())
		}

		code, msgs := readStatus(t, s)
		assert.Equal(t, "status 400", code)
		assert.Contains(t, msgs, "invalid size")
	})
}

func TestServe_PutObjectNegativeSize(t *testing.T) {
	storage := newMockStorage()

	cw := newClientWriter()
	cw.text("version 1")
	cw.flush()
	cw.text("put-object " + testOID)
	cw.text("size=-5")
	cw.delim()
	cw.data([]byte("hello, world!"))
	cw.flush()
	cw.text("quit")
	cw.flush()

	var out bytes.Buffer
	err := Serve(
		context.Background(),
		cw.reader(),
		&out,
		"upload",
		&database.Repository{ID: 1},
		newMockStore(),
		storage.Storage(),
		map[lfsx.Storage]lfsx.Storager{storage.Storage(): storage},
	)
	require.NoError(t, err)

	s := NewPktlineScanner(&out)
	for i := 0; i < 4; i++ {
		require.True(t, s.Scan())
	}

	code, msgs := readStatus(t, s)
	assert.Equal(t, "status 400", code)
	assert.Contains(t, msgs, "invalid size")
}
