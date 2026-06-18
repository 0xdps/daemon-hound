package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// TaskSchedulerManager implements ServiceManager for Windows Task Scheduler.
type TaskSchedulerManager struct{}

// Install creates a scheduled task for daemon.
func (t *TaskSchedulerManager) Install() error {
	daemonPath, err := GetDaemonPath()
	if err != nil {
		return err
	}

	taskName := "DaemonHound"

	// Check if already exists and delete
	t.Uninstall()

	// Create PowerShell script to register task
	taskXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.4" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Date>2026-01-01T00:00:00</Date>
    <Author>DaemonHound</Author>
    <Description>Background vault synchronization daemon</Description>
    <URI>\DaemonHound</URI>
  </RegistrationInfo>
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
    </LogonTrigger>
    <RegistrationTrigger>
      <Enabled>true</Enabled>
    </RegistrationTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>S-1-5-21-0-0-0-1000</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <IdleSettings>
      <Duration>PT10M</Duration>
      <WaitTimeout>PT1H</WaitTimeout>
      <StopOnIdleEnd>false</StopOnIdleEnd>
      <RestartOnIdle>false</RestartOnIdle>
    </IdleSettings>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>false</Hidden>
    <RunOnlyIfIdle>false</RunOnlyIfIdle>
    <DisallowStartOnRemoteAppSession>false</DisallowStartOnRemoteAppSession>
    <UseUnifiedSchedulingEngine>true</UseUnifiedSchedulingEngine>
    <WakeToRun>false</WakeToRun>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <DeleteExpiredTaskAfter>PT0S</DeleteExpiredTaskAfter>
    <RestartCount>3</RestartCount>
    <RestartInterval>PT1M</RestartInterval>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>%s</Command>
      <Arguments>daemon run</Arguments>
    </Exec>
  </Actions>
</Task>`, daemonPath)

	// Write XML to temporary file
	tmpDir := filepath.Join(os.Getenv("TEMP"), "daemon-hound")
	os.MkdirAll(tmpDir, 0755)
	xmlPath := filepath.Join(tmpDir, "daemon-hound-task.xml")

	if err := os.WriteFile(xmlPath, []byte(taskXML), 0644); err != nil {
		return fmt.Errorf("failed to write task XML: %w", err)
	}
	defer os.Remove(xmlPath)

	// Register task using schtasks
	cmd := exec.Command("schtasks", "/create", "/tn", taskName, "/xml", xmlPath, "/f")
	if output, err := cmd.CombinedOutput(); err != nil {
		// Check if already exists
		if strings.Contains(string(output), "already exists") {
			return nil
		}
		return fmt.Errorf("failed to create scheduled task: %w\n%s", err, output)
	}

	// Start the task
	startCmd := exec.Command("schtasks", "/run", "/tn", taskName)
	if err := startCmd.Run(); err != nil {
		// It's OK if it fails - task will run at next logon
		return nil
	}

	return nil
}

// Uninstall removes the scheduled task.
func (t *TaskSchedulerManager) Uninstall() error {
	taskName := "DaemonHound"

	cmd := exec.Command("schtasks", "/delete", "/tn", taskName, "/f")
	if output, err := cmd.CombinedOutput(); err != nil {
		// It's OK if it doesn't exist
		if strings.Contains(string(output), "cannot find") || strings.Contains(string(output), "does not exist") {
			return nil
		}
		return fmt.Errorf("failed to delete scheduled task: %w", err)
	}

	return nil
}

// IsInstalled checks if the daemon task is registered.
func (t *TaskSchedulerManager) IsInstalled() (bool, error) {
	taskName := "DaemonHound"

	cmd := exec.Command("schtasks", "/query", "/tn", taskName)
	if err := cmd.Run(); err != nil {
		return false, nil
	}

	return true, nil
}

// IsRunning checks if the daemon process is running.
func (t *TaskSchedulerManager) IsRunning() (bool, error) {
	cmd := exec.Command("tasklist", "/FI", "IMAGENAME eq dhd.exe")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, err
	}

	return strings.Contains(string(output), "dhd.exe"), nil
}
