package bridge

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
)

// Binary frames carry the bytes of uploads and downloads (protocol spec
// §6.4): the 16-byte id of the attachment, the 8-byte big-endian offset of
// the data in the file, and 1 B to 256 KiB of data. The offset lets the
// receiver drop frames that do not follow what it has (a cancelled download,
// a resumed upload).
const (
	FrameIDLen     = 16
	FrameOffsetLen = 8
	FrameHeaderLen = FrameIDLen + FrameOffsetLen
	MaxFrameData   = 256 << 10
)

// FrameID is an attachment's id as its frames carry it. In JSON it is 32
// lower-case hex digits (Attachment.ID).
type FrameID [FrameIDLen]byte

func (id FrameID) String() string { return hex.EncodeToString(id[:]) }

// ParseFrameID reads an id written as 32 lower-case hex digits.
func ParseFrameID(s string) (FrameID, error) {
	var id FrameID
	if len(s) != 2*FrameIDLen {
		return id, fmt.Errorf("frame id %q: want %d hex digits", s, 2*FrameIDLen)
	}
	for _, c := range s {
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f') {
			return id, fmt.Errorf("frame id %q: want lower-case hex digits", s)
		}
	}
	_, err := hex.Decode(id[:], []byte(s))
	return id, err
}

// ErrFrame is a frame without data, with more than MaxFrameData, or with a
// negative offset.
var ErrFrame = errors.New("a frame has an id, an offset from 0 and 1 B to 256 KiB of data")

// EncodeFrame returns the binary frame of data at offset in the file id.
func EncodeFrame(id FrameID, offset int64, data []byte) ([]byte, error) {
	if len(data) == 0 || len(data) > MaxFrameData || offset < 0 {
		return nil, ErrFrame
	}
	frame := make([]byte, FrameHeaderLen, FrameHeaderLen+len(data))
	copy(frame, id[:])
	binary.BigEndian.PutUint64(frame[FrameIDLen:], uint64(offset))
	return append(frame, data...), nil
}

// DecodeFrame splits a binary frame into its id, its offset and its data. The
// data shares the frame's memory.
func DecodeFrame(frame []byte) (FrameID, int64, []byte, error) {
	var id FrameID
	if len(frame) <= FrameHeaderLen || len(frame) > FrameHeaderLen+MaxFrameData {
		return id, 0, nil, ErrFrame
	}
	off := binary.BigEndian.Uint64(frame[FrameIDLen:FrameHeaderLen])
	if off > math.MaxInt64 {
		return id, 0, nil, ErrFrame
	}
	copy(id[:], frame)
	return id, int64(off), frame[FrameHeaderLen:], nil
}
