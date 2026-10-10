package opencode

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type instance struct {
	pid        int
	started    time.Time
	directory  string
	endpoint   string
	sessionID  string
	continuing bool
}

func (instance instance) identity() string {
	return fmt.Sprintf("process:%d:%d", instance.pid, instance.started.Unix())
}

// OpenCode chooses its own port, so discover listeners belonging to its live
// processes instead of probing arbitrary ports or assuming the default is free.
func discoverInstances(ctx context.Context) ([]instance, error) {
	command := exec.CommandContext(ctx, "ps", "-A", "-o", "pid=,lstart=,command=")
	command.Env = append(os.Environ(), "LC_ALL=C")
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("list OpenCode processes: %w", err)
	}
	instances := parseProcesses(string(output))
	if len(instances) == 0 {
		return nil, nil
	}
	ids := make([]string, len(instances))
	for i, instance := range instances {
		ids[i] = strconv.Itoa(instance.pid)
	}
	pids := strings.Join(ids, ",")
	directories, err := readOpenFiles(ctx, "-a", "-p", pids, "-d", "cwd", "-Fpn")
	if err != nil {
		return nil, err
	}
	listeners, err := readOpenFiles(ctx, "-nP", "-a", "-p", pids, "-iTCP", "-sTCP:LISTEN", "-Fpn")
	if err != nil {
		return nil, err
	}
	for i := range instances {
		pid := instances[i].pid
		if names := directories[pid]; len(names) > 0 {
			instances[i].directory = names[0]
		}
		for _, name := range listeners[pid] {
			host, port, err := net.SplitHostPort(name)
			if err != nil {
				continue
			}
			// Only reach the process over loopback, even when it binds all interfaces.
			if host == "*" || host == "0.0.0.0" {
				host = "127.0.0.1"
			}
			if host == "::" {
				host = "::1"
			}
			ip := net.ParseIP(host)
			if ip != nil && ip.IsLoopback() {
				instances[i].endpoint = "http://" + net.JoinHostPort(host, port)
				break
			}
		}
	}
	return instances, nil
}

func readOpenFiles(ctx context.Context, args ...string) (map[int][]string, error) {
	output, err := exec.CommandContext(ctx, "lsof", args...).Output()
	// lsof exits 1 when no matching files exist, including TUIs without a listener.
	var exitError *exec.ExitError
	if err != nil && !(errors.As(err, &exitError) && exitError.ExitCode() == 1) {
		return nil, fmt.Errorf("locate OpenCode files (requires lsof): %w", err)
	}
	return parseOpenFiles(string(output)), ctx.Err()
}

func parseOpenFiles(output string) map[int][]string {
	files := make(map[int][]string)
	pid := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "p") {
			pid, _ = strconv.Atoi(line[1:])
		}
		if strings.HasPrefix(line, "n") && pid > 0 {
			files[pid] = append(files[pid], line[1:])
		}
	}
	return files
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
			executable = filepath.Base(command[0])
		}
		if executable != "opencode" && executable != "opencode.exe" {
			continue
		}
		// Utility commands are not interactive sessions or persistent servers.
		if len(command) > 1 && isUtilityCommand(command[1]) {
			continue
		}

		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid <= 0 {
			continue
		}
		started, err := time.ParseInLocation("Mon Jan _2 15:04:05 2006", strings.Join(fields[1:6], " "), time.Local)
		if err != nil {
			continue
		}
		entry := instance{pid: pid, started: started}
		for i, arg := range command[1:] {
			if (arg == "--session" || arg == "-s") && i+2 < len(command) {
				entry.sessionID = command[i+2]
			}
			if strings.HasPrefix(arg, "--session=") {
				entry.sessionID = strings.TrimPrefix(arg, "--session=")
			}
			if arg == "--continue" || arg == "-c" {
				entry.continuing = true
			}
		}
		instances = append(instances, entry)
	}
	return instances
}

func isUtilityCommand(command string) bool {
	switch command {
	case "completion", "debug", "providers", "auth", "models", "agent", "session", "stats", "export", "import":
		return true
	case "db", "upgrade", "uninstall", "mcp", "plugin", "--version", "--help", "-v", "-h":
		return true
	default:
		return false
	}
}
