package zpackage

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
)

// The golden bytes below were produced by running the equivalent
// System.IO.BinaryWriter calls under .NET 9 and dumping the resulting stream.
// They are the ground truth for the wire format: if a change here breaks these,
// the change is wrong, not the goldens.
func TestWriterMatchesDotNetBinaryWriter(t *testing.T) {
	tests := []struct {
		name  string
		write func(*Writer)
		want  string
	}{
		{"string_empty", func(w *Writer) { w.WriteString("") }, "00"},
		{"string_abc", func(w *Writer) { w.WriteString("abc") }, "03616263"},

		// Length prefix counts UTF-8 *bytes*, not runes: 6 runes, 9 bytes.
		{"string_utf8", func(w *Writer) { w.WriteString("naïve☃") }, "096E61C3AF7665E29883"},

		// The 7-bit prefix rolls over to two bytes at 128.
		{"string_len127", func(w *Writer) { w.WriteString(strings.Repeat("x", 127)) },
			"7F" + strings.Repeat("78", 127)},
		{"string_len128", func(w *Writer) { w.WriteString(strings.Repeat("x", 128)) },
			"8001" + strings.Repeat("78", 128)},
		{"string_len300", func(w *Writer) { w.WriteString(strings.Repeat("x", 300)) },
			"AC02" + strings.Repeat("78", 300)},

		{"bool_true", func(w *Writer) { w.WriteBool(true) }, "01"},
		{"char_a", func(w *Writer) { w.WriteChar('a') }, "61"},
		{"char_snowman", func(w *Writer) { w.WriteChar('☃') }, "E29883"},
		{"int32_neg2", func(w *Writer) { w.WriteInt32(-2) }, "FEFFFFFF"},
		{"uint32_36", func(w *Writer) { w.WriteUInt32(36) }, "24000000"},
		{"int64_min", func(w *Writer) { w.WriteInt64(-1 << 63) }, "0000000000000080"},
		{"single_pi", func(w *Writer) { w.WriteSingle(3.14159) }, "D00F4940"},
		{"double_pi", func(w *Writer) { w.WriteDouble(3.141592653589793) }, "182D4454FB210940"},
		{"int16_neg300", func(w *Writer) { w.WriteInt16(-300) }, "D4FE"},
		{"bytearray_3", func(w *Writer) { w.WriteByteArray([]byte{1, 2, 3}) }, "03000000010203"},
		{"zdoid", func(w *Writer) { w.WriteZDOID(ZDOID{UserID: 123456789, ID: 42}) },
			"15CD5B07000000002A000000"},
		{"vector3", func(w *Writer) { w.WriteVector3(Vector3{X: 1.5, Y: -2.25, Z: 0}) },
			"0000C03F000010C000000000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := NewWriter()
			tt.write(w)
			got := strings.ToUpper(hex.EncodeToString(w.Bytes()))
			if got != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}

func TestReaderRoundTrip(t *testing.T) {
	w := NewWriter()
	w.WriteUInt8(0xAB)
	w.WriteInt8(-5)
	w.WriteBool(true)
	w.WriteBool(false)
	w.WriteChar('☃')
	w.WriteInt16(-300)
	w.WriteUInt16(65535)
	w.WriteInt32(-2)
	w.WriteUInt32(36)
	w.WriteInt64(-1 << 63)
	w.WriteUInt64(1 << 63)
	w.WriteSingle(3.14159)
	w.WriteDouble(3.141592653589793)
	w.WriteString("naïve☃")
	w.WriteByteArray([]byte{1, 2, 3})
	w.WriteZDOID(ZDOID{UserID: 123456789, ID: 42})
	w.WriteVector3(Vector3{X: 1.5, Y: -2.25, Z: 0})
	w.WriteQuaternion(Quaternion{X: 0, Y: 0.707, Z: 0, W: 0.707})
	w.WriteVector2i(Vector2i{X: -7, Y: 9})
	w.WriteVector2s(Vector2s{X: -7, Y: 9})

	r := NewReader(w.Bytes())
	if got := r.ReadUInt8(); got != 0xAB {
		t.Errorf("Byte = %#x", got)
	}
	if got := r.ReadInt8(); got != -5 {
		t.Errorf("SByte = %d", got)
	}
	if got := r.ReadBool(); !got {
		t.Errorf("Bool = %v, want true", got)
	}
	if got := r.ReadBool(); got {
		t.Errorf("Bool = %v, want false", got)
	}
	if got := r.ReadChar(); got != '☃' {
		t.Errorf("Char = %q", got)
	}
	if got := r.ReadInt16(); got != -300 {
		t.Errorf("Int16 = %d", got)
	}
	if got := r.ReadUInt16(); got != 65535 {
		t.Errorf("UInt16 = %d", got)
	}
	if got := r.ReadInt32(); got != -2 {
		t.Errorf("Int32 = %d", got)
	}
	if got := r.ReadUInt32(); got != 36 {
		t.Errorf("UInt32 = %d", got)
	}
	if got := r.ReadInt64(); got != -1<<63 {
		t.Errorf("Int64 = %d", got)
	}
	if got := r.ReadUInt64(); got != 1<<63 {
		t.Errorf("UInt64 = %d", got)
	}
	if got := r.ReadSingle(); got != 3.14159 {
		t.Errorf("Single = %v", got)
	}
	if got := r.ReadDouble(); got != 3.141592653589793 {
		t.Errorf("Double = %v", got)
	}
	if got := r.ReadString(); got != "naïve☃" {
		t.Errorf("String = %q", got)
	}
	if got := r.ReadByteArray(); !bytes.Equal(got, []byte{1, 2, 3}) {
		t.Errorf("ByteArray = %v", got)
	}
	if got := (r.ReadZDOID()); got != (ZDOID{UserID: 123456789, ID: 42}) {
		t.Errorf("ZDOID = %v", got)
	}
	if got := r.ReadVector3(); got != (Vector3{X: 1.5, Y: -2.25, Z: 0}) {
		t.Errorf("Vector3 = %v", got)
	}
	if got := r.ReadQuaternion(); got != (Quaternion{X: 0, Y: 0.707, Z: 0, W: 0.707}) {
		t.Errorf("Quaternion = %v", got)
	}
	if got := r.ReadVector2i(); got != (Vector2i{X: -7, Y: 9}) {
		t.Errorf("Vector2i = %v", got)
	}
	if got := r.ReadVector2s(); got != (Vector2s{X: -7, Y: 9}) {
		t.Errorf("Vector2s = %v", got)
	}

	if err := r.Err(); err != nil {
		t.Fatalf("Err = %v", err)
	}
	if r.Remaining() != 0 {
		t.Errorf("Remaining = %d, want 0", r.Remaining())
	}
}

// NumItems is Valheim's own count encoding and is deliberately NOT the same as
// the 7-bit string prefix. Mixing the two up silently corrupts every list.
func TestNumItems(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "00"},
		{1, "01"},
		{127, "7F"},
		{128, "8080"},
		{255, "80FF"},
		{256, "8100"},
		{32767, "FFFF"},
	}

	for _, tt := range tests {
		w := NewWriter()
		w.WriteNumItems(tt.n)
		if got := strings.ToUpper(hex.EncodeToString(w.Bytes())); got != tt.want {
			t.Errorf("NumItems(%d) = %s, want %s", tt.n, got, tt.want)
		}
		if got := NewReader(w.Bytes()).ReadNumItems(); got != tt.n {
			t.Errorf("round trip NumItems(%d) = %d", tt.n, got)
		}
	}
}

// A 128-byte string and a 128-item count encode their length differently.
// This is the trap the two schemes set for each other.
func TestNumItemsDiffersFrom7BitPrefix(t *testing.T) {
	items := NewWriter()
	items.WriteNumItems(128)

	str := NewWriter()
	str.WriteString(strings.Repeat("x", 128))

	if bytes.Equal(items.Bytes(), str.Bytes()[:2]) {
		t.Fatal("NumItems and the 7-bit string prefix encode 128 identically; one of them is wrong")
	}
	if got := hex.EncodeToString(items.Bytes()); got != "8080" {
		t.Errorf("NumItems(128) = %s, want 8080", got)
	}
	if got := hex.EncodeToString(str.Bytes()[:2]); got != "8001" {
		t.Errorf("string prefix for 128 = %s, want 8001", got)
	}
}

func TestNestedPackage(t *testing.T) {
	inner := NewWriter()
	inner.WriteString("hello")
	inner.WriteInt32(7)

	outer := NewWriter()
	outer.WriteInt32(1)
	outer.WritePackage(inner)
	outer.WriteString("tail")

	r := NewReader(outer.Bytes())
	if got := r.ReadInt32(); got != 1 {
		t.Errorf("leading Int32 = %d", got)
	}
	in := r.ReadPackage()
	if got := in.ReadString(); got != "hello" {
		t.Errorf("inner String = %q", got)
	}
	if got := in.ReadInt32(); got != 7 {
		t.Errorf("inner Int32 = %d", got)
	}
	if got := r.ReadString(); got != "tail" {
		t.Errorf("trailing String = %q", got)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("Err = %v", err)
	}
}

func TestCompressedPackage(t *testing.T) {
	inner := NewWriter()
	for i := range 100 {
		inner.WriteInt32(int32(i))
	}

	outer := NewWriter()
	if err := outer.WriteCompressedPackage(inner); err != nil {
		t.Fatalf("CompressedPackage: %v", err)
	}

	r := NewReader(outer.Bytes())
	in := r.ReadCompressedPackage()
	if err := r.Err(); err != nil {
		t.Fatalf("Err = %v", err)
	}
	for i := range 100 {
		if got := in.ReadInt32(); got != int32(i) {
			t.Fatalf("inner[%d] = %d", i, got)
		}
	}
	if err := in.Err(); err != nil {
		t.Fatalf("inner Err = %v", err)
	}
}

func TestReaderOverflowIsSticky(t *testing.T) {
	r := NewReader([]byte{1, 2})

	if got := r.ReadInt32(); got != 0 {
		t.Errorf("Int32 past end = %d, want zero value", got)
	}
	if r.Err() == nil {
		t.Fatal("Err = nil after overflow")
	}
	first := r.Err()

	// Later reads must stay no-ops and must not overwrite the first error.
	if got := r.ReadString(); got != "" {
		t.Errorf("String after overflow = %q", got)
	}
	if r.Err() != first {
		t.Errorf("Err changed after a second failed read: %v", r.Err())
	}
}

func TestReaderRejectsMalformedLengthPrefix(t *testing.T) {
	// Five continuation bytes: BinaryReader gives up after five.
	r := NewReader([]byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x01})
	r.ReadString()
	if r.Err() == nil {
		t.Fatal("Err = nil for an over-long 7-bit prefix")
	}
}

func TestReaderStringLengthOverflow(t *testing.T) {
	// Claims 200 bytes but supplies 3.
	r := NewReader([]byte{0xC8, 0x01, 'a', 'b', 'c'})
	if got := r.ReadString(); got != "" {
		t.Errorf("String = %q, want empty", got)
	}
	if r.Err() == nil {
		t.Fatal("Err = nil for a truncated string")
	}
}

func TestZDOIDNone(t *testing.T) {
	if !None.IsNone() {
		t.Error("None.IsNone() = false")
	}
	if (ZDOID{UserID: 1}).IsNone() {
		t.Error("non-zero UserID reported as None")
	}
	if (ZDOID{ID: 1}).IsNone() {
		t.Error("non-zero ID reported as None")
	}
	if got := (ZDOID{UserID: 5, ID: 9}).String(); got != "5:9" {
		t.Errorf("String = %q, want 5:9", got)
	}
}
