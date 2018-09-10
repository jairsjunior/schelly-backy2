package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/satori/go.uuid"
)

//Options command line options
type Options struct {
	listenPort        int
	listenIP          string
	sourcePath        string
	repoDir           string
	maxTimeRunning    int
	preBackupCommand  string
	postBackupCommand string
}

//Response schelly webhook response
type Response struct {
	ID      string  `json:"id",omitempty`
	Status  string  `json:"status",omitempty`
	Message string  `json:"message",omitempty`
	SizeMB  float64 `json:"size_mb",omitempty`
}

var options = new(Options)
var runningBackupAPIID = ""
var currentBackupContext = ShellContext{}
var createBackupChan = make(chan string)

func main() {
	listenPort := flag.Int("listen-port", 7070, "REST API server listen port")
	listenIP := flag.String("listen-ip", "0.0.0.0", "REST API server listen ip address")
	logLevel := flag.String("log-level", "info", "debug, info, warning or error")
	sourcePath := flag.String("source-path", "file:///backup-source/backup-this", "Backup source path. rbd://<pool>/<imagename>[@<snapshotname>] OR file:///[dir]/[file]")
	maxTimeRunning := flag.Int("max-backup-time-running", 7200, "Max time for a single backup to keep running in seconds. After that time the process will be killed")
	preBackupCommand := flag.String("pre-backup-command", "", "Command to be executed before running the backup")
	postBackupCommand := flag.String("post-backup-command", "", "Command to be executed after running the backup")
	flag.Parse()

	switch *logLevel {
	case "debug":
		logrus.SetLevel(logrus.DebugLevel)
		break
	case "warning":
		logrus.SetLevel(logrus.WarnLevel)
		break
	case "error":
		logrus.SetLevel(logrus.ErrorLevel)
		break
	default:
		logrus.SetLevel(logrus.InfoLevel)
	}

	options.listenPort = *listenPort
	options.listenIP = *listenIP
	options.sourcePath = *sourcePath
	options.maxTimeRunning = *maxTimeRunning
	options.preBackupCommand = *preBackupCommand
	options.postBackupCommand = *postBackupCommand

	err := mkDirs("/var/lib/backy2/ids")
	if err != nil {
		logrus.Errorf("Couldn't create id references dir at /var/lib/backy2/ids. err=%s", err)
		os.Exit(1)
	}

	logrus.Info("====Starting Backy2 REST server====")

	logrus.Debugf("Checking if Restic repo was already initialized")
	result, err := execShell("backy2 ls")
	if err != nil {
		logrus.Debugf("Couldn't access Backy2 repo. Trying to create it. err=%s", err)
		info, err := execShell("backy2 initdb")
		if err != nil {
			logrus.Debugf("Error creating Backy2 repo: %s %s", err, result)
			os.Exit(1)
		} else {
			logrus.Infof("Backy2 repo created successfuly. info=%s", info)
		}
	} else {
		logrus.Infof("Backy2 repo already exists and is accessible")
	}

	//process background backup tasks
	go func() {
		handleBackupExecution()
	}()

	router := mux.NewRouter()
	router.HandleFunc("/backups", GetBackups).Methods("GET")
	router.HandleFunc("/backups", CreateBackup).Methods("POST")
	router.HandleFunc("/backups/{id}", GetBackup).Methods("GET")
	router.HandleFunc("/backups/{id}", DeleteBackup).Methods("DELETE")
	listen := fmt.Sprintf("%s:%d", options.listenIP, options.listenPort)
	logrus.Infof("Listening at %s", listen)
	err = http.ListenAndServe(listen, router)
	if err != nil {
		logrus.Errorf("Error while listening requests: %s", err)
		os.Exit(1)
	}
}

//GetBackups - get backups from Backy
func GetBackups(w http.ResponseWriter, r *http.Request) {
	logrus.Debugf("GetBackups r=%s", r)
	result, err := execShell("backy2 -m ls")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(result))
	logrus.Debugf("result: %s", result)
}

//GetBackup - get specific backup from Backy
func GetBackup(w http.ResponseWriter, r *http.Request) {
	logrus.Debugf("GetBackup r=%s", r)
	params := mux.Vars(r)

	apiID := params["id"]

	if runningBackupAPIID == apiID {
		sendResponse(apiID, "running", "backup is still running", -1, http.StatusOK, w)
		return
	}

	backyID, err0 := getBackyID(apiID)
	if err0 != nil {
		logrus.Debugf("BackyID not found for apiId %s. err=%s", apiID, err0)
		http.Error(w, fmt.Sprintf("Backup %s not found", apiID), http.StatusNotFound)
		return
	}

	res, err := findBackup(backyID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if res.ID == "" {
		http.Error(w, fmt.Sprintf("Backup %s not found", apiID), http.StatusNotFound)
		return
	}

	sendResponse(apiID, res.Status, res.Message, res.SizeMB, http.StatusOK, w)
}

//CreateBackup - trigger new backup on Backy2
func CreateBackup(w http.ResponseWriter, r *http.Request) {
	logrus.Infof(">>>>CreateBackup r=%s", r)

	if runningBackupAPIID != "" {
		logrus.Infof("Another backup id %s is already running. Aborting.", runningBackupAPIID)
		http.Error(w, fmt.Sprintf("Another backup id %s is already running. Aborting.", runningBackupAPIID), http.StatusConflict)
		return
	}

	runningBackupAPIID = createAPIID()
	logrus.Debugf("Created apiID %s", runningBackupAPIID)

	createBackupChan <- runningBackupAPIID
	sendResponse(runningBackupAPIID, "running", "backup start running", -1, http.StatusAccepted, w)
}

//DeleteBackup - delete backup from Backy2
func DeleteBackup(w http.ResponseWriter, r *http.Request) {
	logrus.Debugf("DeleteBackup r=%s", r)
	params := mux.Vars(r)

	apiID := params["id"]

	if runningBackupAPIID == apiID {
		err := (*currentBackupContext.cmdRef).Stop()
		if err != nil {
			sendResponse(apiID, "running", "Couldn't cancel current running backup task. err="+err.Error(), -1, http.StatusInternalServerError, w)
		} else {
			sendResponse(apiID, "deleted", "Running backup task was cancelled", -1, http.StatusOK, w)
		}
		return
	}

	backyID, err0 := getBackyID(apiID)
	if err0 != nil {
		logrus.Debugf("BackyID not found for apiId %s. err=%s", apiID, err0)
		http.Error(w, fmt.Sprintf("Backup %s not found", apiID), http.StatusNotFound)
		return
	}

	res, err0 := findBackup(backyID)
	if err0 != nil {
		logrus.Debugf("Backup %s not found for removal", params["id"])
		http.Error(w, err0.Error(), http.StatusInternalServerError)
		return
	}
	if res.ID == "" {
		http.Error(w, fmt.Sprintf("Backup %s not found", params["id"]), http.StatusNotFound)
		return
	}

	logrus.Debugf("Backup api=%s backy=%s found. Proceeding to deletion", apiID, backyID)
	result, err := execShell("backy2 rm " + backyID)
	if err != nil {
		if strings.Contains(err.Error(), "100") {
			http.Error(w, "Cannot delete this backup because it is too young. Configure $PROTECT_YOUNG_BACKUP_DAYS if needed", http.StatusInternalServerError)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	logrus.Debugf("result: %s", result)

	rex, _ := regexp.Compile("Removed backup version ([\\-a-z0-9]+) with")
	id := rex.FindStringSubmatch(result)
	if len(id) != 2 {
		http.Error(w, "Couldn't find remove info from response", http.StatusInternalServerError)
		return
	}

	if id[1] != backyID {
		logrus.Errorf("Returned id from delete is different from requested. %s != %s", id[1], backyID)
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
		return
	}

	sendResponse(apiID, "deleted", result, -1, http.StatusOK, w)
}

func findBackup(id string) (Response, error) {
	result, err := execShell("backy2 -m ls")
	if err != nil {
		return Response{}, err
	}
	logrus.Debugf("Query snapshots result: %s", result)

	rex, _ := regexp.Compile("\\|([0-9]+)\\|" + id + "\\|([0|1])")
	id0 := rex.FindStringSubmatch(result)
	if len(id0) != 3 {
		logrus.Debug("Couldn't find backup id in response '%'", id0, result)
		return Response{}, nil
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

	return Response{
		ID:     id,
		Status: status,
		SizeMB: size / 1000000.0,
	}, nil
}

func sendResponse(id string, status string, message string, size float64, httpStatus int, w http.ResponseWriter) {
	resp := Response{
		ID:      id,
		Status:  status,
		Message: message,
		SizeMB:  size,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	err := json.NewEncoder(w).Encode(resp)
	if err != nil {
		logrus.Errorf("Error encoding response. err=%s", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	} else {
		logrus.Debugf("Response sent %s", resp)
	}
}

func getBackyID(apiID string) (string, error) {
	fn := "/var/lib/backy2/ids/" + apiID
	if _, err := os.Stat(fn); err == nil {
		logrus.Debugf("Found api id reference for %s", apiID)
		data, err0 := ioutil.ReadFile(fn)
		if err0 != nil {
			return "", err0
		} else {
			backyID := string(data)
			logrus.Debugf("apiID %s -> backyID %s", apiID, backyID)
			return backyID, nil
		}
	} else {
		return "", fmt.Errorf("backyid for %s not found", apiID)
	}
}

func createAPIID() string {
	uuid, _ := uuid.NewV4()
	return uuid.String()
}

func saveBackyID(apiID string, backyID string) error {
	logrus.Debugf("Creating new ApiID for backyID %s", backyID)
	fn := "/var/lib/backy2/ids/" + apiID
	if _, err := os.Stat(fn); err == nil {
		err = os.Remove(fn)
		if err != nil {
			return fmt.Errorf("Couldn't replace existing apiID file. err=%s", err)
		}
	}
	return ioutil.WriteFile(fn, []byte(backyID), 0644)
}

func handleBackupExecution() {
	for true {
		logrus.Debugf("Waiting next request to come...")
		<-createBackupChan
		logrus.Debugf("Backup request arrived")

		if options.preBackupCommand != "" {
			logrus.Infof("Running pre-backup command '%s'", options.preBackupCommand)
			out, err := execShellTimeout(options.preBackupCommand, time.Duration(options.maxTimeRunning)*time.Second, &currentBackupContext)
			if err != nil {
				logrus.Debugf("Pre-backup command error. out=%s; err=%s", out, err.Error())
				runningBackupAPIID = ""
				continue
			} else {
				logrus.Debug("Pre-backup command success")
			}
		}

		logrus.Infof("Running Backy2 backup")
		out, err := execShellTimeout("backy2 backup "+options.sourcePath+" "+options.sourcePath, time.Duration(options.maxTimeRunning)*time.Second, &currentBackupContext)
		if err != nil {
			logrus.Debugf("Backy2 error. out=%s; err=%s", out, err.Error())
			runningBackupAPIID = ""
			continue
		} else {
			logrus.Debug("Backy2 backup success")
		}

		rex, _ := regexp.Compile("New version\\: ([\\-a-z0-9]+) \\(Tags")
		id := rex.FindStringSubmatch(out)
		if len(id) == 2 && strings.Contains(out, "Backy complete") {
			backyID := id[1]
			logrus.Infof("Backup success detected")
			saveBackyID(runningBackupAPIID, backyID)
		} else {
			logrus.Errorf("Couldn't find 'Backy complete' or id in command output. out=%s", out)
			runningBackupAPIID = ""
			continue
		}

		//process post backup command after finished
		if options.postBackupCommand != "" {
			logrus.Infof("Running post-backup command '%s'", options.postBackupCommand)
			out, err := execShellTimeout(options.postBackupCommand, time.Duration(options.maxTimeRunning)*time.Second, &currentBackupContext)
			if err != nil {
				logrus.Debugf("Post-backup command error. out=%s; err=%s", out, err.Error())
				runningBackupAPIID = ""
				continue
			} else {
				logrus.Debug("Post-backup command success")
			}
		}
		logrus.Infof("Backup finished")

		//now we can accept another POST /backups call...
		runningBackupAPIID = ""
	}
}
