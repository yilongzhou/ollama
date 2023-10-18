package app

// TODO: build against macOS 11 framework

// #cgo CFLAGS: -x objective-c
// #cgo LDFLAGS: -framework Cocoa
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
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/jmorganca/ollama/server"
	"github.com/jmorganca/ollama/version"
)

func Run() {
	// start the auto update loop
	go func() {
		for {
			err := updater()
			if err != nil {
				log.Printf("couldn't check for update: %v", err)
			}

			time.Sleep(60 * time.Minute)
		}
	}()

	// run the ollama server on port 11434
	// TODO: run this based on the user's preferences
	host, port := "127.0.0.1", "11434"
	ln, err := net.Listen("tcp", net.JoinHostPort(host, port))
	if err != nil {
		log.Fatalf("error listening: %v", err)
	}

	var origins []string
	go server.Serve(ln, origins)

	// Run the native macOS app
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
