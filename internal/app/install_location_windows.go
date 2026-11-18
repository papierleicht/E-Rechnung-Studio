//go:build windows

package app

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

// runningApp returns the .exe the process runs from.
func runningApp() (string, bool) {
	exe, err := os.Executable()
	if err != nil {
		return "", false
	}
	return exe, true
}

// recommendedTarget is the per-user programs folder. It needs no
// administrator rights, unlike C:\Program Files, where updates would fail.
func recommendedTarget(current string) (string, bool) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return "", false
	}
	return filepath.Join(base, "Programs", "papierleicht", filepath.Base(current)), true
}

func copyApp(src, dst string) error { return copyExecutable(src, dst) }

// relaunchAfterExit waits for pid to end, adds a Start menu entry for target
// and starts it. Paths travel in environment variables, not in the script.
func relaunchAfterExit(target string, pid int) error {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return errors.New("APPDATA is not set")
	}
	link := filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "papierleicht.lnk")
	script := `Wait-Process -Id $env:ERS_PID -Timeout 60 -ErrorAction SilentlyContinue; ` +
		`try { $s = (New-Object -ComObject WScript.Shell).CreateShortcut($env:ERS_LINK); ` +
		`$s.TargetPath = $env:ERS_TARGET; $s.WorkingDirectory = Split-Path $env:ERS_TARGET; $s.Save() } catch {}; ` +
		`Start-Process -FilePath $env:ERS_TARGET`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-Command", script)
	cmd.Env = append(os.Environ(), "ERS_PID="+strconv.Itoa(pid), "ERS_TARGET="+target, "ERS_LINK="+link)
	const (
		detachedProcess        = 0x00000008
		createNoWindow         = 0x08000000
		createNewProcessGroup  = 0x00000200
		createBreakawayFromJob = 0x01000000
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: detachedProcess | createNoWindow | createNewProcessGroup | createBreakawayFromJob}
	if err := cmd.Start(); err != nil {
		// Some job objects forbid breaking away; start normally then.
		cmd = exec.Command(cmd.Path, cmd.Args[1:]...)
		cmd.Env = append(os.Environ(), "ERS_PID="+strconv.Itoa(pid), "ERS_TARGET="+target, "ERS_LINK="+link)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: detachedProcess | createNoWindow | createNewProcessGroup}
		return cmd.Start()
	}
	return nil
}
