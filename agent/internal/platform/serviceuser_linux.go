//go:build linux

package platform

import (
	"fmt"
	"os/user"
	"strconv"
)

// The worker's account, the same name as on macOS so paths and messages match.
// useradd accepts the leading underscore; Debian's friendlier adduser would not,
// which is why this uses useradd.
const (
	ServiceUserName  = "_aiul"
	ServiceGroupName = "_aiul"
)

// ServiceAccount reports the uid and gid of our service account, creating nothing.
// -1 means it does not exist yet.
func ServiceAccount() (uid, gid int) {
	uid, gid = -1, -1
	if u, err := user.Lookup(ServiceUserName); err == nil {
		uid, _ = strconv.Atoi(u.Uid)
	}
	if g, err := user.LookupGroup(ServiceGroupName); err == nil {
		gid, _ = strconv.Atoi(g.Gid)
	}
	return uid, gid
}

func CreateServiceAccountCommands() []string {
	return []string{
		fmt.Sprintf("sudo useradd --system --user-group --no-create-home --home-dir /nonexistent --shell /usr/sbin/nologin %s", ServiceUserName),
	}
}

// CreateServiceAccount makes the account if it is not already there.
//
// --system picks an id below 1000, so it never shows on the login screen and
// never counts as a desktop user. No home, no login shell.
func CreateServiceAccount() (uid, gid int, err error) {
	if uid, gid := ServiceAccount(); uid < 0 {
		if err := run("useradd", "--system", "--user-group", "--no-create-home",
			"--home-dir", "/nonexistent", "--shell", "/usr/sbin/nologin",
			"--comment", "AI Usage Logger service", ServiceUserName); err != nil {
			return -1, -1, fmt.Errorf("create the service account: %w", err)
		}
	} else if gid < 0 {
		return -1, -1, fmt.Errorf("%s exists but its group %s does not", ServiceUserName, ServiceGroupName)
	}

	uid, gid = ServiceAccount()
	if uid < 0 || gid < 0 {
		return -1, -1, fmt.Errorf("the service account was not created")
	}
	return uid, gid, nil
}

// DeleteServiceAccount removes it again; userdel also removes the group it made.
func DeleteServiceAccount() error {
	if uid, _ := ServiceAccount(); uid < 0 {
		return nil
	}
	if err := run("userdel", ServiceUserName); err != nil {
		return fmt.Errorf("delete the service account: %w", err)
	}
	return nil
}
