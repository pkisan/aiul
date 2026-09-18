//go:build darwin

package platform

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// The worker runs as a dedicated account rather than as root or as the person
// using the Mac. macOS convention is a leading underscore for system service
// accounts, which also hides it from the login window.
//
// It is created with no login shell and no home directory it can be logged into:
// it exists to own a process and a spool directory, nothing more.
const (
	ServiceUserName  = "_aiul"
	ServiceGroupName = "_aiul"

	// A free id in the range macOS leaves for third-party service accounts.
	serviceUID = 448
	serviceGID = 448
)

// ServiceAccount reports the uid and gid of our service account, creating nothing.
// A uid of -1 means it does not exist yet.
func ServiceAccount() (uid, gid int) {
	return lookupID("Users", ServiceUserName, "UniqueID"),
		lookupID("Groups", ServiceGroupName, "PrimaryGroupID")
}

// CreateServiceAccountCommands returns the exact commands, for showing first.
func CreateServiceAccountCommands() []string {
	return []string{
		fmt.Sprintf("sudo dscl . -create /Groups/%s PrimaryGroupID %d", ServiceGroupName, serviceGID),
		fmt.Sprintf("sudo dscl . -create /Users/%s UniqueID %d", ServiceUserName, serviceUID),
		fmt.Sprintf("sudo dscl . -create /Users/%s PrimaryGroupID %d", ServiceUserName, serviceGID),
		fmt.Sprintf("sudo dscl . -create /Users/%s UserShell /usr/bin/false", ServiceUserName),
		fmt.Sprintf("sudo dscl . -create /Users/%s NFSHomeDirectory /var/empty", ServiceUserName),
		fmt.Sprintf("sudo dscl . -create /Users/%s IsHidden 1", ServiceUserName),
	}
}

// CreateServiceAccount makes the account if it is not already there. Creating one
// that exists is not an error, so install can be run twice.
func CreateServiceAccount() (uid, gid int, err error) {
	if gid := lookupID("Groups", ServiceGroupName, "PrimaryGroupID"); gid < 0 {
		if err := run("dscl", ".", "-create", "/Groups/"+ServiceGroupName); err != nil {
			return -1, -1, fmt.Errorf("create the group: %w", err)
		}
		if err := run("dscl", ".", "-create", "/Groups/"+ServiceGroupName,
			"PrimaryGroupID", strconv.Itoa(serviceGID)); err != nil {
			return -1, -1, fmt.Errorf("set the group id: %w", err)
		}
	}

	if uid := lookupID("Users", ServiceUserName, "UniqueID"); uid < 0 {
		steps := [][]string{
			{"-create", "/Users/" + ServiceUserName},
			{"-create", "/Users/" + ServiceUserName, "UniqueID", strconv.Itoa(serviceUID)},
			{"-create", "/Users/" + ServiceUserName, "PrimaryGroupID", strconv.Itoa(serviceGID)},
			// No login shell and no real home: this account can own a process and
			// a directory, and do nothing else.
			{"-create", "/Users/" + ServiceUserName, "UserShell", "/usr/bin/false"},
			{"-create", "/Users/" + ServiceUserName, "NFSHomeDirectory", "/var/empty"},
			{"-create", "/Users/" + ServiceUserName, "RealName", "AI Usage Logger service"},
			{"-create", "/Users/" + ServiceUserName, "IsHidden", "1"},
		}
		for _, step := range steps {
			if err := run("dscl", append([]string{"."}, step...)...); err != nil {
				return -1, -1, fmt.Errorf("create the service account: %w", err)
			}
		}
	}

	uid, gid = ServiceAccount()
	if uid < 0 || gid < 0 {
		return -1, -1, fmt.Errorf("the service account was not created")
	}

	return uid, gid, nil
}

// DeleteServiceAccount removes it again, for uninstall.
func DeleteServiceAccount() error {
	if lookupID("Users", ServiceUserName, "UniqueID") >= 0 {
		if err := run("dscl", ".", "-delete", "/Users/"+ServiceUserName); err != nil {
			return fmt.Errorf("delete the service account: %w", err)
		}
	}
	if lookupID("Groups", ServiceGroupName, "PrimaryGroupID") >= 0 {
		if err := run("dscl", ".", "-delete", "/Groups/"+ServiceGroupName); err != nil {
			return fmt.Errorf("delete the service group: %w", err)
		}
	}

	return nil
}

// lookupID reads one numeric attribute from the directory service. -1 means the
// record does not exist.
func lookupID(kind, name, attribute string) int {
	out, err := exec.Command("dscl", ".", "-read", "/"+kind+"/"+name, attribute).Output()
	if err != nil {
		return -1
	}

	_, value, found := strings.Cut(string(out), ":")
	if !found {
		return -1
	}

	id, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return -1
	}

	return id
}
