package collector

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func buildSMBIOSStruct(stype byte, formatted []byte, strs ...string) []byte {
	var b bytes.Buffer
	b.WriteByte(stype)
	b.WriteByte(byte(len(formatted)))
	// Caller fills Handle at bytes 2..3 inside formatted? No, formatted starts at byte 0 of struct.
	// Use formatted[4:] if caller provides full header, or prepend handle:
	b.Reset()
	b.Write(formatted)
	if len(strs) == 0 {
		b.Write([]byte{0, 0})
		return b.Bytes()
	}
	for _, s := range strs {
		b.WriteString(s)
		b.WriteByte(0)
	}
	b.WriteByte(0)
	return b.Bytes()
}

func wrapRawSMBIOS(table []byte) []byte {
	hdr := make([]byte, 8)
	hdr[1] = 3
	hdr[2] = 4
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(len(table)))
	return append(hdr, table...)
}

func TestParseRawSMBIOSAllTypesAndExtendedRAM(t *testing.T) {
	// Type 0: BIOS
	t0 := make([]byte, 0x18)
	t0[0] = 0
	t0[1] = byte(len(t0))
	t0[0x04] = 1 // BIOS Vendor
	t0[0x05] = 2 // BIOS Version
	t0[0x08] = 3 // BIOS Date
	s0 := buildSMBIOSStruct(0, t0, "American Megatrends Inc.", "F28", "08/12/2025")

	// Type 1: System
	t1 := make([]byte, 0x1B)
	t1[0] = 1
	t1[1] = byte(len(t1))
	t1[0x04] = 1
	t1[0x05] = 2
	s1 := buildSMBIOSStruct(1, t1, "ASUS", "ROG Maximus Z790")

	// Type 2: Baseboard
	t2 := make([]byte, 0x0F)
	t2[0] = 2
	t2[1] = byte(len(t2))
	t2[0x04] = 1
	t2[0x05] = 2
	s2 := buildSMBIOSStruct(2, t2, "ASUSTeK COMPUTER INC.", "ROG MAXIMUS Z790 HERO")

	// Type 4: Processor
	t4 := make([]byte, 0x20)
	t4[0] = 4
	t4[1] = byte(len(t4))
	t4[0x04] = 1 // Socket
	t4[0x07] = 2 // Vendor
	t4[0x10] = 3 // Version
	binary.LittleEndian.PutUint16(t4[0x14:0x16], 5800)
	s4 := buildSMBIOSStruct(4, t4, "LGA1700", "Intel(R) Corporation", "Intel(R) Core(TM) i9-14900K")

	// Type 17 Slot A: 16GB DDR5 6000 MT/s
	t17a := make([]byte, 0x28)
	t17a[0] = 17
	t17a[1] = byte(len(t17a))
	binary.LittleEndian.PutUint16(t17a[0x0C:0x0E], 16384)
	t17a[0x10] = 1    // DeviceLocator
	t17a[0x11] = 2    // BankLocator
	t17a[0x12] = 0x22 // DDR5
	binary.LittleEndian.PutUint16(t17a[0x15:0x17], 4800)
	t17a[0x17] = 3 // Manufacturer
	t17a[0x1A] = 4 // PartNumber
	binary.LittleEndian.PutUint16(t17a[0x20:0x22], 6000) // Configured speed
	s17a := buildSMBIOSStruct(17, t17a, "DIMM_A2", "BANK 0", "G.Skill", "F5-6000J3038F16G")

	// Type 17 Slot B: empty slot (size == 0) should be skipped
	t17Empty := make([]byte, 0x28)
	t17Empty[0] = 17
	t17Empty[1] = byte(len(t17Empty))
	binary.LittleEndian.PutUint16(t17Empty[0x0C:0x0E], 0)
	s17Empty := buildSMBIOSStruct(17, t17Empty, "DIMM_B1")

	// Type 17 Slot C: 64GB DDR5 using ExtendedSize (rawSize == 0x7FFF)
	t17Ext := make([]byte, 0x28)
	t17Ext[0] = 17
	t17Ext[1] = byte(len(t17Ext))
	binary.LittleEndian.PutUint16(t17Ext[0x0C:0x0E], 0x7FFF)
	t17Ext[0x10] = 1
	t17Ext[0x12] = 0x22 // DDR5
	binary.LittleEndian.PutUint32(t17Ext[0x1C:0x20], 65536)
	binary.LittleEndian.PutUint16(t17Ext[0x20:0x22], 5600)
	s17Ext := buildSMBIOSStruct(17, t17Ext, "DIMM_B2")

	// Type 127: End
	t127 := []byte{127, 4, 0, 0}
	s127 := buildSMBIOSStruct(127, t127)

	var table []byte
	for _, s := range [][]byte{s0, s1, s2, s4, s17a, s17Empty, s17Ext, s127} {
		table = append(table, s...)
	}

	got := ParseRawSMBIOS(wrapRawSMBIOS(table))
	if got.Motherboard.BIOSVendor != "American Megatrends Inc." || got.Motherboard.BIOSVersion != "F28" {
		t.Fatalf("BIOS = %+v", got.Motherboard)
	}
	if got.System.Manufacturer != "ASUS" || got.System.Model != "ROG Maximus Z790" {
		t.Fatalf("System = %+v", got.System)
	}
	if got.Motherboard.Product != "ROG MAXIMUS Z790 HERO" {
		t.Fatalf("Motherboard = %+v", got.Motherboard)
	}
	if got.Socket != "LGA1700" || got.CPUMaxMHz != 5800 {
		t.Fatalf("CPU = socket=%q maxMHz=%d", got.Socket, got.CPUMaxMHz)
	}
	if len(got.RAMModules) != 2 {
		t.Fatalf("RAMModules len = %d, want 2: %+v", len(got.RAMModules), got.RAMModules)
	}
	if got.RAMModules[0].CapacityMB != 16384 || got.RAMModules[0].Type != "DDR5" || got.RAMModules[0].SpeedMTs != 6000 {
		t.Fatalf("RAMModules[0] = %+v", got.RAMModules[0])
	}
	if got.RAMModules[1].CapacityMB != 65536 || got.RAMModules[1].SpeedMTs != 5600 {
		t.Fatalf("RAMModules[1] = %+v", got.RAMModules[1])
	}
	if got.TotalRAMMB != 16384+65536 {
		t.Fatalf("TotalRAMMB = %d, want %d", got.TotalRAMMB, 16384+65536)
	}
}

func TestParseRawSMBIOSTruncatedSafe(t *testing.T) {
	for _, buf := range [][]byte{
		nil,
		{1, 2, 3},
		{0, 3, 4, 0, 100, 0, 0, 0, 17, 2, 0, 0},
		{0, 3, 4, 0, 10, 0, 0, 0, 17, 10, 0, 0, 1, 2, 3},
	} {
		_ = ParseRawSMBIOS(buf)
	}
}

func TestSMBIOSMemoryTypesPlaceholdersAndKiBSize(t *testing.T) {
	cases := map[byte]string{
		0x12: "DDR",
		0x13: "DDR2",
		0x18: "DDR3",
		0x1A: "DDR4",
		0x1B: "LPDDR",
		0x1C: "LPDDR2",
		0x1D: "LPDDR3",
		0x1E: "LPDDR4",
		0x22: "DDR5",
		0x23: "LPDDR5",
		0xFF: "DRAM",
	}
	for code, want := range cases {
		if got := smbiosMemoryTypeName(code); got != want {
			t.Fatalf("smbiosMemoryTypeName(%#x) = %q, want %q", code, got, want)
		}
	}

	for _, ph := range []string{"To Be Filled By O.E.M.", "Default string", "Not Specified", "Unknown", "None", "N/A"} {
		if got := cleanSMBIOSString(ph); got != "" {
			t.Fatalf("cleanSMBIOSString(%q) = %q, want empty", ph, got)
		}
	}
	if got := cleanSMBIOSString(" Corsair "); got != "Corsair" {
		t.Fatalf("cleanSMBIOSString = %q", got)
	}

	if got := findSMBIOSStringsEnd([]byte{1, 2}, 0); got != -1 {
		t.Fatalf("short strings end = %d, want -1", got)
	}
	if got := findSMBIOSStringsEnd([]byte{1, 2, 0, 3, 4}, 0); got != -1 {
		t.Fatalf("unterminated strings end = %d, want -1", got)
	}

	// KiB unit flag (0x8000 | 4096 = 4 MB) and fallback base speed without configured speed bytes.
	t17KiB := make([]byte, 0x1B)
	t17KiB[0] = 17
	t17KiB[1] = byte(len(t17KiB))
	binary.LittleEndian.PutUint16(t17KiB[0x0C:0x0E], 0x8000|4096)
	t17KiB[0x10] = 1
	t17KiB[0x12] = 0x1A // DDR4
	binary.LittleEndian.PutUint16(t17KiB[0x15:0x17], 3200)
	t17KiB[0x17] = 2 // Manufacturer placeholder -> cleaned
	t17KiB[0x1A] = 3
	mod, ok := parseSMBIOSType17(t17KiB, []string{"DIMM_A1", "Default string", "CMK16GX4"})
	if !ok || mod.CapacityMB != 4 || mod.Type != "DDR4" || mod.SpeedMTs != 3200 || mod.PartNumber != "CMK16GX4" {
		t.Fatalf("parseSMBIOSType17 KiB = (%+v, %v)", mod, ok)
	}
	if mod.Slot != "DIMM_A1" || smbiosStr([]string{"A"}, 0) != "" || smbiosStr([]string{"A"}, 5) != "" {
		t.Fatalf("unexpected string indexing result: %+v", mod)
	}
}
