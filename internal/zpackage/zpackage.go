// Package zpackage implements Valheim's ZPackage wire format.
//
// A ZPackage is a MemoryStream wrapped in .NET's BinaryWriter/BinaryReader, so
// the format is whatever those two types do: little-endian, unpadded, with
// strings carrying a 7-bit encoded (LEB128) length prefix. Everything here
// follows from that. See docs/protocol.md.
//
// Reader uses sticky errors: once a read fails, later reads are no-ops and the
// zero value is returned. Decode a whole message, then check Err once, rather
// than testing after every field.
package zpackage

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"unicode/utf16"
	"unicode/utf8"
)

// ErrOverflow is returned when a read runs past the end of the buffer.
var ErrOverflow = errors.New("zpackage: read past end of package")

// ZDOID identifies a networked object. On the wire it is the owning user's ID
// followed by a per-user object counter — 12 bytes, not a single 64-bit handle.
//
// The game interns UserID into a table and stores a ushort key in memory; that
// is an in-process optimisation and never appears on the wire, so both fields
// are kept as-is here.
type ZDOID struct {
	UserID int64
	ID     uint32
}

// None is the nil ZDOID.
var None = ZDOID{}

// IsNone reports whether z is the nil ZDOID.
func (z ZDOID) IsNone() bool { return z.UserID == 0 && z.ID == 0 }

func (z ZDOID) String() string { return fmt.Sprintf("%d:%d", z.UserID, z.ID) }

// Vector3 mirrors UnityEngine.Vector3.
type Vector3 struct {
	X float32 `json:"x"`
	Y float32 `json:"y"`
	Z float32 `json:"z"`
}

func (v Vector3) String() string { return fmt.Sprintf("(%.3f, %.3f, %.3f)", v.X, v.Y, v.Z) }

// LogValue logs a vector as a group of its three numbers rather than as a
// string, so a reader of the log gets fields it can filter on.
func (v Vector3) LogValue() slog.Value {
	mm := func(f float32) float64 { return math.Round(float64(f)*1000) / 1000 }
	return slog.GroupValue(
		slog.Float64("x", mm(v.X)),
		slog.Float64("y", mm(v.Y)),
		slog.Float64("z", mm(v.Z)),
	)
}

// Quaternion mirrors UnityEngine.Quaternion.
type Quaternion struct{ X, Y, Z, W float32 }

// Vector2i mirrors Valheim's Vector2i (two int32s).
type Vector2i struct{ X, Y int32 }

// Vector2s mirrors Valheim's Vector2s (two int16s).
type Vector2s struct{ X, Y int16 }

// ==================================================================================== //
// Writer
// ==================================================================================== //

// Writer builds a ZPackage.
type Writer struct {
	buf []byte
}

// NewWriter returns an empty Writer.
func NewWriter() *Writer { return &Writer{} }

// Bytes returns the encoded package. The result aliases the Writer's buffer.
func (w *Writer) Bytes() []byte { return w.buf }

// Len returns the number of bytes written so far.
func (w *Writer) Len() int { return len(w.buf) }

// Reset truncates the Writer, retaining the underlying buffer.
func (w *Writer) Reset() { w.buf = w.buf[:0] }

func (w *Writer) WriteUInt8(v byte)  { w.buf = append(w.buf, v) }
func (w *Writer) WriteInt8(v int8)   { w.buf = append(w.buf, byte(v)) }
func (w *Writer) WriteInt16(v int16) { w.buf = binary.LittleEndian.AppendUint16(w.buf, uint16(v)) }
func (w *Writer) WriteUInt16(v uint16) {
	w.buf = binary.LittleEndian.AppendUint16(w.buf, v)
}
func (w *Writer) WriteInt32(v int32)   { w.buf = binary.LittleEndian.AppendUint32(w.buf, uint32(v)) }
func (w *Writer) WriteUInt32(v uint32) { w.buf = binary.LittleEndian.AppendUint32(w.buf, v) }
func (w *Writer) WriteInt64(v int64)   { w.buf = binary.LittleEndian.AppendUint64(w.buf, uint64(v)) }
func (w *Writer) WriteUInt64(v uint64) { w.buf = binary.LittleEndian.AppendUint64(w.buf, v) }

func (w *Writer) WriteSingle(v float32) {
	w.buf = binary.LittleEndian.AppendUint32(w.buf, math.Float32bits(v))
}

func (w *Writer) WriteDouble(v float64) {
	w.buf = binary.LittleEndian.AppendUint64(w.buf, math.Float64bits(v))
}

// WriteBool writes a .NET bool: one byte, 0 or 1.
func (w *Writer) WriteBool(v bool) {
	if v {
		w.buf = append(w.buf, 1)
		return
	}
	w.buf = append(w.buf, 0)
}

// WriteChar writes a .NET char. BinaryWriter encodes it with the stream's encoding,
// which is UTF-8, so this is 1-3 bytes rather than a fixed 2.
func (w *Writer) WriteChar(v rune) {
	w.buf = utf8.AppendRune(w.buf, v)
}

// WriteString writes a .NET string: a 7-bit encoded length prefix counting UTF-8
// bytes, then the bytes themselves. The prefix is LEB128 and is NOT the same
// encoding as NumItems.
func (w *Writer) WriteString(v string) {
	w.write7BitEncodedInt(uint32(len(v)))
	w.buf = append(w.buf, v...)
}

func (w *Writer) write7BitEncodedInt(v uint32) {
	for v >= 0x80 {
		w.buf = append(w.buf, byte(v)|0x80)
		v >>= 7
	}
	w.buf = append(w.buf, byte(v))
}

// WriteByteArray writes a length-prefixed byte slice (int32 length, then raw bytes).
func (w *Writer) WriteByteArray(v []byte) {
	w.WriteInt32(int32(len(v)))
	w.buf = append(w.buf, v...)
}

// WriteRaw appends bytes with no length prefix.
func (w *Writer) WriteRaw(v []byte) { w.buf = append(w.buf, v...) }

// WritePackage writes a nested package, length-prefixed with an int32.
func (w *Writer) WritePackage(p *Writer) { w.WriteByteArray(p.Bytes()) }

// WriteCompressedPackage writes a gzip-compressed nested package. Valheim uses
// GZipStream at CompressionLevel.Fastest, not raw deflate.
func (w *Writer) WriteCompressedPackage(p *Writer) error {
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	if err != nil {
		return err
	}
	if _, err := zw.Write(p.Bytes()); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	w.WriteByteArray(buf.Bytes())
	return nil
}

// WriteNumItems writes Valheim's own compact count encoding. This is a different
// scheme from the 7-bit string prefix: values under 128 take one byte,
// otherwise two, high byte first with 0x80 set. The maximum is 32767.
func (w *Writer) WriteNumItems(n int) {
	if n < 128 {
		w.buf = append(w.buf, byte(n))
		return
	}
	w.buf = append(w.buf, byte((n>>8)|0x80), byte(n))
}

func (w *Writer) WriteZDOID(v ZDOID) {
	w.WriteInt64(v.UserID)
	w.WriteUInt32(v.ID)
}

func (w *Writer) WriteVector3(v Vector3) {
	w.WriteSingle(v.X)
	w.WriteSingle(v.Y)
	w.WriteSingle(v.Z)
}

func (w *Writer) WriteQuaternion(v Quaternion) {
	w.WriteSingle(v.X)
	w.WriteSingle(v.Y)
	w.WriteSingle(v.Z)
	w.WriteSingle(v.W)
}

func (w *Writer) WriteVector2i(v Vector2i) {
	w.WriteInt32(v.X)
	w.WriteInt32(v.Y)
}

func (w *Writer) WriteVector2s(v Vector2s) {
	w.WriteInt16(v.X)
	w.WriteInt16(v.Y)
}

// ==================================================================================== //
// Reader
// ==================================================================================== //

// Reader decodes a ZPackage. Errors are sticky: check Err after decoding a
// whole message rather than after every field.
type Reader struct {
	buf []byte
	pos int
	err error
}

// NewReader returns a Reader over buf. buf is not copied.
func NewReader(buf []byte) *Reader { return &Reader{buf: buf} }

// Err returns the first error encountered, if any.
func (r *Reader) Err() error { return r.err }

// Pos returns the current read offset.
func (r *Reader) Pos() int { return r.pos }

// SetPos seeks to off.
func (r *Reader) SetPos(off int) {
	if off < 0 || off > len(r.buf) {
		r.fail(ErrOverflow)
		return
	}
	r.pos = off
}

// Size returns the total length of the package.
func (r *Reader) Size() int { return len(r.buf) }

// Remaining returns the number of unread bytes.
func (r *Reader) Remaining() int { return len(r.buf) - r.pos }

func (r *Reader) fail(err error) {
	if r.err == nil {
		r.err = err
	}
}

// take advances the cursor by n and returns the consumed slice, or nil if the
// read would overflow or an error is already latched.
func (r *Reader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || r.pos+n > len(r.buf) {
		r.fail(fmt.Errorf("%w: want %d bytes at offset %d, have %d", ErrOverflow, n, r.pos, len(r.buf)-r.pos))
		return nil
	}
	b := r.buf[r.pos : r.pos+n]
	r.pos += n
	return b
}

func (r *Reader) ReadUInt8() byte {
	b := r.take(1)
	if b == nil {
		return 0
	}
	return b[0]
}

func (r *Reader) ReadInt8() int8 { return int8(r.ReadUInt8()) }

func (r *Reader) ReadBool() bool { return r.ReadUInt8() != 0 }

func (r *Reader) ReadInt16() int16 { return int16(r.ReadUInt16()) }

func (r *Reader) ReadUInt16() uint16 {
	b := r.take(2)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint16(b)
}

func (r *Reader) ReadInt32() int32 { return int32(r.ReadUInt32()) }

func (r *Reader) ReadUInt32() uint32 {
	b := r.take(4)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint32(b)
}

func (r *Reader) ReadInt64() int64 { return int64(r.ReadUInt64()) }

func (r *Reader) ReadUInt64() uint64 {
	b := r.take(8)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint64(b)
}

func (r *Reader) ReadSingle() float32 { return math.Float32frombits(r.ReadUInt32()) }

func (r *Reader) ReadDouble() float64 { return math.Float64frombits(r.ReadUInt64()) }

// ReadChar reads a .NET char, which BinaryReader decodes as UTF-8.
func (r *Reader) ReadChar() rune {
	if r.err != nil {
		return 0
	}
	if r.pos >= len(r.buf) {
		r.fail(fmt.Errorf("%w: want a char at offset %d", ErrOverflow, r.pos))
		return 0
	}
	ru, size := utf8.DecodeRune(r.buf[r.pos:])
	if ru == utf8.RuneError && size <= 1 {
		r.fail(fmt.Errorf("zpackage: invalid utf-8 char at offset %d", r.pos))
		return 0
	}
	r.pos += size
	return ru
}

// ReadString reads a .NET string: 7-bit encoded byte length, then UTF-8 bytes.
func (r *Reader) ReadString() string {
	n := r.read7BitEncodedInt()
	if r.err != nil {
		return ""
	}
	b := r.take(int(n))
	if b == nil {
		return ""
	}
	return string(b)
}

// read7BitEncodedInt mirrors BinaryReader.Read7BitEncodedInt, which rejects
// prefixes longer than five bytes.
func (r *Reader) read7BitEncodedInt() uint32 {
	var v uint32
	var shift uint
	for i := 0; i < 5; i++ {
		b := r.ReadUInt8()
		if r.err != nil {
			return 0
		}
		v |= uint32(b&0x7F) << shift
		if b&0x80 == 0 {
			return v
		}
		shift += 7
	}
	r.fail(errors.New("zpackage: malformed 7-bit encoded length prefix"))
	return 0
}

// ReadByteArray reads an int32-length-prefixed byte slice. The result aliases the
// Reader's buffer.
func (r *Reader) ReadByteArray() []byte {
	n := r.ReadInt32()
	if r.err != nil {
		return nil
	}
	return r.take(int(n))
}

// ReadRawBytes reads exactly n bytes with no length prefix.
func (r *Reader) ReadRawBytes(n int) []byte { return r.take(n) }

// ReadPackage reads a nested length-prefixed package.
func (r *Reader) ReadPackage() *Reader {
	b := r.ReadByteArray()
	if b == nil {
		return NewReader(nil)
	}
	return NewReader(b)
}

// ReadCompressedPackage reads a gzip-compressed nested package.
func (r *Reader) ReadCompressedPackage() *Reader {
	b := r.ReadByteArray()
	if b == nil {
		return NewReader(nil)
	}
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		r.fail(fmt.Errorf("zpackage: gzip header: %w", err))
		return NewReader(nil)
	}
	defer zr.Close()
	out, err := io.ReadAll(zr)
	if err != nil {
		r.fail(fmt.Errorf("zpackage: gzip body: %w", err))
		return NewReader(nil)
	}
	return NewReader(out)
}

// ReadNumItems reads Valheim's compact count encoding. See Writer.WriteNumItems.
func (r *Reader) ReadNumItems() int {
	n := int(r.ReadUInt8())
	if r.err != nil {
		return 0
	}
	if n&0x80 != 0 {
		n = ((n & 0x7F) << 8) | int(r.ReadUInt8())
	}
	return n
}

func (r *Reader) ReadZDOID() ZDOID {
	return ZDOID{UserID: r.ReadInt64(), ID: r.ReadUInt32()}
}

func (r *Reader) ReadVector3() Vector3 {
	return Vector3{X: r.ReadSingle(), Y: r.ReadSingle(), Z: r.ReadSingle()}
}

func (r *Reader) ReadQuaternion() Quaternion {
	return Quaternion{X: r.ReadSingle(), Y: r.ReadSingle(), Z: r.ReadSingle(), W: r.ReadSingle()}
}

func (r *Reader) ReadVector2i() Vector2i {
	return Vector2i{X: r.ReadInt32(), Y: r.ReadInt32()}
}

func (r *Reader) ReadVector2s() Vector2s {
	return Vector2s{X: r.ReadInt16(), Y: r.ReadInt16()}
}

// StableHashCode reproduces Valheim's String.GetStableHashCode extension
// (assembly_utils/StringExtensionMethods.cs), which names prefabs on the wire
// and in save files.
//
// It is a djb2 variant running two interleaved accumulators over the string's
// UTF-16 code units, even indices into one, odd into the other, combined at
// the end. Three details are load-bearing and easy to get wrong:
//
//   - It iterates UTF-16 code units, not bytes and not runes. Characters
//     outside the BMP occupy two units and are hashed as their surrogate pair.
//   - All arithmetic is int32 with silent wraparound, including the final
//     multiply.
//   - A NUL code unit terminates the loop early, so hashing stops at the first
//     zero rather than running to the end of the string.
func StableHashCode(s string) int32 {
	units := utf16.Encode([]rune(s))

	num := int32(5381)
	num2 := num

	for i := 0; i < len(units) && units[i] != 0; i += 2 {
		num = ((num << 5) + num) ^ int32(units[i])
		if i == len(units)-1 || units[i+1] == 0 {
			break
		}
		num2 = ((num2 << 5) + num2) ^ int32(units[i+1])
	}

	return num + num2*1566083941
}
