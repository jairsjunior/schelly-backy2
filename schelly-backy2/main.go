package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/flaviostutz/schelly-webhook/schellyhook"
)

var sourcePath *string

//Backy2Backuper sample backuper
type Backy2Backuper struct {
}

func main() {
	logrus.Info("====Starting Backy2 REST server====")

	backy2Backuper := Backy2Backuper{}
	err := schellyhook.Initialize(backy2Backuper)
	if err != nil {
		logrus.Errorf("Error initializating Schellyhook. err=%s", err)
		os.Exit(1)
	}
}

//RegisterFlags register command line flags
func (sb Backy2Backuper) RegisterFlags() error {
	sourcePath = flag.String("source-path", "file:///backup-source/backup-this", "Backup source path. rbd://<pool>/<imagename>[@<snapshotname>] OR file:///[dir]/[file]")
	return nil
}

//Init create repo
func (sb Backy2Backuper) Init() error {
	err := mkDirs("/var/lib/backy2/ids")
	if err != nil {
		logrus.Errorf("Couldn't create id references dir at /var/lib/backy2/ids. err=%s", err)
		return err
	}

	logrus.Debugf("Checking if Backy2 repo was already initialized")
	result, err := schellyhook.ExecShell("backy2 ls")
	if err != nil {
		logrus.Debugf("Couldn't access Backy2 repo. Trying to create it. err=%s", err)
		info, err := schellyhook.ExecShell("backy2 initdb")
		if err != nil {
			logrus.Debugf("Error creating Backy2 repo: %s %s", err, result)
			return err
		} else {
			logrus.Infof("Backy2 repo created successfuly. info=%s", info)
		}
	} else {
		logrus.Infof("Backy2 repo already exists and is accessible")
	}
	return nil
}

//CreateNewBackup creates a new backup
func (sb Backy2Backuper) CreateNewBackup(apiID string, timeout time.Duration, shellContext *schellyhook.ShellContext) error {
	logrus.Infof("CreateNewBackup() apiID=%s timeout=%d s", apiID, timeout.Seconds)

	logrus.Infof("Running Backy2 backup")
	out, err := schellyhook.ExecShellTimeout("backy2 backup "+*sourcePath+" "+*sourcePath, timeout, shellContext)
	if err != nil {
		status := (*shellContext).CmdRef.Status()
		if status.Exit == -1 {
			logrus.Warnf("Backy2 command timeout enforced (%d seconds)", (status.StopTs-status.StartTs)/1000000000)
		}
		logrus.Debugf("Backy2 error. out=%s; err=%s", out, err.Error())
		return err
	} else {
		logrus.Debug("Backy2 backup success")
	}

	rex, _ := regexp.Compile("New version\\: ([\\-a-z0-9]+) \\(Tags")
	id := rex.FindStringSubmatch(out)
	if len(id) == 2 && strings.Contains(out, "Backy complete") {
		backyID := id[1]
		logrus.Infof("Backup success")
		saveDataID(apiID, backyID)
	} else {
		logrus.Errorf("Couldn't find 'Backy complete' or id in command output. out=%s", out)
		return fmt.Errorf("Couldn't find 'Backy complete' or id in command output. out=%s", out)
	}

	logrus.Infof("Backy2 backup finished")
	return nil
}

//GetAllBackups returns all backups from underlaying backuper. optional for Schelly
func (sb Backy2Backuper) GetAllBackups() ([]schellyhook.SchellyResponse, error) {
	logrus.Debugf("GetAllBackups")
	result, err := schellyhook.ExecShell("backy2 -m ls")
	if err != nil {
		return nil, err
	}

	backups := make([]schellyhook.SchellyResponse, 0)
	lines := strings.Split(result, "\n")
	for i, line := range lines {
		cols := strings.Split(line, "|")
		if i == 0 || len(cols) < 7 {
			continue
		}

		dataID := cols[6]
		sizeMB, err := strconv.ParseFloat(cols[5], 64)
		if err != nil {
			return nil, err
		}
		sizeMB = sizeMB / 1000000.0
		status := "running"
		if cols[7] == "1" {
			status = "available"
		}

		sr := schellyhook.SchellyResponse{
			// ID:      getAPIID(dataID),
			DataID:  dataID,
			Status:  status,
			Message: line,
			SizeMB:  sizeMB,
		}
		backups = append(backups, sr)
	}

	return backups, nil
}

//GetBackup get an specific backup along with status
func (sb Backy2Backuper) GetBackup(apiID string) (*schellyhook.SchellyResponse, error) {
	logrus.Debugf("GetBackup apiID=%s", apiID)

	backyID, err0 := getDataID(apiID)
	if err0 != nil {
		logrus.Debugf("BackyID not found for apiId %s. err=%s", apiID, err0)
		return nil, nil
	}

	res, err := findBackup(apiID, backyID)
	if err != nil {
		return nil, nil
	}

	return res, nil
}

//DeleteBackup removes current backup from underlaying backup storage
func (sb Backy2Backuper) DeleteBackup(apiID string) error {
	logrus.Debugf("DeleteBackup apiID=%s", apiID)

	backyID, err0 := getDataID(apiID)
	if err0 != nil {
		logrus.Debugf("BackyID not found for apiId %s. err=%s", apiID, err0)
		return err0
	}

	_, err0 = findBackup(apiID, backyID)
	if err0 != nil {
		logrus.Debugf("Backup apiID %s, backyID %s not found for removal", apiID, backyID)
		return err0
	}

	logrus.Debugf("Backup apiID=%s backyID=%s found. Proceeding to deletion", apiID, backyID)
	result, err := schellyhook.ExecShell("backy2 rm " + backyID)
	if err != nil {
		if strings.Contains(err.Error(), "100") {
			return fmt.Errorf("Cannot delete this backup because it is too young. Configure $PROTECT_YOUNG_BACKUP_DAYS if needed. err=%s", err)
		} else {
			return err
		}
	}
	logrus.Debugf("result: %s", result)

	rex, _ := regexp.Compile("Removed backup version ([\\-a-z0-9]+) with")
	id := rex.FindStringSubmatch(result)
	if len(id) != 2 {
		logrus.Errorf("Couldn't find remove info from Backy2 response. out=%s", result)
		return fmt.Errorf("Couldn't find remove info from Backy2 response. out=%s", result)
	}

	if id[1] != backyID {
		logrus.Errorf("Returned id from delete is different from requested. %s != %s", id[1], backyID)
		return fmt.Errorf("Returned id from delete is different from requested. %s != %s", id[1], backyID)
	}

	logrus.Debugf("Delete apiID %s backyID %s successful", apiID, backyID)
	return nil
}

func findBackup(apiID string, backyID string) (*schellyhook.SchellyResponse, error) {
	result, err := schellyhook.ExecShell("backy2 -m ls")
	if err != nil {
		return nil, err
	}

	rex, _ := regexp.Compile("\\|([0-9]+)\\|" + backyID + "\\|([0|1])")
	id0 := rex.FindStringSubmatch(result)
	if len(id0) != 3 {
		logrus.Debug("Couldn't find backup id in response '%'", id0, result)
		return nil, nil
	}

	logrus.Debugf("Backup %s found", id0)
	status := "running"
	if id0[2] == "1" {
		status = "available"
	}

	size, err1 := strconv.ParseFloat(id0[1], 64)
	if err1 != nil {
		logrus.Warnf("Couldn't get size from Backy2 response. err=%s", err1)
		size = -1
	}

	return &schellyhook.SchellyResponse{
		ID:     apiID,
		DataID: backyID,
		Status: status,
		SizeMB: size / 1000000.0,
	}, nil
}

func getDataID(apiID string) (string, error) {
	fn := "/var/lib/backy2/ids/" + apiID
	if _, err := os.Stat(fn); err == nil {
		logrus.Debugf("Found api id reference for %s", apiID)
		data, err0 := ioutil.ReadFile(fn)
		if err0 != nil {
			return "", err0
		} else {
			backyID := string(data)
			logrus.Debugf("apiID %s <-> backyID %s", apiID, backyID)
			return backyID, nil
		}
	} else {
		return "", fmt.Errorf("backyid for %s not found", apiID)
	}
}

func saveDataID(apiID string, backyID string) error {
	logrus.Debugf("Setting apiID %s <-> backyID %s", apiID, backyID)
	fn := "/var/lib/backy2/ids/" + apiID
	if _, err := os.Stat(fn); err == nil {
		err = os.Remove(fn)
		if err != nil {
			return fmt.Errorf("Couldn't replace existing apiID file. err=%s", err)
		}
	}
	return ioutil.WriteFile(fn, []byte(backyID), 0644)
}

func mkDirs(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.MkdirAll(path, os.ModePerm)
	}
	return nil
}
