package pi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type instance struct {
	pid       int
	started   time.Time
	directory string
}

func (instance instance) identity() string {
	return fmt.Sprintf("process:%d:%d", instance.pid, instance.started.Unix())
}

func discoverInstances(ctx context.Context) ([]instance, error) {
	command := exec.CommandContext(ctx, "ps", "-A", "-o", "pid=,lstart=,command=")
	command.Env = append(os.Environ(), "LC_ALL=C")
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("list Pi processes: %w", err)
	}
	instances := parseProcesses(string(output))
	if len(instances) == 0 {
		return nil, nil
	}
	ids := make([]string, len(instances))
	for i, entry := range instances {
		ids[i] = strconv.Itoa(entry.pid)
	}
	output, err = exec.CommandContext(ctx, "lsof", "-a", "-p", strings.Join(ids, ","), "-d", "cwd", "-Fpn").Output()
	var exitError *exec.ExitError
	if err != nil && !(errors.As(err, &exitError) && exitError.ExitCode() == 1) {
		return nil, fmt.Errorf("locate Pi working directories (requires lsof): %w", err)
	}
	directories := make(map[int]string)
	pid := 0
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "p") {
			pid, _ = strconv.Atoi(line[1:])
		}
		if strings.HasPrefix(line, "n") && pid > 0 {
			directories[pid] = line[1:]
		}
	}
	for i := range instances {
		instances[i].directory = directories[instances[i].pid]
	}
	return instances, ctx.Err()
}

func parseProcesses(output string) []instance {
	var instances []instance
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 7 {
			continue
		}
		command := fields[6:]
		executable := filepath.Base(command[0])
		if executable == "node" && len(command) > 1 {
			command = command[1:]
			script := filepath.ToSlash(command[0])
			executable = filepath.Base(script)
			if strings.HasSuffix(script, "/pi-coding-agent/dist/cli.js") ||
				strings.HasSuffix(script, "/pi-coding-agent/dist/bundle/cli.js") {
				executable = "pi"
			}
		}
		if executable != "pi" && executable != "pi.exe" {
			continue
		}
		utility := false
		for _, arg := range command[1:] {
			switch arg {
			case "--version", "-v", "--help", "-h":
				utility = true
			}
		}
		if len(command) > 1 {
			switch command[1] {
			case "install", "remove", "uninstall", "update", "list", "config", "auth", "mcp":
				utility = true
			}
		}
		if utility {
			continue
		}
		pid, pidErr := strconv.Atoi(fields[0])
		started, timeErr := time.ParseInLocation("Mon Jan _2 15:04:05 2006",
			strings.Join(fields[1:6], " "), time.Local)
		if pidErr == nil && pid > 0 && timeErr == nil {
			instances = append(instances, instance{pid: pid, started: started})
		}
	}
	return instances
}
