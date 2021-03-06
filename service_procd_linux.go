package service

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"text/template"
)

func isProcd() bool {
	if _, err := os.Stat("/sbin/procd"); err == nil {
		return true
	}
	return false
}

func newProcdService(i Interface, c *Config) (Service, error) {
	u := &procd{
		i:      i,
		Config: c,
	}

	return u, nil
}

func (u *procd) String() string {
	if len(u.DisplayName) > 0 {
		return u.DisplayName
	}
	return u.Name
}

// procd - standard record (struct) for linux procd version of daemon package
type procd struct {
	i Interface
	*Config
}

// Standard service path for procd daemons
func (u *procd) servicePath() string {
	return "/etc/init.d/" + u.Name
}

// Is a service installed
func (u *procd) IsInstalled() bool {
	if _, err := os.Stat(u.servicePath()); err == nil {
		return true
	}
	return false
}

func (u *procd) ServiceName() string {
	return u.Name
}

// Check service is running
func (u *procd) checkRunning() (int, error) {
	pidfile := fmt.Sprintf("/var/run/%s.pid", u.Name)
	pid, err := ioutil.ReadFile(pidfile)
	if err != nil {
		return -1, ErrServiceIsNotRunning
	}
	if string(pid) == "" {
		os.Remove(pidfile)
		return -1, ErrServiceIsNotRunning
	}
	intpid, _ := strconv.Atoi(strings.TrimSpace(string(pid)))
	proc, err := os.FindProcess(intpid)
	if err == nil {
		err = proc.Signal(syscall.Signal(0))
	}
	if err != nil {
		os.Remove(pidfile)
		return -1, err
	}
	return intpid, nil
}

// Install the service
func (u *procd) Install() error {
	srvPath := u.servicePath()

	if u.IsInstalled() {
		u.Uninstall()
	}

	file, err := os.Create(srvPath)
	if err != nil {
		return err
	}
	defer file.Close()

	templ, err := template.New("procdConfig").Funcs(template.FuncMap{"StringsJoin": strings.Join}).Parse(procdConfig)
	if err != nil {
		return err
	}
	envs := make([]string, 0, len(u.Config.Envs))
	if len(u.Config.Envs) > 0 {
		for k, v := range u.Config.Envs {
			envs = append(envs, fmt.Sprintf(`%s="%s"`, k, v))
		}
	}

	if err := templ.Execute(
		file,
		&struct {
			Name, Description string
			Args, WorkingDir  string
			Cmd               string
			Envs              []string
		}{
			Name:        u.Name,
			Cmd:         u.Executable,
			Description: u.Description,
			WorkingDir:  u.WorkingDirectory,
			Envs:        envs,
		},
	); err != nil {
		return err
	}

	if err := os.Chmod(srvPath, 0755); err != nil {
		return err
	}
	file.Close()

	run(u.servicePath(), "enable")
	return nil
}

// Uninstall removes the service
func (u *procd) Uninstall() error {
	u.Stop()

	run(u.servicePath(), "disable")

	return os.Remove(u.servicePath())
}

func (u *procd) Run() (err error) {
	err = u.i.Start(u)
	if err != nil {
		return err
	}

	u.Option.funcSingle(optionRunWait, func() {
		var sigChan = make(chan os.Signal, 3)
		signal.Notify(sigChan, syscall.SIGTERM, os.Interrupt)
		<-sigChan
	})()

	return u.i.Stop(u)
}

func (u *procd) Restart() error {
	if err := exec.Command(u.servicePath(), "restart").Run(); err != nil {
		return err
	}
	return nil
}

func (u *procd) Update() error {
	u.Uninstall()
	return u.Install()
}

func (u *procd) PID() (int, error) {
	return u.checkRunning()
}

func (u *procd) Logger(errs chan<- error) (Logger, error) {
	if system.Interactive() {
		return ConsoleLogger, nil
	}
	return u.SystemLogger(errs)
}

func (u *procd) SystemLogger(errs chan<- error) (Logger, error) {
	return newSysLogger(u.Name, errs)
}

// Start the service
func (u *procd) Start() error {
	if !u.IsInstalled() {
		return ErrServiceIsNotInstalled
	}
	if err := exec.Command(u.servicePath(), "start").Run(); err != nil {
		return err
	}
	return nil
}

// Stop the service
func (u *procd) Stop() error {
	if !u.IsInstalled() {
		return ErrServiceIsNotInstalled
	}
	if err := exec.Command(u.servicePath(), "stop").Run(); err != nil {
		return err
	}
	return nil
}

// Status - Get service status
func (u *procd) Status() (string, error) {
	pid, err := u.checkRunning()
	if err != nil {
		return "", err
	}
	return "(pid: " + strconv.Itoa(pid) + ")", nil
}

var procdConfig = `#!/bin/sh /etc/rc.common

# {{.Name}} {{.Description}}
USE_PROCD=1
START=120
STOP=120

start_service() {
  procd_open_instance
  procd_set_param command {{.Cmd}} {{.Args}}

  # respawn automatically if something died, be careful if you have an alternative process supervisor
  # if process dies sooner than respawn_threshold, it is considered crashed and after 5 retries the service is stopped
  procd_set_param respawn
  procd_set_param limits core="unlimited"
  procd_set_param stdout 1
  procd_set_param stderr 1
  procd_set_param pidfile /var/run/{{.Name}}.pid

{{if len .Envs}}
  procd_set_param env \
  {{ StringsJoin .Envs "\\\n " }}
{{end}}

  procd_close_instance
}
`
