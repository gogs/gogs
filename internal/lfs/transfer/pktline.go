package transfer

import (
	"encoding/binary"
	"encoding/hex"
	"io"
	"strings"

	"github.com/cockroachdb/errors"
)

const (
	// maxPacketDataLen is the maximum data length in a single pkt-line packet
	// (65520 total - 4 byte hex header).
	maxPacketDataLen = 65516

	pktFlush = "0000"
	pktDelim = "0001"
)

// PktlineScanner reads pkt-line formatted packets from an io.Reader.
type PktlineScanner struct {
	r       io.Reader
	err     error
	data    []byte
	isFlush bool
	isDelim bool
}

// NewPktlineScanner returns a new PktlineScanner reading from r.
func NewPktlineScanner(r io.Reader) *PktlineScanner {
	return &PktlineScanner{r: r}
}

// Scan reads the next pkt-line packet. Returns false when no more packets are
// available or an error occurred.
func (s *PktlineScanner) Scan() bool {
	s.data = nil
	s.isFlush = false
	s.isDelim = false

	var hdr [4]byte
	_, err := io.ReadFull(s.r, hdr[:])
	if err != nil {
		if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
			s.err = errors.Wrap(err, "read pkt-line header")
		}
		return false
	}

	switch string(hdr[:]) {
	case pktFlush:
		s.isFlush = true
		return true
	case pktDelim:
		s.isDelim = true
		return true
	}

	length, err := hexToUint16(hdr[:])
	if err != nil {
		s.err = errors.Wrap(err, "decode pkt-line length")
		return false
	}
	if length < 4 {
		s.err = errors.Errorf("invalid pkt-line length: %d", length)
		return false
	}

	dataLen := int(length) - 4
	s.data = make([]byte, dataLen)
	_, err = io.ReadFull(s.r, s.data)
	if err != nil {
		s.err = errors.Wrap(err, "read pkt-line data")
		return false
	}
	return true
}

// Bytes returns the raw data of the last scanned packet. Returns nil for flush
// and delim packets.
func (s *PktlineScanner) Bytes() []byte {
	return s.data
}

// Text returns the data of the last scanned packet as a string with the
// trailing newline removed.
func (s *PktlineScanner) Text() string {
	return strings.TrimSuffix(string(s.data), "\n")
}

// IsFlush returns true if the last scanned packet was a flush packet (0000).
func (s *PktlineScanner) IsFlush() bool {
	return s.isFlush
}

// IsDelim returns true if the last scanned packet was a delim packet (0001).
func (s *PktlineScanner) IsDelim() bool {
	return s.isDelim
}

// Err returns the first error encountered during scanning.
func (s *PktlineScanner) Err() error {
	return s.err
}

// PktlineWriter writes pkt-line formatted packets to an io.Writer.
type PktlineWriter struct {
	w io.Writer
}

// NewPktlineWriter returns a new PktlineWriter writing to w.
func NewPktlineWriter(w io.Writer) *PktlineWriter {
	return &PktlineWriter{w: w}
}

// WritePacket writes a single data packet with the pkt-line length prefix.
func (pw *PktlineWriter) WritePacket(data []byte) error {
	length := len(data) + 4
	if length > 65520 {
		return errors.New("packet exceeds maximum length")
	}

	var hdr [4]byte
	uint16ToHex(uint16(length), hdr[:])
	if _, err := pw.w.Write(hdr[:]); err != nil {
		return errors.Wrap(err, "write pkt-line header")
	}
	if _, err := pw.w.Write(data); err != nil {
		return errors.Wrap(err, "write pkt-line data")
	}
	return nil
}

// WritePacketText writes a text line as a pkt-line packet, appending a newline
// if not already present.
func (pw *PktlineWriter) WritePacketText(line string) error {
	if !strings.HasSuffix(line, "\n") {
		line += "\n"
	}
	return pw.WritePacket([]byte(line))
}

// WriteFlush writes a flush packet (0000).
func (pw *PktlineWriter) WriteFlush() error {
	_, err := pw.w.Write([]byte(pktFlush))
	return errors.Wrap(err, "write flush packet")
}

// WriteDelim writes a delim packet (0001).
func (pw *PktlineWriter) WriteDelim() error {
	_, err := pw.w.Write([]byte(pktDelim))
	return errors.Wrap(err, "write delim packet")
}

// WriteData streams binary data as a sequence of pkt-line packets, each at
// most maxPacketDataLen bytes. The caller is responsible for writing a flush
// packet after the data if needed.
func (pw *PktlineWriter) WriteData(r io.Reader) error {
	buf := make([]byte, maxPacketDataLen)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if writeErr := pw.WritePacket(buf[:n]); writeErr != nil {
				return writeErr
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return errors.Wrap(err, "read data for pkt-line")
		}
	}
}

// pktlineDataReader reads binary data from a sequence of pkt-line data packets
// until a flush packet is encountered, presenting them as a continuous
// io.Reader.
type pktlineDataReader struct {
	scanner *PktlineScanner
	buf     []byte
	done    bool
}

// newPktlineDataReader returns a streaming reader over pkt-line data packets.
func newPktlineDataReader(s *PktlineScanner) *pktlineDataReader {
	return &pktlineDataReader{scanner: s}
}

// Read implements io.Reader by concatenating pkt-line data packets until flush.
func (r *pktlineDataReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}

	// Drain the leftover buffer from the previous packet first.
	if len(r.buf) > 0 {
		n := copy(p, r.buf)
		r.buf = r.buf[n:]
		return n, nil
	}

	if !r.scanner.Scan() {
		r.done = true
		if err := r.scanner.Err(); err != nil {
			return 0, err
		}
		return 0, io.EOF
	}
	if r.scanner.IsFlush() {
		r.done = true
		return 0, io.EOF
	}
	if r.scanner.IsDelim() {
		r.done = true
		return 0, io.EOF
	}

	data := r.scanner.Bytes()
	n := copy(p, data)
	if n < len(data) {
		r.buf = data[n:]
	}
	return n, nil
}

// hexToUint16 decodes a 4-byte ASCII hexadecimal length into uint16.
func hexToUint16(b []byte) (uint16, error) {
	var decoded [2]byte
	_, err := hex.Decode(decoded[:], b)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(decoded[:]), nil
}

// uint16ToHex encodes a uint16 length as a 4-byte ASCII hexadecimal value.
func uint16ToHex(v uint16, b []byte) {
	var raw [2]byte
	binary.BigEndian.PutUint16(raw[:], v)
	hex.Encode(b, raw[:])
}
