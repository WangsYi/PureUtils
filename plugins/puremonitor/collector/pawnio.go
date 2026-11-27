package collector

import (
	"encoding/binary"
	"math"
	"strings"
)

const (
	pawnIOFnNameLength = 32

	// PawnIO device IOCTLs: CTL_CODE(41394 /* 0xA1B2 */, func, METHOD_BUFFERED, FILE_ANY_ACCESS)
	ioctlPawnIOLoadBinary = 0xA1B22084
	ioctlPawnIOExecuteFn  = 0xA1B22104

	// Intel MSR addresses
	msrIntelTempTarget   = 0x1A2
	msrIntelThermStatus  = 0x19C
	msrIntelPkgThermStat = 0x1B1
	msrIntelRAPLUnit     = 0x606
	msrIntelPkgEnergy    = 0x611

	// AMD Family 17h/19h/1Ah SMN & MSR addresses
	smnAMDZenCurTmp   = 0x00059800
	msrAMDRAPLUnit    = 0xC0010299
	msrAMDPkgEnergy   = 0xC001029B
)

// BuildPawnIOExecuteInput packs a 32-byte null-padded function name followed by uint64 little-endian args.
func BuildPawnIOExecuteInput(fnName string, args []uint64) []byte {
	buf := make([]byte, pawnIOFnNameLength+len(args)*8)
	nameBytes := []byte(strings.TrimSpace(fnName))
	if len(nameBytes) > pawnIOFnNameLength-1 {
		nameBytes = nameBytes[:pawnIOFnNameLength-1]
	}
	copy(buf[:pawnIOFnNameLength], nameBytes)
	for i, arg := range args {
		binary.LittleEndian.PutUint64(buf[pawnIOFnNameLength+i*8:], arg)
	}
	return buf
}

// DecodeIntelTjMax extracts TjMax from MSR_TEMPERATURE_TARGET (0x1A2) bits [23:16].
func DecodeIntelTjMax(msr1A2 uint64) float64 {
	tj := float64((msr1A2 >> 16) & 0xFF)
	if tj < 50 || tj > 125 {
		return 100
	}
	return tj
}

// DecodeIntelThermTemp extracts Celsius temperature from IA32_THERM_STATUS (0x19C) or IA32_PACKAGE_THERM_STATUS (0x1B1).
func DecodeIntelThermTemp(msrVal uint64, tjMax float64) (tempC float64, ok bool) {
	if tjMax <= 0 {
		tjMax = 100
	}
	// Bit 31 is Reading Valid.
	if msrVal&(1<<31) == 0 {
		return 0, false
	}
	delta := float64((msrVal >> 16) & 0x7F)
	temp := tjMax - delta
	if temp <= 0 || temp > 130 {
		return 0, false
	}
	return temp, true
}

// DecodeAMDZenTemp extracts Celsius Tctl/Tdie from SMN register 0x00059800 (SMU_THM_TCON_CUR_TMP).
func DecodeAMDZenTemp(smnRaw uint64) (tempC float64, ok bool) {
	raw32 := uint32(smnRaw)
	curTmp := float64((raw32>>21)&0x7FF) * 0.125
	if raw32&(1<<19) != 0 {
		curTmp -= 49.0
	}
	if curTmp <= 0 || curTmp > 130 {
		return 0, false
	}
	return curTmp, true
}

// DecodeEnergyUnitJ extracts RAPL energy unit in Joules from MSR 0x606 / 0xC0010299 bits [12:8].
func DecodeEnergyUnitJ(msrVal uint64) float64 {
	shift := (msrVal >> 8) & 0x1F
	if shift > 30 {
		return 0
	}
	return 1.0 / math.Exp2(float64(shift))
}

// ComputeRAPLPowerW computes average package power in Watts from wrapping 32-bit energy counters.
func ComputeRAPLPowerW(prevRaw, curRaw uint64, energyUnitJ, dtSeconds float64) float64 {
	if energyUnitJ <= 0 || dtSeconds <= 0.05 {
		return 0
	}
	prev32 := uint32(prevRaw)
	cur32 := uint32(curRaw)
	delta := uint64(cur32 - prev32) // wraps cleanly mod 2^32
	watts := float64(delta) * energyUnitJ / dtSeconds
	if watts < 0 || watts > 1500 {
		return 0
	}
	return watts
}
