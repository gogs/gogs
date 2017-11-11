package mssql

import (
	"encoding/binary"
	"errors"
	"io"
)

type packetType uint8

type header struct {
	PacketType packetType
	Status     uint8
	Size       uint16
	Spid       uint16
	PacketNo   uint8
	Pad        uint8
}

// tdsBuffer reads and writes TDS packets of data to the transport.
// The write and read buffers are spearate to make sending attn signals
// possible without locks. Currently attn signals are only sent during
// reads, not writes.
type tdsBuffer struct {
	transport io.ReadWriteCloser

	// Write fields.
	wbuf []byte
	wpos uint16

	// Read fields.
	rbuf        []byte
	rpos        uint16
	rsize       uint16
	final       bool
	packet_type packetType

	// afterFirst is assigned to right after tdsBuffer is created and
	// before the first use. It is executed after the first packet is
	// writen and then removed.
	afterFirst func()
}

func newTdsBuffer(bufsize int, transport io.ReadWriteCloser) *tdsBuffer {
	w := new(tdsBuffer)
	w.wbuf = make([]byte, bufsize)
	w.rbuf = make([]byte, bufsize)
	w.wpos = 0
	w.rpos = 8
	w.transport = transport
	return w
}

func (rw *tdsBuffer) ResizeBuffer(packetsizei int) {
	if len(rw.rbuf) != packetsizei {
		newbuf := make([]byte, packetsizei)
		copy(newbuf, rw.rbuf)
		rw.rbuf = newbuf
	}
	if len(rw.wbuf) != packetsizei {
		newbuf := make([]byte, packetsizei)
		copy(newbuf, rw.wbuf)
		rw.wbuf = newbuf
	}
}

func (w *tdsBuffer) PackageSize() uint32 {
	return uint32(len(w.wbuf))
}

func (w *tdsBuffer) flush() (err error) {
	// writing packet size
	binary.BigEndian.PutUint16(w.wbuf[2:], w.wpos)

	// writing packet into underlying transport
	if _, err = w.transport.Write(w.wbuf[:w.wpos]); err != nil {
		return err
	}

	// execute afterFirst hook if it is set
	if w.afterFirst != nil {
		w.afterFirst()
		w.afterFirst = nil
	}

	w.wpos = 8
	// packet number
	w.wbuf[6] += 1
	return nil
}

func (w *tdsBuffer) Write(p []byte) (total int, err error) {
	total = 0
	for {
		copied := copy(w.wbuf[w.wpos:], p)
		w.wpos += uint16(copied)
		total += copied
		if copied == len(p) {
			break
		}
		if err = w.flush(); err != nil {
			return
		}
		p = p[copied:]
	}
	return
}

func (w *tdsBuffer) WriteByte(b byte) error {
	if int(w.wpos) == len(w.wbuf) {
		if err := w.flush(); err != nil {
			return err
		}
	}
	w.wbuf[w.wpos] = b
	w.wpos += 1
	return nil
}

func (w *tdsBuffer) BeginPacket(packet_type packetType) {
	w.wbuf[0] = byte(packet_type)
	w.wbuf[1] = 0 // packet is incomplete
	w.wbuf[4] = 0 // spid
	w.wbuf[5] = 0
	w.wbuf[6] = 1 // packet id
	w.wbuf[7] = 0 // window
	w.wpos = 8
}

func (w *tdsBuffer) FinishPacket() error {
	w.wbuf[1] = 1 // this is last packet
	return w.flush()
}

func (r *tdsBuffer) readNextPacket() error {
	header := header{}
	var err error
	err = binary.Read(r.transport, binary.BigEndian, &header)
	if err != nil {
		return err
	}
	offset := uint16(binary.Size(header))
	if int(header.Size) > len(r.rbuf) {
		return errors.New("Invalid packet size, it is longer than buffer size")
	}
	if int(offset) > int(header.Size) {
		return errors.New("Invalid packet size, it is shorter than header size")
	}
	_, err = io.ReadFull(r.transport, r.rbuf[offset:header.Size])
	if err != nil {
		return err
	}
	r.rpos = offset
	r.rsize = header.Size
	r.final = header.Status != 0
	r.packet_type = header.PacketType
	return nil
}

func (r *tdsBuffer) BeginRead() (packetType, error) {
	err := r.readNextPacket()
	if err != nil {
		return 0, err
	}
	return r.packet_type, nil
}

func (r *tdsBuffer) ReadByte() (res byte, err error) {
	if r.rpos == r.rsize {
		if r.final {
			return 0, io.EOF
		}
		err = r.readNextPacket()
		if err != nil {
			return 0, err
		}
	}
	res = r.rbuf[r.rpos]
	r.rpos++
	return res, nil
}

func (r *tdsBuffer) byte() byte {
	b, err := r.ReadByte()
	if err != nil {
		badStreamPanic(err)
	}
	return b
}

func (r *tdsBuffer) ReadFull(buf []byte) {
	_, err := io.ReadFull(r, buf[:])
	if err != nil {
		badStreamPanic(err)
	}
}

func (r *tdsBuffer) uint64() uint64 {
	var buf [8]byte
	r.ReadFull(buf[:])
	return binary.LittleEndian.Uint64(buf[:])
}

func (r *tdsBuffer) int32() int32 {
	return int32(r.uint32())
}

func (r *tdsBuffer) uint32() uint32 {
	var buf [4]byte
	r.ReadFull(buf[:])
	return binary.LittleEndian.Uint32(buf[:])
}

func (r *tdsBuffer) uint16() uint16 {
	var buf [2]byte
	r.ReadFull(buf[:])
	return binary.LittleEndian.Uint16(buf[:])
}

func (r *tdsBuffer) BVarChar() string {
	l := int(r.byte())
	return r.readUcs2(l)
}

func (r *tdsBuffer) UsVarChar() string {
	l := int(r.uint16())
	return r.readUcs2(l)
}

func (r *tdsBuffer) readUcs2(numchars int) string {
	b := make([]byte, numchars*2)
	r.ReadFull(b)
	res, err := ucs22str(b)
	if err != nil {
		badStreamPanic(err)
	}
	return res
}

func (r *tdsBuffer) Read(buf []byte) (copied int, err error) {
	copied = 0
	err = nil
	if r.rpos == r.rsize {
		if r.final {
			return 0, io.EOF
		}
		err = r.readNextPacket()
		if err != nil {
			return
		}
	}
	copied = copy(buf, r.rbuf[r.rpos:r.rsize])
	r.rpos += uint16(copied)
	return
}
