// +build windows

package oshelper

import "code.cloudfoundry.org/localdriver"

type osHelper struct {
}

func NewOsHelper() localdriver.OsHelper {
	return &osHelper{}
}

func (o *osHelper) Umask(mask int) (oldmask int) {
	return 0
}
