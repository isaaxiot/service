package service

import (
	"fmt"
	"github.com/isaaxiot/service/process"
	"strconv"
)

var Supervise = false

func init() {
	ChooseSystem(supervisedSystem{})
}

type supervisedService struct {
	i Interface
	*Config
	procMgr *process.ProcessManager
}

type supervisedSystem struct{}

func (supervisedSystem) String() string {
	return "supervised-service"
}
func (supervisedSystem) Detect() bool {
	return Supervise
}
func (supervisedSystem) Interactive() bool {
	return false
}

func (supervisedSystem) New(i Interface, c *Config) (Service, error) {
	s := &supervisedService{
		i:       i,
		Config:  c,
		procMgr: process.NewProcessManager(),
	}
	s.procMgr.CreateProcess(s.parseConfig())
	return s, nil
}

func (s *supervisedService) String() string {
	if len(s.DisplayName) > 0 {
		return s.DisplayName
	}
	return s.Name
}

func (s *supervisedService) parseConfig() *process.ConfigEntry {
	redirect := "false"
	stderr := s.Option.string(optionStandardErrorPath, "")
	if stderr == "" {
		redirect = "true"
	}
	return &process.ConfigEntry{
		Name: s.Name,
		KeyValues: map[string]string{
			"command":         s.Config.Executable,
			"stdout_logfile":  s.Option.string(optionStandardOutPath, ""),
			"stderr_logfile":  stderr,
			"user":            s.Config.UserName,
			"directory":       s.Config.WorkingDirectory,
			"redirect_stderr": redirect,
			"autostart":       "true",
			"autorestart":     "true",
			"startretries":    "10",
		},
		Envs: s.Envs,
	}
}

func (s *supervisedService) Install() error {
	p := s.procMgr.Find(s.Name)
	if p == nil {
		return nil
	}
	p.Start(false)
	return nil
}

func (s *supervisedService) Uninstall() error {
	p := s.procMgr.Find(s.Name)
	if p == nil {
		return nil
	}
	if p.GetPid() == 0 {
		p.Attach()
	}
	p.Stop(false)
	return nil
}

func (s *supervisedService) Update() error {
	p := s.procMgr.Find(s.Name)
	if p == nil {
		return nil
	}
	p.Stop(false)
	s.procMgr.Remove(s.Name)
	p = s.procMgr.CreateProcess(s.parseConfig())
	p.Start(false)
	return nil
}

func (s *supervisedService) Run() error {
	p := s.procMgr.CreateProcess(s.parseConfig())
	if p == nil {
		return nil
	}
	p.Start(true)
	return s.i.Stop(s)
}

func (s *supervisedService) Start() error {
	p := s.procMgr.Find(s.Name)
	if p == nil {
		return fmt.Errorf("couldn't find program")
	}
	if s.Config.Executable == "" {
		return fmt.Errorf("empty executable")
	}
	if err := p.Attach(); err != nil {
		p.Start(false)
	}
	return nil
}

func (s *supervisedService) Stop() error {
	p := s.procMgr.Find(s.Name)
	if p == nil {
		p.Stop(true)
	}
	return nil
}

func (s *supervisedService) Restart() error {
	p := s.procMgr.Find(s.Name)
	if p == nil {
		return nil
	}
	p.Stop(true)
	p.Start(false)
	return nil
}

func (s *supervisedService) Logger(errs chan<- error) (Logger, error) {
	return ConsoleLogger, nil
}

func (s *supervisedService) SystemLogger(errs chan<- error) (Logger, error) {
	return ConsoleLogger, nil
}

func (s *supervisedService) checkRunning() (int, error) {
	p := s.procMgr.Find(s.Name)
	if p == nil {
		return -1, fmt.Errorf("couldn't find program")
	}
	pid := p.GetPid()
	if pid == 0 {
		if err := p.Attach(); err != nil {
			return 0, err
		}
		pid = p.GetPid()
	}
	return pid, nil
}

func (s *supervisedService) PID() (int, error) {
	return s.checkRunning()
}

func (s *supervisedService) Status() (string, error) {
	pid, err := s.checkRunning()
	if err != nil {
		return "", err
	}
	return "running (pid: " + strconv.Itoa(pid) + ")", nil
}
