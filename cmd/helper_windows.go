package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/Unknwon/com"

	"github.com/gpmgo/gopm/doc"
)

func makeLink(srcPath, destPath string) error {
	// Check if Windows version is XP.
	if getWindowsVersion() >= 6 {
		cmd := exec.Command("cmd", "/c", "mklink", "/j", destPath, srcPath)
		return cmd.Run()
	}

	// XP.
	isWindowsXP = true
	// if both are ntfs file system
	if volumnType(srcPath) == "NTFS" && volumnType(destPath) == "NTFS" {
		// if has junction command installed
		file, err := exec.LookPath("junction")
		if err == nil {
			path, _ := filepath.Abs(file)
			if com.IsFile(path) {
				cmd := exec.Command("cmd", "/c", "junction", destPath, srcPath)
				return cmd.Run()
			}
		}
	}
	os.RemoveAll(destPath)

	return com.CopyDir(srcPath, destPath, func(filePath string) bool {
		return strings.Contains(filePath, doc.VENDOR)
	})
}

func volumnType(dir string) string {
	pd := dir[:3]
	dll := syscall.MustLoadDLL("kernel32.dll")
	GetVolumeInformation := dll.MustFindProc("GetVolumeInformationW")

	var volumeNameSize uint32 = 260
	var nFileSystemNameSize, lpVolumeSerialNumber uint32
	var lpFileSystemFlags, lpMaximumComponentLength uint32
	var lpFileSystemNameBuffer, volumeName [260]byte
	var ps *uint16 = syscall.StringToUTF16Ptr(pd)

	_, _, _ = GetVolumeInformation.Call(uintptr(unsafe.Pointer(ps)),
		uintptr(unsafe.Pointer(&volumeName)),
		uintptr(volumeNameSize),
		uintptr(unsafe.Pointer(&lpVolumeSerialNumber)),
		uintptr(unsafe.Pointer(&lpMaximumComponentLength)),
		uintptr(unsafe.Pointer(&lpFileSystemFlags)),
		uintptr(unsafe.Pointer(&lpFileSystemNameBuffer)),
		uintptr(unsafe.Pointer(&nFileSystemNameSize)), 0)

	var bytes []byte
	if lpFileSystemNameBuffer[6] == 0 {
		bytes = []byte{lpFileSystemNameBuffer[0], lpFileSystemNameBuffer[2],
			lpFileSystemNameBuffer[4]}
	} else {
		bytes = []byte{lpFileSystemNameBuffer[0], lpFileSystemNameBuffer[2],
			lpFileSystemNameBuffer[4], lpFileSystemNameBuffer[6]}
	}

	return string(bytes)
}

func getWindowsVersion() int {
	dll := syscall.MustLoadDLL("kernel32.dll")
	p := dll.MustFindProc("GetVersion")
	v, _, _ := p.Call()
	return int(byte(v))
}
