package hermes

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
		return nil, fmt.Errorf("list Hermes processes: %w", err)
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
		return nil, fmt.Errorf("locate Hermes working directories (requires lsof): %w", err)
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
		if strings.HasPrefix(executable, "python") && len(command) > 1 {
			command = command[1:]
			for len(command) > 0 && (command[0] == "-u" || command[0] == "-B" || command[0] == "-I") {
				command = command[1:]
			}
			if len(command) == 0 {
				continue
			}
			if command[0] == "-m" && len(command) > 1 && command[1] == "hermes_cli.main" {
				command = command[1:]
				executable = "hermes"
			} else {
				executable = filepath.Base(command[0])
				script := filepath.ToSlash(command[0])
				if (executable == "cli.py" && strings.Contains(script, "/hermes-agent/")) ||
					strings.HasSuffix(script, "/hermes_cli/main.py") {
					executable = "hermes"
				}
			}
		}
		if (executable != "hermes" && executable != "hermes.exe") || !interactiveArguments(command[1:]) {
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

// Hermes defaults to chat. Skip option values before deciding whether a positional
// argument selects chat or a utility, including profile-prefixed shell launches.
func interactiveArguments(arguments []string) bool {
	for i := 0; i < len(arguments); i++ {
		argument := arguments[i]
		switch argument {
		case "--help", "-h", "--version":
			return false
		case "-m", "--model", "--provider", "--reasoning", "-t", "--toolsets", "--resume", "-r", "--in",
			"-p", "--profile", "-s", "--skills", "-c", "--continue", "-q", "--query", "-z", "--query-file",
			"--max-turns", "--run-budget", "--format", "--source", "--usage-file":
			if i+1 < len(arguments) && !strings.HasPrefix(arguments[i+1], "-") {
				i++
			}
		default:
			if !strings.HasPrefix(argument, "-") {
				if argument != "chat" {
					return false
				}
			}
		}
	}
	return true
}
