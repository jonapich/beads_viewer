//go:build windows

package api

import (
	"errors"
	"os/exec"
	"strings"
	"syscall"
)

// explorerCommandLine is the command line that selects abs in Explorer: explorer wants the path
// quoted after the comma (`explorer.exe /select,"C:\a b\c.png"`), which Go's own argument
// quoting does not produce, so the line is written out whole. A quote, a line break or NUL in the
// path is refused (a Windows file name cannot hold a quote, so this never rejects a real file).
func explorerCommandLine(abs string) (string, error) {
	if abs == "" || strings.ContainsAny(abs, "\"\r\n\x00") {
		return "", errors.New("path cannot be passed to explorer")
	}
	return `explorer.exe /select,"` + abs + `"`, nil
}

// defaultReveal runs explorer.exe /select. Explorer's exit status is not meaningful (it is 1 on
// success), so the process is only started and reaped. It selects the file; it never opens it.
func defaultReveal(abs string) error {
	line, err := explorerCommandLine(abs)
	if err != nil {
		return err
	}
	cmd := exec.Command("explorer.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: line}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
