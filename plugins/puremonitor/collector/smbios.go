package collector

import (
	"encoding/binary"
	"strings"
)

// SMBIOSData holds parsed Type 0, 1, 2, 4, and 17 information.
type SMBIOSData struct {
	System      SystemInfo
	Motherboard MotherboardInfo
	Socket      string
	CPUVendor   string
	CPUVersion  string
	CPUMaxMHz   int
	RAMModules  []RAMModuleInfo
	TotalRAMMB  uint64
}

// ParseRawSMBIOS parses a Windows RawSMBIOSData buffer returned by
// GetSystemFirmwareTable('RSMB', 0, ...).
func ParseRawSMBIOS(raw []byte) SMBIOSData {
	if len(raw) < 8 {
		return SMBIOSData{}
	}
	tableLen := int(binary.LittleEndian.Uint32(raw[4:8]))
	table := raw[8:]
	if tableLen > 0 && tableLen < len(table) {
		table = table[:tableLen]
	}
	return ParseSMBIOSTable(table)
}

// ParseSMBIOSTable parses raw SMBIOS structures without the 8-byte Windows header.
func ParseSMBIOSTable(table []byte) SMBIOSData {
	var out SMBIOSData
	pos := 0
	for pos+4 <= len(table) {
		stype := table[pos]
		slen := int(table[pos+1])
		if slen < 4 || pos+slen > len(table) {
			break
		}
		formatted := table[pos : pos+slen]
		stringsStart := pos + slen
		stringsEnd := findSMBIOSStringsEnd(table, stringsStart)
		if stringsEnd < 0 {
			break
		}
		strs := parseSMBIOSStrings(table[stringsStart:stringsEnd])

		switch stype {
		case 0: // BIOS Information
			if len(formatted) > 0x04 {
				out.Motherboard.BIOSVendor = smbiosStr(strs, formatted[0x04])
			}
			if len(formatted) > 0x05 {
				out.Motherboard.BIOSVersion = smbiosStr(strs, formatted[0x05])
			}
			if len(formatted) > 0x08 {
				out.Motherboard.BIOSDate = smbiosStr(strs, formatted[0x08])
			}
		case 1: // System Information
			if len(formatted) > 0x04 {
				out.System.Manufacturer = smbiosStr(strs, formatted[0x04])
			}
			if len(formatted) > 0x05 {
				out.System.Model = smbiosStr(strs, formatted[0x05])
			}
		case 2: // Baseboard Information
			if len(formatted) > 0x04 {
				out.Motherboard.Manufacturer = smbiosStr(strs, formatted[0x04])
			}
			if len(formatted) > 0x05 {
				out.Motherboard.Product = smbiosStr(strs, formatted[0x05])
			}
		case 4: // Processor Information
			if len(formatted) > 0x04 && out.Socket == "" {
				out.Socket = smbiosStr(strs, formatted[0x04])
			}
			if len(formatted) > 0x07 && out.CPUVendor == "" {
				out.CPUVendor = smbiosStr(strs, formatted[0x07])
			}
			if len(formatted) > 0x10 && out.CPUVersion == "" {
				out.CPUVersion = smbiosStr(strs, formatted[0x10])
			}
			if len(formatted) >= 0x16 && out.CPUMaxMHz == 0 {
				out.CPUMaxMHz = int(binary.LittleEndian.Uint16(formatted[0x14:0x16]))
			}
		case 17: // Memory Device
			if mod, ok := parseSMBIOSType17(formatted, strs); ok {
				out.RAMModules = append(out.RAMModules, mod)
				out.TotalRAMMB += mod.CapacityMB
			}
		case 127: // End-of-table
			return out
		}

		pos = stringsEnd
	}
	return out
}

func findSMBIOSStringsEnd(buf []byte, start int) int {
	if start+2 > len(buf) {
		return -1
	}
	for i := start; i+1 < len(buf); i++ {
		if buf[i] == 0 && buf[i+1] == 0 {
			return i + 2
		}
	}
	return -1
}

func parseSMBIOSStrings(raw []byte) []string {
	if len(raw) <= 2 && len(raw) > 0 && raw[0] == 0 {
		return nil
	}
	var out []string
	cur := 0
	for i := 0; i < len(raw); i++ {
		if raw[i] == 0 {
			if i == cur {
				break
			}
			out = append(out, cleanSMBIOSString(string(raw[cur:i])))
			cur = i + 1
		}
	}
	return out
}

func smbiosStr(strs []string, idx byte) string {
	if idx == 0 {
		return ""
	}
	pos := int(idx) - 1
	if pos < 0 || pos >= len(strs) {
		return ""
	}
	return strs[pos]
}

func cleanSMBIOSString(s string) string {
	s = strings.TrimSpace(s)
	switch strings.ToLower(s) {
	case "to be filled by o.e.m.", "default string", "not specified", "unknown", "none", "n/a":
		return ""
	default:
		return s
	}
}

func parseSMBIOSType17(formatted []byte, strs []string) (RAMModuleInfo, bool) {
	if len(formatted) < 0x15 {
		return RAMModuleInfo{}, false
	}
	rawSize := binary.LittleEndian.Uint16(formatted[0x0C:0x0E])
	if rawSize == 0 || rawSize == 0xFFFF {
		return RAMModuleInfo{}, false
	}
	var capMB uint64
	if rawSize == 0x7FFF {
		if len(formatted) >= 0x20 {
			ext := binary.LittleEndian.Uint32(formatted[0x1C:0x20]) & 0x7FFFFFFF
			capMB = uint64(ext)
		}
	} else if rawSize&0x8000 != 0 {
		// Bit 15 set => value is in KiB.
		kb := uint64(rawSize & 0x7FFF)
		capMB = kb / 1024
	} else {
		capMB = uint64(rawSize)
	}
	if capMB == 0 {
		return RAMModuleInfo{}, false
	}

	var slot, bank, mfg, part string
	if len(formatted) > 0x10 {
		slot = smbiosStr(strs, formatted[0x10])
	}
	if len(formatted) > 0x11 {
		bank = smbiosStr(strs, formatted[0x11])
	}
	var memType byte
	if len(formatted) > 0x12 {
		memType = formatted[0x12]
	}
	var speedMTs int
	if len(formatted) >= 0x17 {
		speedMTs = int(binary.LittleEndian.Uint16(formatted[0x15:0x17]))
	}
	if len(formatted) > 0x17 {
		mfg = smbiosStr(strs, formatted[0x17])
	}
	if len(formatted) > 0x1A {
		part = smbiosStr(strs, formatted[0x1A])
	}
	if len(formatted) >= 0x22 {
		cfgSpeed := int(binary.LittleEndian.Uint16(formatted[0x20:0x22]))
		if cfgSpeed > 0 && cfgSpeed != 0xFFFF {
			speedMTs = cfgSpeed
		}
	}

	return RAMModuleInfo{
		Slot:         slot,
		Bank:         bank,
		Manufacturer: mfg,
		PartNumber:   part,
		Type:         smbiosMemoryTypeName(memType),
		CapacityMB:   capMB,
		SpeedMTs:     speedMTs,
	}, true
}

func smbiosMemoryTypeName(code byte) string {
	switch code {
	case 0x12:
		return "DDR"
	case 0x13:
		return "DDR2"
	case 0x18:
		return "DDR3"
	case 0x1A:
		return "DDR4"
	case 0x1B:
		return "LPDDR"
	case 0x1C:
		return "LPDDR2"
	case 0x1D:
		return "LPDDR3"
	case 0x1E:
		return "LPDDR4"
	case 0x22:
		return "DDR5"
	case 0x23:
		return "LPDDR5"
	default:
		return "DRAM"
	}
}
