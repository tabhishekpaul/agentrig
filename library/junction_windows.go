package library

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

const (
	fsctlSetReparsePoint = 0x000900A4
	reparseTagMountPoint = 0xA0000003
)

// junction makes at a directory junction to real by writing the mount-point
// reparse data itself (no cmd.exe, so no path is ever parsed by a shell).
func junction(at, real string) error {
	if !filepath.IsAbs(real) {
		return errors.New("junction target must be absolute")
	}
	sub := utf16.Encode([]rune(`\??\` + real))
	prt := utf16.Encode([]rune(real))
	path := make([]byte, 0, (len(sub)+len(prt)+2)*2)
	for _, s := range [][]uint16{sub, prt} {
		for _, c := range append(s, 0) {
			path = binary.LittleEndian.AppendUint16(path, c)
		}
	}
	buf := make([]byte, 8, 16+len(path))
	binary.LittleEndian.PutUint32(buf[0:], reparseTagMountPoint)
	binary.LittleEndian.PutUint16(buf[4:], uint16(8+len(path))) // data length
	buf = binary.LittleEndian.AppendUint16(buf, 0)              // substitute name offset
	buf = binary.LittleEndian.AppendUint16(buf, uint16(len(sub)*2))
	buf = binary.LittleEndian.AppendUint16(buf, uint16((len(sub)+1)*2)) // print name offset
	buf = binary.LittleEndian.AppendUint16(buf, uint16(len(prt)*2))
	buf = append(buf, path...)

	if err := os.Mkdir(at, 0o755); err != nil {
		return err
	}
	p, err := syscall.UTF16PtrFromString(at)
	if err != nil {
		os.Remove(at)
		return err
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_OPEN_REPARSE_POINT|syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		os.Remove(at)
		return err
	}
	var n uint32
	err = syscall.DeviceIoControl(h, fsctlSetReparsePoint, (*byte)(unsafe.Pointer(&buf[0])), uint32(len(buf)), nil, 0, &n, nil)
	syscall.CloseHandle(h)
	if err != nil {
		os.Remove(at)
	}
	return err
}
