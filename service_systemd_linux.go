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
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"text/template"
)

func isSystemd() bool {
	if _, err := os.Stat("/run/systemd/system"); err == nil {
		return true
	}
	return false
}

type systemd struct {
	i Interface
	*Config
}

func newSystemdService(i Interface, c *Config) (Service, error) {
	s := &systemd{
		i:      i,
		Config: c,
	}

	return s, nil
}

func (s *systemd) String() string {
	if len(s.DisplayName) > 0 {
		return s.DisplayName
	}
	return s.Name
}

// Systemd services should be supported, but are not currently.
var errNoUserServiceSystemd = errors.New("User services are not supported on systemd.")

func (s *systemd) configPath() (cp string, err error) {
	cp = "/etc/systemd/system/" + s.Config.Name + ".service"
	return
}

func (s *systemd) template() *template.Template {
	return template.Must(template.New("").Funcs(tf).Parse(systemdScript))
}

func (s *systemd) Install() error {
	confPath, err := s.configPath()
	if err != nil {
		return err
	}
	_, err = os.Stat(confPath)
	if err == nil {
		return fmt.Errorf("Init already exists: %s", confPath)
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

	var to = &struct {
		*Config
		Path              string
		ReloadSignal      string
		PIDFile           string
		StandardOutPath   string
		StandardErrorPath string
	}{
		s.Config,
		path,
		s.Option.string(optionReloadSignal, ""),
		s.Option.string(optionPIDFile, ""),
		s.Option.string(optionStandardOutPath, ""),
		s.Option.string(optionStandardErrorPath, ""),
	}

	err = s.template().Execute(f, to)
	if err != nil {
		return err
	}

	err = run("systemctl", "enable", s.Name+".service")
	if err != nil {
		return err
	}
	return run("systemctl", "daemon-reload")
}

func (s *systemd) Uninstall() error {
	err := run("systemctl", "disable", s.Name+".service")
	if err != nil {
		return err
	}
	cp, err := s.configPath()
	if err != nil {
		return err
	}
	if err := os.Remove(cp); err != nil {
		return err
	}
	return nil
}

func (s *systemd) Logger(errs chan<- error) (Logger, error) {
	if system.Interactive() {
		return ConsoleLogger, nil
	}
	return s.SystemLogger(errs)
}

func (s *systemd) SystemLogger(errs chan<- error) (Logger, error) {
	return newSysLogger(s.Name, errs)
}

func (s *systemd) Run() (err error) {
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

func (s *systemd) Start() error {
	return run("systemctl", "start", s.Name+".service")
}

func (s *systemd) Stop() error {
	return run("systemctl", "stop", s.Name+".service")
}

func (s *systemd) Restart() error {
	return run("systemctl", "restart", s.Name+".service")
}

func (s *systemd) PID() (int, error) {
	return s.checkRunning()
}

func (s *systemd) checkRunning() (int, error) {
	out, err := exec.Command("systemctl", "status", s.Name+".service").Output()
	if err != nil {
		return -1, err
	}
	if matched, err := regexp.MatchString("Active: active", string(out)); err == nil && matched {
		reg := regexp.MustCompile("Main PID: ([0-9]+)")
		data := reg.FindStringSubmatch(string(out))
		if len(data) > 1 {
			return strconv.Atoi(data[1])
		}
		return -1, nil
	}
	return -1, ErrServiceIsNotRunning
}

func (s *systemd) Status() (string, error) {
	pid, err := s.checkRunning()
	if err != nil {
		return "", err
	}
	return "running (pid: " + strconv.Itoa(pid) + ")", nil
}

func (s *systemd) Update() error {
	if err := run("systemctl", "daemon-reload"); err != nil {
		return err
	}

	if err := run("systemctl", "enable", s.Name+".service"); err != nil {
		return err
	}

	return nil
}

const systemdScript = `[Unit]
Description={{.Description}}
ConditionFileIsExecutable={{.Path|cmdEscape}}

[Service]
StartLimitInterval=5
StartLimitBurst=10
{{if .StandardOutPath}}
ExecStart=/bin/sh -c '{{.Path}} {{range .Arguments}} {{.|cmd}}{{end}} >>{{.StandardOutPath}} 2>&1'
{{else}}
ExecStart={{.Path|cmdEscape}}{{range .Arguments}} {{.|cmd}}{{end}}
{{end}}
{{if .ChRoot}}RootDirectory={{.ChRoot|cmd}}{{end}}
{{if .WorkingDirectory}}WorkingDirectory={{.WorkingDirectory|cmdEscape}}{{end}}
{{if .UserName}}User={{.UserName}}{{end}}
{{if .ReloadSignal}}ExecReload=/bin/kill -{{.ReloadSignal}} "$MAINPID"{{end}}
{{if .PIDFile}}PIDFile={{.PIDFile|cmd}}{{end}}
Restart=always
RestartSec=120
EnvironmentFile=-/etc/sysconfig/{{.Name}}

[Install]
WantedBy=multi-user.target
`
