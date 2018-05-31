//+build !go1.8

package service

import (
	"github.com/kardianos/osext"
)

func (c *Config) execPath() (string, error) {
	if len(c.Executable) != 0 {
		return c.Executable, nil
	}
	return osext.Executable()
}
