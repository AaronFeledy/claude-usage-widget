//go:build windows

package contract

import (
	"errors"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

var privateAdvapi32 = windows.NewLazySystemDLL("advapi32.dll")
var privateGetAce = privateAdvapi32.NewProc("GetAce")

type privateACEHeader struct {
	Type, Flags byte
	Size        uint16
}

type privateAccessAllowedACE struct {
	Header   privateACEHeader
	Mask     uint32
	SidStart uint32
}

type privateACLHeader struct {
	Revision  byte
	Reserved  byte
	Size      uint16
	Count     uint16
	Reserved2 uint16
}

const privateFileAllAccess = 0x001f01ff

func securePrivateDirectory(path string, configure bool) error {
	return validatePrivateWindows(path, true, configure)
}
func securePrivateFile(path string, newlyCreated bool) error {
	return validatePrivateWindows(path, false, newlyCreated)
}

func validatePrivateWindows(path string, directory, setACL bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if pathIsLinkOrReparse(path, info) || directory != info.IsDir() || (!directory && !info.Mode().IsRegular()) {
		return errors.New("private storage path is unsafe")
	}
	owner, err := currentOwnerIdentity()
	if err != nil {
		return err
	}
	if setACL {
		descriptor, descriptorErr := windows.SecurityDescriptorFromString("O:" + owner + "D:P(A;;FA;;;" + owner + ")")
		if descriptorErr != nil {
			return descriptorErr
		}
		dacl, _, descriptorErr := descriptor.DACL()
		if descriptorErr != nil {
			return descriptorErr
		}
		sid, descriptorErr := windows.StringToSid(owner)
		if descriptorErr != nil {
			return descriptorErr
		}
		if descriptorErr = windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, sid, nil, dacl, nil); descriptorErr != nil {
			return descriptorErr
		}
	}
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	actualOwner, _, err := descriptor.Owner()
	expectedOwner, sidErr := windows.StringToSid(owner)
	if err != nil || sidErr != nil || actualOwner == nil || !actualOwner.Equals(expectedOwner) {
		return errors.New("private storage owner is unsafe")
	}
	control, _, controlErr := descriptor.Control()
	dacl, _, daclErr := descriptor.DACL()
	if controlErr != nil || daclErr != nil || control&windows.SE_DACL_PROTECTED == 0 || dacl == nil || (*privateACLHeader)(unsafe.Pointer(dacl)).Count != 1 {
		return errors.New("private storage ACL is unsafe")
	}
	var acePointer unsafe.Pointer
	ok, _, _ := privateGetAce.Call(uintptr(unsafe.Pointer(dacl)), 0, uintptr(unsafe.Pointer(&acePointer)))
	if ok == 0 || acePointer == nil {
		return errors.New("private storage ACL is unsafe")
	}
	ace := (*privateAccessAllowedACE)(acePointer)
	sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	sidOffset := int(unsafe.Offsetof(ace.SidStart))
	if ace.Header.Type != 0 || ace.Header.Flags != 0 || int(ace.Header.Size) < sidOffset+8 || ace.Mask != privateFileAllAccess || !sid.IsValid() ||
		sidOffset+sid.Len() != int(ace.Header.Size) || !sid.Equals(expectedOwner) {
		return errors.New("private storage ACL is unsafe")
	}
	return nil
}
