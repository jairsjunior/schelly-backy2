package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/go-cmd/cmd"
)

type ShellContext struct {
	cmdRef *cmd.Cmd
}

func execShellTimeout(command string, timeout time.Duration, ctx *ShellContext) (string, error) {
	acmd := cmd.NewCmd("bash", "-c", command)
	statusChan := acmd.Start() // non-blocking
	if ctx != nil {
		ctx.cmdRef = acmd
	}

	//kill if taking too long
	if timeout != 0 {
		go func() {
			<-time.After(time.Duration(options.maxTimeRunning) * time.Second)
			logrus.Warnf("Stopping pre command execution because it is taking too long")
			acmd.Stop()
		}()
	}

	logrus.Debugf("Waiting for command to finish...")
	<-statusChan
	logrus.Debugf("Command finished")

	out := getCmdOutput(acmd)
	status := acmd.Status()
	logrus.Debugf("Output: %s", out)
	if status.Exit != 0 {
		return out, fmt.Errorf("Failed to run command: '%s'; exit=%d", command, status.Exit)
	} else {
		return out, nil
	}
}

func execShell(command string) (string, error) {
	return execShellTimeout(command, 0, nil)
}

func getCmdOutput(cmd *cmd.Cmd) string {
	status := cmd.Status()
	out := strings.Join(status.Stdout, "\n")
	out = out + "\n" + strings.Join(status.Stderr, "\n")
	return out
}

func mkDirs(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.MkdirAll(path, os.ModePerm)
	}
	return nil
}

// // sh is a simple os.exec Command tool, returns trimmed string output
// func sh(name string, args ...string) (string, error) {
// 	cmd := exec.Command(name, args...)
// 	logrus.Debugf("sh CMD: %q", cmd)
// 	out, err := cmd.Output()
// 	return strings.Trim(string(out), " \n"), err
// }

// // ShResult used for channel in timeout
// type ShResult struct {
// 	Output string // STDOUT
// 	Err    error  // go error, not STDERR
// }

// type ShTimeoutError struct {
// 	timeout time.Duration
// }

// func (e ShTimeoutError) Error() string {
// 	return fmt.Sprintf("Reached TIMEOUT on shell command")
// }

// // shWithDefaultTimeout will use the defaultShellTimeout so you dont have to pass one
// func shWithDefaultTimeout(name string, args ...string) (string, error) {
// 	return shWithTimeout(defaultShellTimeout, name, args...)
// }

// // shWithTimeout will run the Cmd and wait for the specified duration
// func shWithTimeout(howLong time.Duration, name string, args ...string) (string, error) {
// 	// duration can't be zero
// 	if howLong <= 0 {
// 		return "", fmt.Errorf("Timeout duration needs to be positive")
// 	}
// 	// set up the results channel
// 	resultsChan := make(chan ShResult, 1)
// 	logrus.Debugf("shWithTimeout: %v, %s, %v", howLong, name, args)

// 	// fire up the goroutine for the actual shell command
// 	go func() {
// 		out, err := sh(name, args...)
// 		resultsChan <- ShResult{Output: out, Err: err}
// 	}()

// 	select {
// 	case res := <-resultsChan:
// 		return res.Output, res.Err
// 	case <-time.After(howLong):
// 		return "", ShTimeoutError{timeout: howLong}
// 	}

// 	return "", nil
// }
