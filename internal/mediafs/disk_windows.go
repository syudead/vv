package mediafs

import (
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/syudead/vv/internal/domain"
)

// Storage IOCTLs from ntddstor.h; golang.org/x/sys/windows does not define them.
const (
	ioctlStorageGetDeviceNumber      = 0x2D1080
	ioctlStorageQueryProperty        = 0x2D1400
	storageDeviceSeekPenaltyProperty = 7
	propertyStandardQuery            = 0
)

type storageDeviceNumber struct {
	DeviceType      uint32
	DeviceNumber    uint32
	PartitionNumber uint32
}

type storagePropertyQuery struct {
	PropertyID           uint32
	QueryType            uint32
	AdditionalParameters [4]byte
}

type deviceSeekPenaltyDescriptor struct {
	Version           uint32
	Size              uint32
	IncursSeekPenalty byte
	_                 [3]byte
}

// ResolveDisk returns the physical disk of a media folder's volume and whether it incurs a seek penalty. A volume the
// storage IOCTLs do not answer, such as a network share or a volume spanning disks, has the volume as key and an
// unknown type. Some virtual disks report no seek penalty and count as solid-state. An unreadable folder returns an
// error, as does an empty one while populated (see openReadableRoot).
func (FS) ResolveDisk(root string, populated bool) (domain.Disk, error) {
	dir, err := openReadableRoot(root, populated)
	if err != nil {
		return domain.Disk{}, err
	}
	_ = dir.Close()
	volume, err := volumeName(root)
	if err != nil {
		return domain.Disk{Key: "volume:" + strings.ToLower(volumeOrRoot(root))}, nil
	}
	unknown := domain.Disk{Key: "volume:" + strings.ToLower(volume)}
	name, err := windows.UTF16PtrFromString(strings.TrimSuffix(volume, `\`))
	if err != nil {
		return unknown, nil
	}
	// Zero access rights suffice for these queries and need no administrator rights.
	handle, err := windows.CreateFile(name, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil,
		windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return unknown, nil
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	var number storageDeviceNumber
	var returned uint32
	if err := windows.DeviceIoControl(handle, ioctlStorageGetDeviceNumber, nil, 0,
		(*byte)(unsafe.Pointer(&number)), uint32(unsafe.Sizeof(number)), &returned, nil); err != nil {
		return unknown, nil
	}
	disk := domain.Disk{Key: fmt.Sprintf("disk:%d", number.DeviceNumber)}
	query := storagePropertyQuery{PropertyID: storageDeviceSeekPenaltyProperty, QueryType: propertyStandardQuery}
	var penalty deviceSeekPenaltyDescriptor
	if err := windows.DeviceIoControl(handle, ioctlStorageQueryProperty,
		(*byte)(unsafe.Pointer(&query)), uint32(unsafe.Sizeof(query)),
		(*byte)(unsafe.Pointer(&penalty)), uint32(unsafe.Sizeof(penalty)), &returned, nil); err != nil {
		return disk, nil
	}
	if penalty.IncursSeekPenalty != 0 {
		disk.Type = domain.DiskRotational
	} else {
		disk.Type = domain.DiskSolidState
	}
	return disk, nil
}

// volumeName returns the \\?\Volume{GUID}\ name of the volume holding path.
func volumeName(path string) (string, error) {
	mount := volumeOrRoot(path)
	name, err := windows.UTF16PtrFromString(mount)
	if err != nil {
		return "", err
	}
	buf := make([]uint16, windows.MAX_PATH)
	if err := windows.GetVolumeNameForVolumeMountPoint(name, &buf[0], uint32(len(buf))); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buf), nil
}

// volumeOrRoot returns the mount point of the volume holding path, such as C:\ or \\server\share\, or path itself
// when Windows does not say.
func volumeOrRoot(path string) string {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return path
	}
	buf := make([]uint16, windows.MAX_PATH)
	if err := windows.GetVolumePathName(name, &buf[0], uint32(len(buf))); err != nil {
		return path
	}
	return windows.UTF16ToString(buf)
}
