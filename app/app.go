package app

// TODO: build me with backwards compatibility to macOS 11

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
	"strings"
	"syscall"
	"time"

	"github.com/jmorganca/ollama/server"
	"github.com/jmorganca/ollama/version"
)

func Run() {
	// TODO: auto update loop
	// Look for a new update
	// Download it to a temporary location

	// run the ollama server on port 11434
	// TODO: run this based on the user's preferences
	host, port := "127.0.0.1", "11434"
	ln, err := net.Listen("tcp", net.JoinHostPort(host, port))
	if err != nil {
		log.Fatalf("error listening: %v", err)
	}

	var origins []string
	go server.Serve(ln, origins)

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	go func() {
		err := check()
		if err != nil {
			log.Printf("couldnt check for update: %v", err)
		}
		for range ticker.C {
			err = check()
			if err != nil {
				log.Printf("couldnt check for update: %v", err)
			}
		}
	}()

	// Run the native macOS app
	C.run()
}

//export Restart
func Restart() {
	fmt.Println("restart clicked")
}

//export Quit
func Quit() {
	os.Exit(0)
}

func check() error {
	// make get request to ollama.ai/api/update
	// TODO: add missing request headers (signature, user agent, etc.)

	url := fmt.Sprintf("https://ollama.ai/api/update?version=%s", version.Version)

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

	var response struct {
		Url string `json:"url"`
	}

	json.Unmarshal(body, &response)

	// TODO: skip download if the file is already downloaded with the same filesize and checksum

	log.Printf("downloading update... %s", response.Url)

	resp, err = http.Get(response.Url)
	if err != nil {
		return fmt.Errorf("failed to download update: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	// Create a temporary file
	f, err := os.CreateTemp("", "ollama-update-*.zip")
	if err != nil {
		return err
	}
	defer f.Close()

	log.Printf("creating temp file %s...", f.Name())

	_, err = io.Copy(f, resp.Body)
	if err != nil {
		return err
	}

	log.Print("performing update...")

	return update(f.Name())
}

type params struct {
	AppPath string
	ZipPath string
	Pid     int
}

var script = `
PID=%d
APP_DIR="%s"
BACKUP_DIR="%s/OllamaBackup.app"
ZIP_PATH="%s"

on_error() {
    mv "$BACKUP_DIR" "$APP_DIR"
}

trap on_error ERR

kill $PID
mv "$APP_DIR" "$BACKUP_DIR"
unzip "$ZIP_PATH" -d "$(dirname $APP_DIR)"
open "$APP_DIR"
`

func update(zipdir string) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("couldnt determine executable path")
	}

	appdir, ok := strings.CutSuffix(execPath, "/Contents/MacOS/Ollama")
	if !ok {
		return fmt.Errorf("could not find the .app directory in the path of %s", execPath)
	}

	arg := fmt.Sprintf(script, os.Getpid(), appdir, os.TempDir(), zipdir)

	fmt.Println(arg)

	cmd := exec.Command("/bin/bash", "-c", arg)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd.Start()
}
