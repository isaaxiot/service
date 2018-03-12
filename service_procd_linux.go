package service

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strconv"
	"syscall"
	"text/template"
)

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

// Standard service path for systemV daemons
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
	output, err := exec.Command(u.servicePath(), "status").Output()
	if err == nil {
		if matched, err := regexp.MatchString("running", string(output)); err == nil && matched {
			reg := regexp.MustCompile("running ([0-9]+)")
			data := reg.FindStringSubmatch(string(output))
			if len(data) > 1 {
				return strconv.Atoi(data[1])
			}
			return -1, nil
		}
	}
	return -1, ErrServiceIsNotRunning
}

// Install the service
func (u *procd) Install() error {
	srvPath := u.servicePath()

	if u.IsInstalled() {
		return fmt.Errorf("Init already exists: %s", srvPath)
	}

	file, err := os.Create(srvPath)
	if err != nil {
		return err
	}
	defer file.Close()

	templ, err := template.New("procdConfig").Parse(procdConfig)
	if err != nil {
		return err
	}

	if err := templ.Execute(
		file,
		&struct {
			Name, Description string
			Args, WorkingDir  string
			Cmd               string
		}{
			Name:        u.Name,
			Cmd:         u.Executable,
			Description: u.Description,
			WorkingDir:  u.WorkingDirectory},
	); err != nil {
		return err
	}

	if err := os.Chmod(srvPath, 0755); err != nil {
		return err
	}
	file.Close()
	if u.Name == "isaaxd" {
		if err := exec.Command(u.servicePath(), "enable").Run(); err != nil {
			return err
		}
	}

	return nil
}

// Uninstall removes the service
func (u *procd) Uninstall() error {
	u.Stop()

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
func (u *procd) Status() error {
	if !u.IsInstalled() {
		return ErrServiceIsNotInstalled
	}
	pid, err := u.checkRunning()
	if err != nil {
		return err
	}
	fmt.Println("(pid: " + strconv.Itoa(pid) + ")")
	return nil
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

  procd_set_param limits core="unlimited"  # If you need to set ulimit for your process
  procd_close_instance
}
`
