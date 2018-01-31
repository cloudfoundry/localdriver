// +build windows

package oshelper

type osHelper struct {
}

func NewOsHelper() OsHelper {
	return &osHelper{}
}

func (o *osHelper) Umask(mask int) (oldmask int) {
}
