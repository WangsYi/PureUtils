//go:build windows

package collector

import (
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modIPHlpAPI      = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetIfTable2  = modIPHlpAPI.NewProc("GetIfTable2")
	procFreeMibTable = modIPHlpAPI.NewProc("FreeMibTable")
)

type mibIfTable2 struct {
	NumEntries uint32
	_          uint32
	Table      [1]windows.MibIfRow2
}

type netSampler struct {
	prevIn   map[uint64]uint64
	prevOut  map[uint64]uint64
	prevTime time.Time
}

func newNetSampler() *netSampler {
	return &netSampler{
		prevIn:  make(map[uint64]uint64),
		prevOut: make(map[uint64]uint64),
	}
}

func (n *netSampler) sample(selectedAdapter string, now time.Time) (downBps, upBps float64, totalDown, totalUp uint64, activeName string) {
	if procGetIfTable2.Find() != nil || procFreeMibTable.Find() != nil {
		return 0, 0, 0, 0, ""
	}
	var tblPtr uintptr
	r, _, _ := procGetIfTable2.Call(uintptr(unsafe.Pointer(&tblPtr)))
	if r != 0 || tblPtr == 0 {
		return 0, 0, 0, 0, ""
	}
	defer procFreeMibTable.Call(tblPtr)

	tbl := (*mibIfTable2)(unsafe.Pointer(tblPtr))
	count := int(tbl.NumEntries)
	if count <= 0 || count > 2048 {
		return 0, 0, 0, 0, ""
	}
	rows := unsafe.Slice(&tbl.Table[0], count)

	dt := now.Sub(n.prevTime).Seconds()
	sel := strings.TrimSpace(selectedAdapter)
	autoSel := sel == "" || strings.EqualFold(sel, "auto")

	var bestActive string
	var bestTraffic float64
	for i := range rows {
		row := &rows[i]
		if row.Type == 24 || row.OperStatus != 1 { // skip loopback and non-up
			continue
		}
		if row.InterfaceAndOperStatusFlags&0x01 == 0 { // HardwareInterface flag bit 0? Actually filter virtual by desc
			// keep checking desc; some virtual/bridge NICs carry traffic, physical adapters preferred
		}
		desc := strings.TrimSpace(windows.UTF16ToString(row.Description[:]))
		alias := strings.TrimSpace(windows.UTF16ToString(row.Alias[:]))
		if isVirtualAdapterDesc(desc) {
			continue
		}
		// Skip NDIS filter / lightweight filter layers (they duplicate parent NIC bytes!)
		// HardwareInterface bit 0 in InterfaceAndOperStatusFlags = 1 for real miniports.
		if row.InterfaceAndOperStatusFlags&0x01 == 0 {
			continue
		}
		if !autoSel && !strings.EqualFold(alias, sel) && !strings.EqualFold(desc, sel) {
			continue
		}

		luid := row.InterfaceLuid
		inB := row.InOctets
		outB := row.OutOctets
		totalDown += inB
		totalUp += outB

		if dt > 0.05 {
			if pIn, ok := n.prevIn[luid]; ok && inB >= pIn {
				d := float64(inB-pIn) / dt
				downBps += d
				if d > bestTraffic {
					bestTraffic = d
					bestActive = alias
				}
			}
			if pOut, ok := n.prevOut[luid]; ok && outB >= pOut {
				u := float64(outB-pOut) / dt
				upBps += u
				if u > bestTraffic {
					bestTraffic = u
					bestActive = alias
				}
			}
		}
		if bestActive == "" {
			bestActive = alias
		}
		n.prevIn[luid] = inB
		n.prevOut[luid] = outB
	}
	n.prevTime = now
	return downBps, upBps, totalDown, totalUp, bestActive
}
