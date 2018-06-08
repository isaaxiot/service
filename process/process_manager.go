package process

import (
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"
)

type ProcessManager struct {
	procs map[string]*Process
	lock  sync.Mutex
}

func NewProcessManager() *ProcessManager {
	return &ProcessManager{procs: make(map[string]*Process)}
}

func (pm *ProcessManager) CreateProcess(config *ConfigEntry) *Process {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	return pm.createProgram(config)
}

func (pm *ProcessManager) StartAutoStartPrograms() {
	pm.ForEachProcess(func(proc *Process) {
		if proc.isAutoStart() {
			proc.Start(false)
		}
	})
}

func (pm *ProcessManager) createProgram(config *ConfigEntry) *Process {
	procName := config.Name

	proc, ok := pm.procs[procName]

	if !ok {
		proc = NewProcess(config)
		pm.procs[procName] = proc
	}
	log.Info("create process:", procName)
	return proc
}

func (pm *ProcessManager) Add(name string, proc *Process) {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	pm.procs[name] = proc
	log.Info("add process:", name)
}

// remove the process from the manager
//
// Arguments:
// name - the name of program
//
// Return the process or nil
func (pm *ProcessManager) Remove(name string) *Process {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	proc, _ := pm.procs[name]
	delete(pm.procs, name)
	log.Info("remove process:", name)
	return proc
}

// return process if found or nil if not found
func (pm *ProcessManager) Find(name string) *Process {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	proc, ok := pm.procs[name]
	if ok {
		log.Debug("succeed to find process:", name)
	} else {
		//remove group field if it is included
		if pos := strings.Index(name, ":"); pos != -1 {
			proc, ok = pm.procs[name[pos+1:]]
		}
		if !ok {
			log.Info("fail to find process:", name)
		}
	}
	return proc
}

// clear all the processes
func (pm *ProcessManager) Clear() {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	pm.procs = make(map[string]*Process)
}

func (pm *ProcessManager) ForEachProcess(procFunc func(p *Process)) {
	pm.lock.Lock()
	defer pm.lock.Unlock()

	procs := pm.getAllProcess()
	for _, proc := range procs {
		procFunc(proc)
	}
}

func (pm *ProcessManager) getAllProcess() []*Process {
	tmpProcs := make([]*Process, 0)
	for _, proc := range pm.procs {
		tmpProcs = append(tmpProcs, proc)
	}
	return tmpProcs
}

func (pm *ProcessManager) StopAllProcesses() {
	pm.ForEachProcess(func(proc *Process) {
		proc.Stop(true)
	})
}
