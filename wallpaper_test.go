package remotecontrol_test

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/xconnio/remotecontrol-app"
)

// A dconf database holding /k = 'x': a root item "/" and its child "k".
func TestGvdbLookup(t *testing.T) {
	data := make([]byte, 80)
	copy(data, "GVariant")
	binary.LittleEndian.PutUint32(data[16:], 24) // root table start
	binary.LittleEndian.PutUint32(data[20:], 80) // root table end, no buckets
	item := func(off, parent, keyStart, keyLen int, typ byte, valStart, valEnd int) {
		binary.LittleEndian.PutUint32(data[off+4:], uint32(parent))
		binary.LittleEndian.PutUint32(data[off+8:], uint32(keyStart))
		binary.LittleEndian.PutUint16(data[off+12:], uint16(keyLen))
		data[off+14] = typ
		binary.LittleEndian.PutUint32(data[off+16:], uint32(valStart))
		binary.LittleEndian.PutUint32(data[off+20:], uint32(valEnd))
	}
	item(32, 0xffffffff, 80, 1, 'L', 0, 0)
	item(56, 0, 81, 1, 'v', 82, 86)
	data = append(data, "/kx\x00\x00s"...)

	require.Equal(t, "x", remotecontrol.GvdbLookup(data, "/k"))
}
