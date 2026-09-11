//go:build windows

package contract

import "golang.org/x/sys/windows"

func currentOwnerIdentity() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return user.User.Sid.String(), nil
}
