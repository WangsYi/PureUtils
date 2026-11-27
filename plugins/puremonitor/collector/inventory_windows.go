//go:build windows

package collector

import (
	"fmt"
	"net"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	modDXGI                             = windows.NewLazySystemDLL("dxgi.dll")
	modUser32Inv                        = windows.NewLazySystemDLL("user32.dll")
	procCreateDXGIFactory1              = modDXGI.NewProc("CreateDXGIFactory1")
	procGetLogicalProcessorInfoEx       = modKernel32.NewProc("GetLogicalProcessorInformationEx")
	procGetDiskFreeSpaceExW             = modKernel32.NewProc("GetDiskFreeSpaceExW")
	procGetVolumeInformationW           = modKernel32.NewProc("GetVolumeInformationW")
	procGetLogicalDriveStringsW         = modKernel32.NewProc("GetLogicalDriveStringsW")
	procGetDriveTypeW                   = modKernel32.NewProc("GetDriveTypeW")
	procEnumDisplayDevicesW             = modUser32Inv.NewProc("EnumDisplayDevicesW")
	procEnumDisplaySettingsExW          = modUser32Inv.NewProc("EnumDisplaySettingsExW")
)

type winGUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var iidIDXGIFactory1 = winGUID{
	Data1: 0x770aae78,
	Data2: 0xf26f,
	Data3: 0x4dba,
	Data4: [8]byte{0xa8, 0x29, 0x25, 0x3c, 0x83, 0xd1, 0xb3, 0x87},
}

type dxgiAdapterDesc1 struct {
	Description           [128]uint16
	VendorID              uint32
	DeviceID              uint32
	SubSysID              uint32
	Revision              uint32
	DedicatedVideoMemory  uintptr
	DedicatedSystemMemory uintptr
	SharedSystemMemory    uintptr
	AdapterLuid           int64
	Flags                 uint32
}

// CollectInventory gathers full static OS, motherboard, CPU, RAM, GPU, storage, NIC, and display specs.
func CollectInventory() HardwareInventory {
	inv := HardwareInventory{
		CollectedAt: time.Now(),
	}
	smbios := ReadSMBIOS()
	inv.OS = collectOSInfo()
	inv.System = smbios.System
	inv.Motherboard = smbios.Motherboard
	inv.CPU = collectCPUHardwareInfo(smbios)
	inv.RAMModules = smbios.RAMModules
	inv.TotalRAMMB = smbios.TotalRAMMB
	if inv.TotalRAMMB == 0 {
		if mem := readMemoryStatus(); mem.TotalPhys > 0 {
			inv.TotalRAMMB = mem.TotalPhys / (1024 * 1024)
		}
	}
	inv.GPUs = collectGPUInventory()
	inv.Disks = collectPhysicalDisks()
	inv.Volumes = collectVolumes()
	inv.NetAdapters = collectNetworkAdapters()
	inv.Displays = collectDisplays()
	return inv
}

func collectOSInfo() OSInfo {
	ver := windows.RtlGetVersion()
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x64"
	}
	name := "Windows"
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE); err == nil {
		defer k.Close()
		if prod, _, err := k.GetStringValue("ProductName"); err == nil && prod != "" {
			name = prod
			if ver != nil && ver.BuildNumber >= 22000 && strings.Contains(name, "Windows 10") {
				name = strings.Replace(name, "Windows 10", "Windows 11", 1)
			}
		}
		if disp, _, err := k.GetStringValue("DisplayVersion"); err == nil && disp != "" {
			return OSInfo{
				Name:    name,
				Version: disp,
				Build:   fmt.Sprintf("%d", ver.BuildNumber),
				Arch:    arch,
			}
		}
	}
	return OSInfo{
		Name:    name,
		Version: fmt.Sprintf("%d.%d", ver.MajorVersion, ver.MinorVersion),
		Build:   fmt.Sprintf("%d", ver.BuildNumber),
		Arch:    arch,
	}
}

func collectCPUHardwareInfo(smbios SMBIOSData) CPUHardwareInfo {
	info := CPUHardwareInfo{
		Socket:      smbios.Socket,
		Vendor:      smbios.CPUVendor,
		Name:        smbios.CPUVersion,
		MaxClockMHz: smbios.CPUMaxMHz,
	}
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE); err == nil {
		defer k.Close()
		if s, _, err := k.GetStringValue("ProcessorNameString"); err == nil && strings.TrimSpace(s) != "" {
			info.Name = strings.TrimSpace(s)
		}
		if v, _, err := k.GetStringValue("VendorIdentifier"); err == nil && strings.TrimSpace(v) != "" {
			info.Vendor = strings.TrimSpace(v)
		}
		if mhz, _, err := k.GetIntegerValue("~MHz"); err == nil && mhz > 0 {
			info.BaseClockMHz = int(mhz)
			if info.MaxClockMHz == 0 {
				info.MaxClockMHz = int(mhz)
			}
		}
	}

	phys, logical, l1, l2, l3, pCores, eCores := queryCPUProcessorTopology()
	info.PhysicalCores = phys
	info.LogicalCores = logical
	info.L1CacheKB = l1
	info.L2CacheKB = l2
	info.L3CacheKB = l3
	info.PCores = pCores
	info.ECores = eCores
	if info.LogicalCores == 0 {
		info.LogicalCores = runtime.NumCPU()
	}
	if info.PhysicalCores == 0 {
		info.PhysicalCores = info.LogicalCores
	}
	return info
}

func queryCPUProcessorTopology() (physCores, logicalCores, l1KB, l2KB, l3KB, pCores, eCores int) {
	if procGetLogicalProcessorInfoEx.Find() != nil {
		return 0, 0, 0, 0, 0, 0, 0
	}
	const relationAll = 0xFFFF
	var length uint32
	procGetLogicalProcessorInfoEx.Call(relationAll, 0, uintptr(unsafe.Pointer(&length)))
	if length == 0 || length > 1024*1024 {
		return 0, 0, 0, 0, 0, 0, 0
	}
	buf := make([]byte, length)
	r, _, _ := procGetLogicalProcessorInfoEx.Call(relationAll, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&length)))
	if r == 0 {
		return 0, 0, 0, 0, 0, 0, 0
	}

	effClasses := map[byte]int{}
	var seenL1, seenL2, seenL3 int
	for off := 0; off+8 <= int(length); {
		rel := *(*uint32)(unsafe.Pointer(&buf[off]))
		sz := int(*(*uint32)(unsafe.Pointer(&buf[off+4])))
		if sz < 8 || off+sz > int(length) {
			break
		}
		switch rel {
		case 0: // RelationProcessorCore
			physCores++
			if off+12 <= int(length) {
				effClass := buf[off+10]
				effClasses[effClass]++
			}
			// GroupCount at off+26 (uint16), GroupMask array starts at off+48 (GROUP_AFFINITY = 16 bytes each: Mask uintptr, Group uint16, Reserved [3]uint16).
			if off+48 <= int(length) {
				groupCount := int(*(*uint16)(unsafe.Pointer(&buf[off+26])))
				for g := 0; g < groupCount && off+48+g*16+8 <= off+sz; g++ {
					mask := *(*uint64)(unsafe.Pointer(&buf[off+48+g*16]))
					logicalCores += popCount64(mask)
				}
			}
		case 2: // RelationCache
			// CACHE_RELATIONSHIP starts at off+8: Level(1B), Associativity(1B), LineSize(2B), CacheSize(4B).
			if off+16 <= int(length) {
				level := buf[off+8]
				cacheSize := int(*(*uint32)(unsafe.Pointer(&buf[off+12])))
				switch level {
				case 1:
					seenL1 += cacheSize
				case 2:
					seenL2 += cacheSize
				case 3:
					seenL3 += cacheSize
				}
			}
		}
		off += sz
	}
	l1KB = seenL1 / 1024
	l2KB = seenL2 / 1024
	l3KB = seenL3 / 1024
	if len(effClasses) > 1 {
		pCores = effClasses[1]
		eCores = effClasses[0]
		if pCores == 0 {
			pCores = effClasses[0]
		}
	}
	return physCores, logicalCores, l1KB, l2KB, l3KB, pCores, eCores
}

func popCount64(x uint64) int {
	c := 0
	for x != 0 {
		x &= x - 1
		c++
	}
	return c
}

func comVtbl(obj uintptr, slot int, args ...uintptr) uintptr {
	vtbl := *(*uintptr)(unsafe.Pointer(obj))
	fn := *(*uintptr)(unsafe.Pointer(vtbl + uintptr(slot)*unsafe.Sizeof(uintptr(0))))
	full := make([]uintptr, 0, len(args)+1)
	full = append(full, obj)
	full = append(full, args...)
	r, _, _ := syscall.SyscallN(fn, full...)
	return r
}

func collectGPUInventory() []GPUHardwareInfo {
	if procCreateDXGIFactory1.Find() != nil {
		return nil
	}
	var factory uintptr
	hr, _, _ := procCreateDXGIFactory1.Call(uintptr(unsafe.Pointer(&iidIDXGIFactory1)), uintptr(unsafe.Pointer(&factory)))
	if int32(hr) < 0 || factory == 0 {
		return nil
	}
	defer comVtbl(factory, 2) // Release

	drivers := readGPUDriverVersionsFromRegistry()
	var out []GPUHardwareInfo
	for idx := uint32(0); idx < 16; idx++ {
		var adapter uintptr
		// IDXGIFactory1::EnumAdapters1 is slot 12
		hrEnum := comVtbl(factory, 12, uintptr(idx), uintptr(unsafe.Pointer(&adapter)))
		if int32(hrEnum) < 0 || adapter == 0 {
			break
		}
		var desc dxgiAdapterDesc1
		// IDXGIAdapter1::GetDesc1 is slot 10
		hrDesc := comVtbl(adapter, 10, uintptr(unsafe.Pointer(&desc)))
		comVtbl(adapter, 2) // Release
		if int32(hrDesc) < 0 {
			continue
		}
		// Skip Microsoft Basic Render Driver (software adapter, Flag bit 1 = DXGI_ADAPTER_FLAG_SOFTWARE = 2)
		if desc.Flags&2 != 0 || desc.VendorID == 0x1414 {
			continue
		}
		name := strings.TrimSpace(windows.UTF16ToString(desc.Description[:]))
		if name == "" {
			continue
		}
		vendor := gpuVendorFromID(desc.VendorID, name)
		drv := drivers[strings.ToLower(name)]
		if drv == "" && vendor != "" {
			drv = drivers[vendor]
		}
		out = append(out, GPUHardwareInfo{
			Name:          name,
			Vendor:        vendor,
			VendorID:      desc.VendorID,
			DeviceID:      desc.DeviceID,
			VRAMMB:        uint64(desc.DedicatedVideoMemory) / (1024 * 1024),
			DriverVersion: drv,
		})
	}
	return out
}

func gpuVendorFromID(vid uint32, name string) string {
	switch vid {
	case 0x10DE:
		return "NVIDIA"
	case 0x1002, 0x1022:
		return "AMD"
	case 0x8086:
		return "Intel"
	}
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "nvidia") || strings.Contains(lower, "geforce") || strings.Contains(lower, "rtx") || strings.Contains(lower, "gtx"):
		return "NVIDIA"
	case strings.Contains(lower, "amd") || strings.Contains(lower, "radeon"):
		return "AMD"
	case strings.Contains(lower, "intel") || strings.Contains(lower, "arc") || strings.Contains(lower, "uhd") || strings.Contains(lower, "iris"):
		return "Intel"
	default:
		return ""
	}
}

func readGPUDriverVersionsFromRegistry() map[string]string {
	out := make(map[string]string)
	base, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}`, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return out
	}
	defer base.Close()
	subkeys, err := base.ReadSubKeyNames(64)
	if err != nil {
		return out
	}
	for _, sk := range subkeys {
		if len(sk) != 4 {
			continue
		}
		k, err := registry.OpenKey(base, sk, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		desc, _, _ := k.GetStringValue("DriverDesc")
		ver, _, _ := k.GetStringValue("DriverVersion")
		prov, _, _ := k.GetStringValue("ProviderName")
		k.Close()
		desc = strings.TrimSpace(desc)
		ver = strings.TrimSpace(ver)
		if desc != "" && ver != "" {
			out[strings.ToLower(desc)] = ver
			if v := gpuVendorFromID(0, prov+" "+desc); v != "" {
				if _, exists := out[v]; !exists {
					out[v] = ver
				}
			}
		}
	}
	return out
}

func collectPhysicalDisks() []DiskHardwareInfo {
	const (
		ioctlStorageQueryProperty = 0x002D1400
		ioctlDiskGetDriveGeometry = 0x000700A0 // IOCTL_DISK_GET_DRIVE_GEOMETRY_EX
	)
	var out []DiskHardwareInfo
	for idx := 0; idx < 16; idx++ {
		path, _ := windows.UTF16PtrFromString(fmt.Sprintf(`\\.\PhysicalDrive%d`, idx))
		h, err := windows.CreateFile(path, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
		if err != nil {
			continue
		}
		info := DiskHardwareInfo{Index: idx}

		// StorageDeviceProperty = 0
		type storagePropertyQuery struct {
			PropertyID uint32
			QueryType  uint32
			Additional [1]byte
		}
		q := storagePropertyQuery{PropertyID: 0, QueryType: 0}
		buf := make([]byte, 2048)
		var ret uint32
		if err := windows.DeviceIoControl(h, ioctlStorageQueryProperty, (*byte)(unsafe.Pointer(&q)), uint32(unsafe.Sizeof(q)), &buf[0], uint32(len(buf)), &ret, nil); err == nil && ret >= 36 {
			// STORAGE_DEVICE_DESCRIPTOR:
			// [12] VendorIdOffset, [16] ProductIdOffset, [20] ProductRevisionOffset, [24] SerialNumberOffset, [28] BusType (uint32)
			vendOff := *(*uint32)(unsafe.Pointer(&buf[12]))
			prodOff := *(*uint32)(unsafe.Pointer(&buf[16]))
			serOff := *(*uint32)(unsafe.Pointer(&buf[24]))
			busType := *(*uint32)(unsafe.Pointer(&buf[28]))
			rem := buf[11] != 0
			vend := readCString(buf[:ret], vendOff)
			prod := readCString(buf[:ret], prodOff)
			info.Serial = readCString(buf[:ret], serOff)
			info.Model = strings.TrimSpace(strings.TrimSpace(vend) + " " + strings.TrimSpace(prod))
			if info.Model == "" {
				info.Model = fmt.Sprintf("PhysicalDrive%d", idx)
			}
			info.BusType = storageBusTypeName(busType)
			if rem {
				info.MediaType = "Removable"
			}
		}

		// StorageDeviceSeekPenaltyProperty = 7
		if info.MediaType == "" {
			q7 := storagePropertyQuery{PropertyID: 7, QueryType: 0}
			var seekBuf [16]byte
			if err := windows.DeviceIoControl(h, ioctlStorageQueryProperty, (*byte)(unsafe.Pointer(&q7)), uint32(unsafe.Sizeof(q7)), &seekBuf[0], uint32(len(seekBuf)), &ret, nil); err == nil && ret >= 12 {
				incursSeek := seekBuf[8] != 0
				if incursSeek {
					info.MediaType = "HDD"
				} else {
					info.MediaType = "SSD"
				}
			}
		}
		if info.MediaType == "" {
			info.MediaType = "SSD"
		}

		// IOCTL_DISK_GET_DRIVE_GEOMETRY_EX: DISK_GEOMETRY (24B) + DiskSize (int64 at offset 24)
		var geomBuf [64]byte
		if err := windows.DeviceIoControl(h, ioctlDiskGetDriveGeometry, nil, 0, &geomBuf[0], uint32(len(geomBuf)), &ret, nil); err == nil && ret >= 32 {
			sz := *(*int64)(unsafe.Pointer(&geomBuf[24]))
			if sz > 0 {
				info.SizeBytes = uint64(sz)
			}
		}

		_ = windows.CloseHandle(h)
		if info.Model != "" || info.SizeBytes > 0 {
			out = append(out, info)
		}
	}
	return out
}

func readCString(buf []byte, off uint32) string {
	if off == 0 || int(off) >= len(buf) {
		return ""
	}
	end := int(off)
	for end < len(buf) && buf[end] != 0 {
		end++
	}
	return strings.TrimSpace(string(buf[off:end]))
}

func storageBusTypeName(code uint32) string {
	switch code {
	case 3:
		return "ATA"
	case 7:
		return "USB"
	case 8:
		return "RAID"
	case 11:
		return "SATA"
	case 12:
		return "SD"
	case 17:
		return "NVMe"
	case 18:
		return "SCM"
	default:
		return "Storage"
	}
}

func collectVolumes() []VolumeInfo {
	buf := make([]uint16, 256)
	n, _, _ := procGetLogicalDriveStringsW.Call(uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])))
	if n == 0 || n >= uintptr(len(buf)) {
		return nil
	}
	var out []VolumeInfo
	for i := 0; i < int(n); {
		end := i
		for end < int(n) && buf[end] != 0 {
			end++
		}
		if end == i {
			break
		}
		drive := windows.UTF16ToString(buf[i:end])
		i = end + 1

		p, _ := windows.UTF16PtrFromString(drive)
		dt, _, _ := procGetDriveTypeW.Call(uintptr(unsafe.Pointer(p)))
		// DRIVE_FIXED=3, DRIVE_REMOVABLE=2
		if dt != 3 && dt != 2 {
			continue
		}
		var freeAvail, total, totalFree uint64
		ok, _, _ := procGetDiskFreeSpaceExW.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&freeAvail)), uintptr(unsafe.Pointer(&total)), uintptr(unsafe.Pointer(&totalFree)))
		if ok == 0 || total == 0 {
			continue
		}
		var volName [128]uint16
		var fsName [64]uint16
		procGetVolumeInformationW.Call(
			uintptr(unsafe.Pointer(p)),
			uintptr(unsafe.Pointer(&volName[0])), uintptr(len(volName)),
			0, 0, 0,
			uintptr(unsafe.Pointer(&fsName[0])), uintptr(len(fsName)),
		)
		out = append(out, VolumeInfo{
			Drive:      strings.TrimSuffix(drive, `\`),
			Label:      strings.TrimSpace(windows.UTF16ToString(volName[:])),
			FileSystem: strings.TrimSpace(windows.UTF16ToString(fsName[:])),
			TotalBytes: total,
			FreeBytes:  freeAvail,
		})
	}
	return out
}

func collectNetworkAdapters() []NetAdapterInfo {
	const flags = windows.GAA_FLAG_SKIP_ANYCAST | windows.GAA_FLAG_SKIP_MULTICAST | windows.GAA_FLAG_SKIP_DNS_SERVER
	size := uint32(16 * 1024)
	var buf []byte
	for i := 0; i < 3; i++ {
		buf = make([]byte, size)
		aa := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
		err := windows.GetAdaptersAddresses(windows.AF_INET, flags, 0, aa, &size)
		if err == nil {
			break
		}
		if err != windows.ERROR_BUFFER_OVERFLOW {
			return nil
		}
	}
	var out []NetAdapterInfo
	for aa := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])); aa != nil; aa = aa.Next {
		if aa.IfType == 24 { // IF_TYPE_SOFTWARE_LOOPBACK
			continue
		}
		desc := strings.TrimSpace(windows.UTF16PtrToString(aa.Description))
		name := strings.TrimSpace(windows.UTF16PtrToString(aa.FriendlyName))
		if isVirtualAdapterDesc(desc) {
			continue
		}
		var macParts []string
		for i := uint32(0); i < aa.PhysicalAddressLength && int(i) < len(aa.PhysicalAddress); i++ {
			macParts = append(macParts, fmt.Sprintf("%02X", aa.PhysicalAddress[i]))
		}
		ipv4 := ""
		for ua := aa.FirstUnicastAddress; ua != nil; ua = ua.Next {
			if ua.Address.Sockaddr == nil {
				continue
			}
			rawSock := (*windows.RawSockaddrInet4)(unsafe.Pointer(ua.Address.Sockaddr))
			if rawSock.Family == windows.AF_INET {
				ipv4 = net.IPv4(rawSock.Addr[0], rawSock.Addr[1], rawSock.Addr[2], rawSock.Addr[3]).String()
				break
			}
		}
		out = append(out, NetAdapterInfo{
			Index:       aa.IfIndex,
			Name:        name,
			Description: desc,
			MAC:         strings.Join(macParts, ":"),
			SpeedMbps:   aa.ReceiveLinkSpeed / 1_000_000,
			IPv4:        ipv4,
			Connected:   aa.OperStatus == 1, // IfOperStatusUp
			Physical:    aa.IfType == 6 || aa.IfType == 71,
		})
	}
	return out
}

func isVirtualAdapterDesc(desc string) bool {
	l := strings.ToLower(desc)
	return strings.Contains(l, "loopback") ||
		strings.Contains(l, "wan miniport") ||
		strings.Contains(l, "teredo") ||
		strings.Contains(l, "isatap") ||
		strings.Contains(l, "kernel debug") ||
		strings.Contains(l, "qos packet") ||
		strings.Contains(l, "wfp ")
}

type displayDeviceW struct {
	Cb           uint32
	DeviceName   [32]uint16
	DeviceString [128]uint16
	StateFlags   uint32
	DeviceID     [128]uint16
	DeviceKey    [128]uint16
}

type devModeW struct {
	DeviceName       [32]uint16
	SpecVersion      uint16
	DriverVersion    uint16
	Size             uint16
	DriverExtra      uint16
	Fields           uint32
	PositionX        int32
	PositionY        int32
	DisplayOrient    uint32
	DisplayFixedOut  uint32
	Color            int16
	Duplex           int16
	YResolution      int16
	TTOption         int16
	Collate          int16
	FormName         [32]uint16
	LogPixels        uint16
	BitsPerPel       uint32
	PelsWidth        uint32
	PelsHeight       uint32
	DisplayFlags     uint32
	DisplayFrequency uint32
}

func collectDisplays() []DisplayHardwareInfo {
	const (
		displayDeviceAttached = 0x00000001
		displayDevicePrimary  = 0x00000004
		enumCurrentSettings   = ^uint32(0)
	)
	var out []DisplayHardwareInfo
	for i := uint32(0); i < 16; i++ {
		var dd displayDeviceW
		dd.Cb = uint32(unsafe.Sizeof(dd))
		r, _, _ := procEnumDisplayDevicesW.Call(0, uintptr(i), uintptr(unsafe.Pointer(&dd)), 0)
		if r == 0 {
			break
		}
		if dd.StateFlags&displayDeviceAttached == 0 {
			continue
		}
		adapter := strings.TrimSpace(windows.UTF16ToString(dd.DeviceString[:]))

		monName := "Display " + strconv.Itoa(len(out)+1)
		var monDev displayDeviceW
		monDev.Cb = uint32(unsafe.Sizeof(monDev))
		if mr, _, _ := procEnumDisplayDevicesW.Call(uintptr(unsafe.Pointer(&dd.DeviceName[0])), 0, uintptr(unsafe.Pointer(&monDev)), 0); mr != 0 {
			if s := strings.TrimSpace(windows.UTF16ToString(monDev.DeviceString[:])); s != "" {
				monName = s
			}
		}

		var dm devModeW
		dm.Size = uint16(unsafe.Sizeof(dm))
		var res string
		var hz int
		if er, _, _ := procEnumDisplaySettingsExW.Call(uintptr(unsafe.Pointer(&dd.DeviceName[0])), uintptr(enumCurrentSettings), uintptr(unsafe.Pointer(&dm)), 0); er != 0 {
			res = fmt.Sprintf("%dx%d", dm.PelsWidth, dm.PelsHeight)
			hz = int(dm.DisplayFrequency)
		}
		out = append(out, DisplayHardwareInfo{
			Name:       monName,
			Adapter:    adapter,
			Resolution: res,
			RefreshHz:  hz,
			Primary:    dd.StateFlags&displayDevicePrimary != 0,
		})
	}
	return out
}
