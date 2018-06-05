// Copyright 2015 Daniel Theophanes.
// Use of this source code is governed by a zlib-style
// license that can be found in the LICENSE file.

package service

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"text/template"
	"time"
)

const maxPathSize = 32 * 1024

const version = "darwin-launchd"

type darwinSystem struct{}

func (darwinSystem) String() string {
	return version
}
func (darwinSystem) Detect() bool {
	return true
}
func (darwinSystem) Interactive() bool {
	return interactive
}
func (darwinSystem) New(i Interface, c *Config) (Service, error) {
	s := &darwinLaunchdService{
		i:      i,
		Config: c,

		userService: c.Option.bool(optionUserService, optionUserServiceDefault),
	}

	return s, nil
}

func init() {
	ChooseSystem(darwinSystem{})
}

var interactive = false

func init() {
	var err error
	interactive, err = isInteractive()
	if err != nil {
		panic(err)
	}
}

func isInteractive() (bool, error) {
	// TODO: The PPID of Launchd is 1. The PPid of a service process should match launchd's PID.
	return os.Getppid() != 1, nil
}

type darwinLaunchdService struct {
	i Interface
	*Config

	userService bool
}

// Is a service installed
func (s *darwinLaunchdService) IsInstalled() bool {
	confPath, err := s.getServiceFilePath()
	if err != nil {
		return false
	}

	if _, err := os.Stat(confPath); err == nil {
		return true
	}
	return false
}

func (s *darwinLaunchdService) String() string {
	if len(s.DisplayName) > 0 {
		return s.DisplayName
	}
	return s.Name
}

func (s *darwinLaunchdService) getHomeDir() (string, error) {
	u, err := user.Current()
	if err == nil {
		return u.HomeDir, nil
	}

	// alternate methods
	homeDir := os.Getenv("HOME") // *nix
	if homeDir == "" {
		return "", errors.New("User home directory not found.")
	}
	return homeDir, nil
}

func (s *darwinLaunchdService) getServiceFilePath() (string, error) {
	if s.userService {
		homeDir, err := s.getHomeDir()
		if err != nil {
			return "", err
		}
		return homeDir + "/Library/LaunchAgents/" + s.Name + ".plist", nil
	}
	return "/Library/LaunchDaemons/" + s.Name + ".plist", nil
}

func (s *darwinLaunchdService) Install() error {
	confPath, err := s.getServiceFilePath()
	if err != nil {
		return err
	}

	if s.IsInstalled() {
		return fmt.Errorf("Init already exists: %s", confPath)
	}

	if s.userService {
		// Ensure that ~/Library/LaunchAgents exists.
		if err := os.MkdirAll(filepath.Dir(confPath), 0700); err != nil {
			return err
		}
	}

	f, err := os.Create(confPath)
	if err != nil {
		return err
	}
	defer f.Close()

	path, err := s.execPath()
	if err != nil {
		return err
	}

	cmd := strings.Split(path, " ")
	if len(cmd) > 1 {
		path = cmd[0]
		s.Config.Arguments = append(cmd[1:], s.Config.Arguments...)
	}
	if filepath.Base(path) == path { //check IsAbs
		if newpath, err := exec.LookPath(path); err == nil {
			path = newpath
		}
	}

	var to = &struct {
		*Config
		Path string

		KeepAlive, RunAtLoad bool
		SessionCreate        bool
		StandardOutPath      string
		StandardErrorPath    string
	}{
		Config:            s.Config,
		Path:              path,
		KeepAlive:         s.Option.bool(optionKeepAlive, optionKeepAliveDefault),
		RunAtLoad:         s.Option.bool(optionRunAtLoad, optionRunAtLoadDefault),
		SessionCreate:     s.Option.bool(optionSessionCreate, optionSessionCreateDefault),
		StandardOutPath:   s.Option.string(optionStandardOutPath, ""),
		StandardErrorPath: s.Option.string(optionStandardErrorPath, ""),
	}

	functions := template.FuncMap{
		"bool": func(v bool) string {
			if v {
				return "true"
			}
			return "false"
		},
	}
	t := template.Must(template.New("launchdConfig").Funcs(functions).Parse(launchdConfig))
	return t.Execute(f, to)
}

func (s *darwinLaunchdService) Uninstall() error {
	s.Stop()

	confPath, err := s.getServiceFilePath()
	if err != nil {
		return err
	}

	run("launchctl", "remove", confPath)

	return os.Remove(confPath)
}

func (s *darwinLaunchdService) Start() error {
	confPath, err := s.getServiceFilePath()
	if err != nil {
		return err
	}
	return run("launchctl", "load", confPath)
}

func (s *darwinLaunchdService) Stop() error {
	confPath, err := s.getServiceFilePath()
	if err != nil {
		return err
	}
	return run("launchctl", "unload", confPath)
}

func (s *darwinLaunchdService) Restart() error {
	err := s.Stop()
	if err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)
	return s.Start()
}

func (s *darwinLaunchdService) Run() error {
	var err error

	err = s.i.Start(s)
	if err != nil {
		return err
	}

	s.Option.funcSingle(optionRunWait, func() {
		var sigChan = make(chan os.Signal, 3)
		signal.Notify(sigChan, syscall.SIGTERM, os.Interrupt)
		<-sigChan
	})()

	return s.i.Stop(s)
}

func (s *darwinLaunchdService) Logger(errs chan<- error) (Logger, error) {
	if interactive {
		return ConsoleLogger, nil
	}
	return s.SystemLogger(errs)
}

func (s *darwinLaunchdService) SystemLogger(errs chan<- error) (Logger, error) {
	return newSysLogger(s.Name, errs)
}

func (s *darwinLaunchdService) PID() (int, error) {
	return s.checkRunning()
}

func (s *darwinLaunchdService) Update() error {
	s.Uninstall()
	return s.Install()
}

// Check service is running
func (s *darwinLaunchdService) checkRunning() (int, error) {
	output, err := exec.Command("launchctl", "list", s.Name).Output()
	if err != nil {
		return -1, err
	}
	if matched, err := regexp.MatchString(s.Name, string(output)); err == nil && matched {
		reg := regexp.MustCompile("PID\" = ([0-9]+);")
		data := reg.FindStringSubmatch(string(output))
		if len(data) > 1 {
			return strconv.Atoi(data[1])
		}
		return 0, nil
	}
	return -1, ErrServiceIsNotRunning
}

func (s *darwinLaunchdService) Status() (string, error) {
	if !s.IsInstalled() {
		return "", ErrServiceIsNotInstalled
	}

	pid, err := s.checkRunning()
	if err != nil {
		return "", err
	}
	return "running (pid: " + strconv.Itoa(pid) + ")", nil
}

var launchdConfig = `<?xml version='1.0' encoding='UTF-8'?>
<!DOCTYPE plist PUBLIC "-//Apple Computer//DTD PLIST 1.0//EN"
"http://www.apple.com/DTDs/PropertyList-1.0.dtd" >
<plist version='1.0'>
<dict>
<key>Label</key><string>{{html .Name}}</string>
    <key>EnvironmentVariables</key>
    <dict>
{{ range $key, $value := .Config.Envs }}<key>{{ $key }}</key>
        <string>{{ $value }}</string>
{{ end }}
    </dict>

<key>ProgramArguments</key>
<array>
        <string>{{html .Path}}</string>
{{range .Config.Arguments}}
        <string>{{html .}}</string>
{{end}}
</array>
{{if .UserName}}<key>UserName</key><string>{{html .UserName}}</string>{{end}}
{{if .ChRoot}}<key>RootDirectory</key><string>{{html .ChRoot}}</string>{{end}}
{{if .WorkingDirectory}}<key>WorkingDirectory</key><string>{{html .WorkingDirectory}}</string>{{end}}
{{if .StandardOutPath}}<key>StandardOutPath</key><string>{{html .StandardOutPath}}</string>{{end}}
{{if .StandardErrorPath}}<key>StandardErrorPath</key><string>{{html .StandardErrorPath}}</string>{{end}}
<key>SessionCreate</key><{{bool .SessionCreate}}/>
<key>KeepAlive</key><{{bool .KeepAlive}}/>
<key>RunAtLoad</key><{{bool .RunAtLoad}}/>
<key>Disabled</key><false/>
</dict>
</plist>
`
