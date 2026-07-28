package handeval

import (
	_ "embed"
	"encoding/binary"
	"fmt"

	"gopkg.aoctech.app/poker/api/internal/engine/handeval/hashq"
)

// tablesBlob is produced by `go generate ./internal/engine/handeval/...`.
// Embedding it keeps startup to a single ~120 KB decode instead of the
// multi-second enumeration that building these tables from scratch costs.
//
//go:embed tables.bin
var tablesBlob []byte

const tablesMagic = "PHE1"

var (
	// flushTable is indexed by the 13-bit rank mask of a suit holding 5+ cards.
	flushTable []uint16
	// noFlushTable is indexed by hashq.Hash of the hand's rank multiset.
	noFlushTable []uint16
	// categoryTable is indexed by Score and yields its Category.
	categoryTable []byte
)

func init() {
	if err := loadTables(tablesBlob); err != nil {
		// A corrupt or stale table silently mis-ranks every showdown, so this
		// has to be fatal at process start rather than a wrong answer later.
		panic("handeval: " + err.Error())
	}
}

func loadTables(blob []byte) error {
	const headerLen = len(tablesMagic) + 3*4
	if len(blob) < headerLen || string(blob[:len(tablesMagic)]) != tablesMagic {
		return fmt.Errorf("tables.bin is missing or not a %s blob — run go generate ./internal/engine/handeval/", tablesMagic)
	}
	header := blob[len(tablesMagic):headerLen]
	nFlush := int(binary.LittleEndian.Uint32(header[0:4]))
	nNoFlush := int(binary.LittleEndian.Uint32(header[4:8]))
	nCategory := int(binary.LittleEndian.Uint32(header[8:12]))

	if nFlush != 1<<hashq.Cards || nNoFlush != hashq.Size {
		return fmt.Errorf("tables.bin has %d flush / %d non-flush entries, want %d / %d",
			nFlush, nNoFlush, 1<<hashq.Cards, hashq.Size)
	}
	want := headerLen + 2*nFlush + 2*nNoFlush + nCategory
	if len(blob) != want {
		return fmt.Errorf("tables.bin is %d bytes, want %d", len(blob), want)
	}

	body := blob[headerLen:]
	flushTable = decodeU16(body[:2*nFlush])
	noFlushTable = decodeU16(body[2*nFlush : 2*nFlush+2*nNoFlush])
	categoryTable = body[2*nFlush+2*nNoFlush:]
	return nil
}

func decodeU16(b []byte) []uint16 {
	out := make([]uint16, len(b)/2)
	for i := range out {
		out[i] = binary.LittleEndian.Uint16(b[2*i:])
	}
	return out
}
