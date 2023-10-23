package app

// TODO: build against macOS 11 framework

// #cgo CFLAGS: -x objective-c -Wno-deprecated-declarations
// #cgo LDFLAGS: -framework Cocoa -framework LocalAuthentication
// #include "app.h"
import "C"
import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/jmorganca/ollama/server"
	"github.com/jmorganca/ollama/version"
)

func logfile() (*os.File, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve user home directory: %v", err)
	}

	logdir := filepath.Join(home, ".ollama", "logs")
	logfile := filepath.Join(logdir, "server.log")

	err = os.MkdirAll(logdir, 0755)
	if err != nil {
		return nil, fmt.Errorf("failed to create log directory: %v", err)
	}

	file, err := os.OpenFile(logfile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}

	return file, nil
}

func Run() {
	C.killOtherInstances()

	// start auto updates
	// go func() {
	// 	for {
	// 		updater()
	// 		time.Sleep(time.Hour)
	// 	}
	// }()

	// log to ~/.ollama/logs/server.log
	// TODO (jmorganca): rotate logs if file is too big
	// TODO (jmorganca): use `log` package instead of redirecting
	// TODO (jmorganca): also print to stdout and stderr for easier debugging
	f, err := logfile()
	if err != nil {
		log.Fatalf("failed to configure logging: %v", err)
	}
	os.Stdout = f
	os.Stderr = f
	log.SetOutput(f)
	gin.DefaultWriter = f

	// run the ollama server
	ln, err := net.Listen("tcp", "127.0.0.1:11434")
	if err != nil {
		log.Fatalf("error listening: %v", err)
	}

	var origins []string
	go server.Serve(ln, origins)

	// Finally run the main loop of the macOS app
	C.run()
}

//export Quit
func Quit() {
	syscall.Kill(os.Getpid(), syscall.SIGTERM)
}

var script = `
PID=%d
APP_PATH="%s"
TMP_DIR="%s"
BACKUP_DIR="$TMP_DIR/OllamaBackup.app"
ZIP_FILE="%s"

rm -rf "$TMP_DIR/Ollama.app" "$BACKUP_DIR"
unzip "$ZIP_FILE" -d "$TMP_DIR"
kill $PID
while kill -0 $PID; do
    sleep 0.05
done

mv "$APP_PATH" "$BACKUP_DIR"
mv "$TMP_DIR/Ollama.app" "$APP_PATH"
open "$APP_PATH"
`

func updater() error {
	// make get request to ollama.ai/api/update
	// TODO: add missing request headers (signature, user agent, etc.)

	url := fmt.Sprintf("https://ollama.ai/api/update?version=%s&arch=%s&os=%s", version.Version, runtime.GOARCH, runtime.GOOS)
	fmt.Println(url)

	// Make the GET request
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("failed to check for updates status: %d", resp.StatusCode)
	}

	if resp.StatusCode == 204 {
		log.Printf("auto updater: no updates available")
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %v", err)
	}

	var update struct {
		Url  string `json:"url"`
		Size int    `json:"size"`
	}

	json.Unmarshal(body, &update)

	// TODO: skip download if the file is already downloaded with the same name, filesize and checksum

	log.Printf("downloading update... %s", update.Url)

	resp, err = http.Get(update.Url)
	if err != nil {
		return fmt.Errorf("failed to download update: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	f, err := os.CreateTemp("", "ollama-*.zip")
	if err != nil {
		return err
	}
	defer f.Close()

	zipfile := f.Name()

	log.Printf("creating temp file %s...", zipfile)

	_, err = io.Copy(f, resp.Body)
	if err != nil {
		return err
	}

	// TODO: validate sha256 as well
	log.Printf("validating update filesize...")
	// fi, err := os.Stat(zipfile)
	// if err != nil {
	// 	return fmt.Errorf("failed to stat downloaded file: %v", err)
	// }

	// if fi.Size() != int64(update.Size) {
	// 	return fmt.Errorf("downloaded file size %d does not match expected size %d", fi.Size(), update.Size)
	// }

	log.Printf("performing update with %s...", zipfile)

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("couldnt determine executable path")
	}

	appPath, ok := strings.CutSuffix(execPath, "/Contents/MacOS/Ollama")
	if !ok {
		return fmt.Errorf("could not find the .app directory in the path of %s", execPath)
	}

	arg := fmt.Sprintf(script, os.Getpid(), appPath, os.TempDir(), zipfile)

	cmd := exec.Command("/bin/bash", "-c", arg)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd.Start()
}
