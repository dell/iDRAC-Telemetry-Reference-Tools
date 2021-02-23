// Licensed to You under the Apache License, Version 2.0.

// +build linux

package ps

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type UnixProcess struct {
	pid   int
	ppid  int
	state rune
	pgrp  int
	sid   int

	binary string
}

func (p *UnixProcess) Pid() int {
	return p.pid
}

func (p *UnixProcess) PPid() int {
	return p.ppid
}

func (p *UnixProcess) Executable() string {
	return p.binary
}

func (p *UnixProcess) Running() bool {
	return p.pid != -1
}

func (p *UnixProcess) Enabled() bool {
	cmd := exec.Command("systemctl", "is-enabled", p.binary)
	err := cmd.Run()
	if err != nil {
		log.Print("Error: ", err)
		return false
	}
	return true
}

func processes(whitelist []string) (map[string]Process, error) {
	d, err := os.Open("/proc")
	if err != nil {
		return nil, err
	}
	defer d.Close()

	results := make(map[string]Process)
	for {
		fis, err := d.Readdir(10)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		for _, fi := range fis {
			// We only care about directories, since all pids are dirs
			if !fi.IsDir() {
				continue
			}

			// We only care if the name starts with a numeric
			name := fi.Name()
			if name[0] < '0' || name[0] > '9' {
				continue
			}

			// From this point forward, any errors we just ignore, because
			// it might simply be that the process doesn't exist anymore.
			pid, err := strconv.ParseInt(name, 10, 0)
			if err != nil {
				continue
			}

			p, err := newUnixProcess(int(pid), whitelist)
			if err != nil {
				continue
			}
			var proc Process
			proc.Pid = p.pid
			proc.Running = true
			proc.Enabled = p.Enabled()
			results[p.binary] = proc
		}
	}
	for _, entry := range whitelist {
		_, ok := results[entry]
		if !ok {
			var proc Process
			proc.Pid = -1
			proc.Running = false
			proc.Enabled = false
			results[entry] = proc
		}
	}

	return results, nil
}

func newUnixProcess(pid int, whitelist []string) (*UnixProcess, error) {
	p := &UnixProcess{pid: pid}
	return p, p.Refresh(whitelist)
}

// Refresh reloads all the data associated with this process.
func (p *UnixProcess) Refresh(whitelist []string) error {
	statPath := fmt.Sprintf("/proc/%d/stat", p.pid)
	dataBytes, err := ioutil.ReadFile(statPath)
	if err != nil {
		return err
	}

	// First, parse out the image name
	data := string(dataBytes)
	binStart := strings.IndexRune(data, '(') + 1
	binEnd := strings.IndexRune(data[binStart:], ')')
	p.binary = data[binStart : binStart+binEnd]

	found := false
	for _, entry := range whitelist {
		if p.binary == entry {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("Process %s not in whitelist", p.binary)
	}

	// Move past the image name and start parsing the rest
	data = data[binStart+binEnd+2:]
	_, err = fmt.Sscanf(data,
		"%c %d %d %d",
		&p.state,
		&p.ppid,
		&p.pgrp,
		&p.sid)

	return err
}
