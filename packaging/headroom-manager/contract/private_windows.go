//go:build windows

package contract

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

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
	if err != nil || actualOwner == nil || actualOwner.String() != owner {
		return errors.New("private storage owner is unsafe")
	}
	sddl := descriptor.String()
	// Windows can retain the informational auto-inheritance flags even after
	// replacing and protecting the DACL. They do not grant access. Require the
	// protected bit and exactly one full-control ACE for the current owner.
	if !privateWindowsDACLIsSafe(sddl, owner) {
		return errors.New("private storage ACL is unsafe")
	}
	return nil
}
