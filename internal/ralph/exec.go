package ralph

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
)

// runIn runs name with args in dir, wiring stdout/stderr to the current
// process. Pass "" for dir to inherit cwd.
func runIn(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runInQuiet is like runIn but discards stdout/stderr — used for predicate
// commands like `git show-ref --quiet`.
func runInQuiet(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	return cmd.Run()
}

// outputIn captures stdout (stderr surfaces in the returned error).
func outputIn(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("%s: %w: %s", name, err, stderr.String())
		}
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return string(out), nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
