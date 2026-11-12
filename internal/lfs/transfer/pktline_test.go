package transfer

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPktlineScanner(t *testing.T) {
	t.Run("data packets", func(t *testing.T) {
		// "0009hello\n" = length 9 (4 header + 5 data)
		input := "000ahello\n"
		s := NewPktlineScanner(strings.NewReader(input))

		require.True(t, s.Scan())
		assert.Equal(t, "hello", s.Text())
		assert.Equal(t, []byte("hello\n"), s.Bytes())
		assert.False(t, s.IsFlush())
		assert.False(t, s.IsDelim())

		assert.False(t, s.Scan())
		assert.NoError(t, s.Err())
	})

	t.Run("flush packet", func(t *testing.T) {
		s := NewPktlineScanner(strings.NewReader("0000"))

		require.True(t, s.Scan())
		assert.True(t, s.IsFlush())
		assert.False(t, s.IsDelim())
		assert.Nil(t, s.Bytes())
		assert.Empty(t, s.Text())
	})

	t.Run("delim packet", func(t *testing.T) {
		s := NewPktlineScanner(strings.NewReader("0001"))

		require.True(t, s.Scan())
		assert.True(t, s.IsDelim())
		assert.False(t, s.IsFlush())
		assert.Nil(t, s.Bytes())
	})

	t.Run("mixed sequence", func(t *testing.T) {
		// "version 1\n" (len=14 -> 000e) + flush + "status 200\n" (len=15 -> 000f) + flush
		input := "000eversion 1\n" + "0000" + "000fstatus 200\n" + "0000"
		s := NewPktlineScanner(strings.NewReader(input))

		require.True(t, s.Scan())
		assert.Equal(t, "version 1", s.Text())

		require.True(t, s.Scan())
		assert.True(t, s.IsFlush())

		require.True(t, s.Scan())
		assert.Equal(t, "status 200", s.Text())

		require.True(t, s.Scan())
		assert.True(t, s.IsFlush())

		assert.False(t, s.Scan())
		assert.NoError(t, s.Err())
	})

	t.Run("empty input", func(t *testing.T) {
		s := NewPktlineScanner(strings.NewReader(""))
		assert.False(t, s.Scan())
		assert.NoError(t, s.Err())
	})

	t.Run("truncated header", func(t *testing.T) {
		s := NewPktlineScanner(strings.NewReader("00"))
		assert.False(t, s.Scan())
		assert.NoError(t, s.Err()) // EOF during header is treated as clean termination
	})

	t.Run("invalid hex header", func(t *testing.T) {
		s := NewPktlineScanner(strings.NewReader("xxxx"))
		assert.False(t, s.Scan())
		assert.Error(t, s.Err())
	})

	t.Run("truncated data", func(t *testing.T) {
		// Header says 10 bytes total (6 data), but only 3 data bytes follow.
		s := NewPktlineScanner(strings.NewReader("000aabc"))
		assert.False(t, s.Scan())
		assert.Error(t, s.Err())
	})
}

func TestPktlineWriter(t *testing.T) {
	t.Run("write packet", func(t *testing.T) {
		var buf bytes.Buffer
		pw := NewPktlineWriter(&buf)

		require.NoError(t, pw.WritePacket([]byte("hello\n")))
		assert.Equal(t, "000ahello\n", buf.String())
	})

	t.Run("write packet text", func(t *testing.T) {
		var buf bytes.Buffer
		pw := NewPktlineWriter(&buf)

		require.NoError(t, pw.WritePacketText("hello"))
		assert.Equal(t, "000ahello\n", buf.String())
	})

	t.Run("write packet text with trailing newline", func(t *testing.T) {
		var buf bytes.Buffer
		pw := NewPktlineWriter(&buf)

		require.NoError(t, pw.WritePacketText("hello\n"))
		assert.Equal(t, "000ahello\n", buf.String())
	})

	t.Run("write flush", func(t *testing.T) {
		var buf bytes.Buffer
		pw := NewPktlineWriter(&buf)

		require.NoError(t, pw.WriteFlush())
		assert.Equal(t, "0000", buf.String())
	})

	t.Run("write delim", func(t *testing.T) {
		var buf bytes.Buffer
		pw := NewPktlineWriter(&buf)

		require.NoError(t, pw.WriteDelim())
		assert.Equal(t, "0001", buf.String())
	})

	t.Run("write data streaming", func(t *testing.T) {
		var buf bytes.Buffer
		pw := NewPktlineWriter(&buf)

		data := []byte("some binary data")
		require.NoError(t, pw.WriteData(bytes.NewReader(data)))

		// Verify it can be scanned back.
		s := NewPktlineScanner(&buf)
		require.True(t, s.Scan())
		assert.Equal(t, data, s.Bytes())
	})
}

func TestPktlineRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	pw := NewPktlineWriter(&buf)

	require.NoError(t, pw.WritePacketText("version 1"))
	require.NoError(t, pw.WriteFlush())
	require.NoError(t, pw.WritePacketText("status 200"))
	require.NoError(t, pw.WritePacketText("size=42"))
	require.NoError(t, pw.WriteDelim())
	require.NoError(t, pw.WritePacket([]byte("binary content")))
	require.NoError(t, pw.WriteFlush())

	s := NewPktlineScanner(&buf)

	require.True(t, s.Scan())
	assert.Equal(t, "version 1", s.Text())

	require.True(t, s.Scan())
	assert.True(t, s.IsFlush())

	require.True(t, s.Scan())
	assert.Equal(t, "status 200", s.Text())

	require.True(t, s.Scan())
	assert.Equal(t, "size=42", s.Text())

	require.True(t, s.Scan())
	assert.True(t, s.IsDelim())

	require.True(t, s.Scan())
	assert.Equal(t, []byte("binary content"), s.Bytes())

	require.True(t, s.Scan())
	assert.True(t, s.IsFlush())

	assert.False(t, s.Scan())
	assert.NoError(t, s.Err())
}

func TestPktlineDataReader(t *testing.T) {
	t.Run("reads data packets until flush", func(t *testing.T) {
		var buf bytes.Buffer
		pw := NewPktlineWriter(&buf)
		require.NoError(t, pw.WritePacket([]byte("chunk1")))
		require.NoError(t, pw.WritePacket([]byte("chunk2")))
		require.NoError(t, pw.WriteFlush())

		s := NewPktlineScanner(&buf)
		dr := newPktlineDataReader(s)

		data, err := io.ReadAll(dr)
		require.NoError(t, err)
		assert.Equal(t, []byte("chunk1chunk2"), data)
	})

	t.Run("reads data packets until delim", func(t *testing.T) {
		var buf bytes.Buffer
		pw := NewPktlineWriter(&buf)
		require.NoError(t, pw.WritePacket([]byte("data")))
		require.NoError(t, pw.WriteDelim())

		s := NewPktlineScanner(&buf)
		dr := newPktlineDataReader(s)

		data, err := io.ReadAll(dr)
		require.NoError(t, err)
		assert.Equal(t, []byte("data"), data)
	})

	t.Run("empty data", func(t *testing.T) {
		var buf bytes.Buffer
		pw := NewPktlineWriter(&buf)
		require.NoError(t, pw.WriteFlush())

		s := NewPktlineScanner(&buf)
		dr := newPktlineDataReader(s)

		data, err := io.ReadAll(dr)
		require.NoError(t, err)
		assert.Empty(t, data)
	})

	t.Run("small reads", func(t *testing.T) {
		var buf bytes.Buffer
		pw := NewPktlineWriter(&buf)
		require.NoError(t, pw.WritePacket([]byte("abcdef")))
		require.NoError(t, pw.WriteFlush())

		s := NewPktlineScanner(&buf)
		dr := newPktlineDataReader(s)

		// Read 2 bytes at a time to exercise the leftover buffer.
		p := make([]byte, 2)
		var result []byte
		for {
			n, err := dr.Read(p)
			result = append(result, p[:n]...)
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
		}
		assert.Equal(t, []byte("abcdef"), result)
	})
}
