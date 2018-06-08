package process

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

type ConfigEntry struct {
	ConfigDir string
	Name      string
	Arguments []string
	KeyValues map[string]string
	Envs      map[string]string
}

// dump the configuration as string
func (c *ConfigEntry) String() string {
	buf := bytes.NewBuffer(make([]byte, 0))
	fmt.Fprintf(buf, "configDir=%s\n", c.ConfigDir)
	for k, v := range c.KeyValues {
		fmt.Fprintf(buf, "%s=%s\n", k, v)
	}
	return buf.String()

}

func (c *ConfigEntry) GetEnv() []string {
	v := make([]string, 0, len(c.Envs))
	for key, value := range c.Envs {
		v = append(v, fmt.Sprintf("%s=%s", key, value))
	}
	return v
}

func (c *ConfigEntry) GetArgs() []string {
	return c.Arguments
}

// get value of key as bool
func (c *ConfigEntry) GetBool(key string, defValue bool) bool {
	value, ok := c.KeyValues[key]

	if ok {
		b, err := strconv.ParseBool(value)
		if err == nil {
			return b
		}
	}
	return defValue
}

// check if has parameter
func (c *ConfigEntry) HasParameter(key string) bool {
	_, ok := c.KeyValues[key]
	return ok
}

func toInt(s string, factor int, defValue int) int {
	i, err := strconv.Atoi(s)
	if err == nil {
		return i * factor
	}
	return defValue
}

// get the value of the key as int
func (c *ConfigEntry) GetInt(key string, defValue int) int {
	value, ok := c.KeyValues[key]

	if ok {
		return toInt(value, 1, defValue)
	}
	return defValue
}

//get the value of key as string
func (c *ConfigEntry) GetString(key string, defValue string) string {
	if s, ok := c.KeyValues[key]; ok {
		return s
	}
	return defValue
}

func (c *ConfigEntry) GetStringArray(key string, sep string) []string {
	s, ok := c.KeyValues[key]

	if ok {
		return strings.Split(s, sep)
	}
	return make([]string, 0)
}

// get the value of key as the bytes setting.
//
//	logSize=1MB
//	logSize=1GB
//	logSize=1KB
//	logSize=1024
//
func (c *ConfigEntry) GetBytes(key string, defValue int) int {
	v, ok := c.KeyValues[key]

	if ok {
		if len(v) > 2 {
			lastTwoBytes := v[len(v)-2:]
			if lastTwoBytes == "MB" {
				return toInt(v[:len(v)-2], 1024*1024, defValue)
			} else if lastTwoBytes == "GB" {
				return toInt(v[:len(v)-2], 1024*1024*1024, defValue)
			} else if lastTwoBytes == "KB" {
				return toInt(v[:len(v)-2], 1024, defValue)
			}
		}
		return toInt(v, 1, defValue)
	}
	return defValue
}
