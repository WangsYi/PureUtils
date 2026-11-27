//go:build windows

package collector

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	storageDeviceTemperatureProperty = 34
	ioctlStorageQueryProperty        = 0x002D1400
)

type storageTempInfo struct {
	Index             uint16
	Temperature       int16
	OverThreshold     int16
	UnderThreshold    int16
	OverTempFlag      byte
	UnderTempFlag     byte
	EventGenerated    byte
	_                 byte
	_                 int32
}

type storageTemperatureDataDescriptor struct {
	Version                 uint32
	Size                    uint32
	CriticalTemperature     int16
	WarningTemperature      int16
	InfoCount               uint16
	_                       [2]byte
	_                       [4]uint32
	TemperatureInfo         [4]storageTempInfo
}

// DiskTemperatureReading holds SMART/NVMe temperature info for a physical disk.
type DiskTemperatureReading struct {
	Index     int     `json:"index"`
	TempC     float64 `json:"tempC"`
	WarnTempC float64 `json:"warnTempC"`
	CritTempC float64 `json:"critTempC"`
}

// QueryDiskTemperatures queries physical disk temperatures using StorageDeviceTemperatureProperty.
// Typically requires elevated privileges (--monitor-helper) due to Windows PhysicalDrive ACLs.
func QueryDiskTemperatures(maxIndex int) []DiskTemperatureReading {
	if maxIndex <= 0 {
		maxIndex = 16
	}
	type storagePropertyQuery struct {
		PropertyID uint32
		QueryType  uint32
		Additional [1]byte
	}
	var out []DiskTemperatureReading
	for idx := 0; idx < maxIndex; idx++ {
		path, _ := windows.UTF16PtrFromString(fmt.Sprintf(`\\.\PhysicalDrive%d`, idx))
		h, err := windows.CreateFile(path, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
		if err != nil {
			continue
		}
		q := storagePropertyQuery{
			PropertyID: storageDeviceTemperatureProperty,
			QueryType:  0,
		}
		var desc storageTemperatureDataDescriptor
		var ret uint32
		err = windows.DeviceIoControl(
			h,
			ioctlStorageQueryProperty,
			(*byte)(unsafe.Pointer(&q)),
			uint32(unsafe.Sizeof(q)),
			(*byte)(unsafe.Pointer(&desc)),
			uint32(unsafe.Sizeof(desc)),
			&ret,
			nil,
		)
		_ = windows.CloseHandle(h)
		if err != nil || ret < 16 || desc.InfoCount == 0 {
			continue
		}
		t := float64(desc.TemperatureInfo[0].Temperature)
		if t <= 0 || t > 130 {
			continue
		}
		out = append(out, DiskTemperatureReading{
			Index:     idx,
			TempC:     t,
			WarnTempC: float64(desc.WarningTemperature),
			CritTempC: float64(desc.CriticalTemperature),
		})
	}
	return out
}
