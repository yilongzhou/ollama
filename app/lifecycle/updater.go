package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/jmorganca/ollama/auth"
	"github.com/jmorganca/ollama/version"
)

var (
	UpdateCheckURLBase = "https://ollama.ai/api/update"
	UpdateDownloaded   = false
)

// TODO - maybe move up to the API package?
type UpdateResponse struct {
	UpdateURL     string `json:"url"`
	UpdateVersion string `json:"version"`
}

func getClient(req *http.Request) http.Client {
	proxyURL, err := http.ProxyFromEnvironment(req)
	if err != nil {
		slog.Warn(fmt.Sprintf("failed to handle proxy: %s", err))
		return http.Client{}
	}

	return http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
	}
}

func IsNewReleaseAvailable(ctx context.Context) (bool, UpdateResponse) {
	var updateResp UpdateResponse
	updateCheckURL := UpdateCheckURLBase + "?os=" + runtime.GOOS + "&arch=" + runtime.GOARCH + "&version=" + version.Version
	headers := make(http.Header)
	err := auth.SignRequest(http.MethodGet, updateCheckURL, nil, headers)
	if err != nil {
		slog.Info(fmt.Sprintf("failed to sign update request %s", err))
	}
	slog.Debug(fmt.Sprintf("XXX checking for update via %s - %v", updateCheckURL, headers))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, updateCheckURL, nil)
	if err != nil {
		slog.Warn(fmt.Sprintf("failed to check for update: %s", err))
		return false, updateResp
	}
	req.Header = headers
	req.Header.Set("User-Agent", fmt.Sprintf("ollama/%s (%s %s) Go/%s", version.Version, runtime.GOARCH, runtime.GOOS, runtime.Version()))
	client := getClient(req)

	resp, err := client.Do(req)
	if err != nil {
		slog.Warn(fmt.Sprintf("failed to check for update: %s", err))
		return false, updateResp
	}
	defer resp.Body.Close()

	if resp.StatusCode == 204 {
		slog.Debug("XXX got 204 when checking for update")
		return false, updateResp
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Debug(fmt.Sprintf("XXX failed to read body response: %s", err))
	}
	err = json.Unmarshal(body, &updateResp)
	if err != nil {
		slog.Warn(fmt.Sprintf("malformed response checking for update: %s", err))
		return false, updateResp
	}
	// Extract the version string from the URL
	updateResp.UpdateVersion = path.Base(path.Dir(updateResp.UpdateURL))

	slog.Info("New update available at " + updateResp.UpdateURL)
	return true, updateResp
}

func DownloadNewRelease(ctx context.Context, updateResp UpdateResponse) error {
	// Do a head first to check etag info
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, updateResp.UpdateURL, nil)
	if err != nil {
		return err
	}
	// Rate limiting and private repo support
	token := os.Getenv("GITHUB_TOKEN")
	if token != "" {
		req.Header.Add("Authorization", "Bearer "+token)
	}
	client := getClient(req)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error checking update: %w", err)
	}
	resp.Body.Close()
	etag := strings.Trim(resp.Header.Get("etag"), "\"")
	if etag == "" {
		slog.Debug("no etag detected, falling back to filename based dedup")
		etag = "_"
	}
	filename := Installer
	_, params, err := mime.ParseMediaType(resp.Header.Get("content-disposition"))
	if err == nil {
		filename = params["filename"]
	}

	stageFilename := filepath.Join(UpdateStageDir, etag, filename)
	slog.Debug("XXX update will be staged as " + stageFilename)

	// Check to see if we already have it downloaded
	_, err = os.Stat(stageFilename)
	if err == nil {
		slog.Debug("update already downloaded")
		return nil
	}

	cleanupOldDownloads()

	req.Method = http.MethodGet
	resp, err = client.Do(req)
	if err != nil {
		return fmt.Errorf("error checking update: %w", err)
	}
	defer resp.Body.Close()
	etag = strings.Trim(resp.Header.Get("etag"), "\"")
	if etag == "" {
		slog.Debug("no etag detected, falling back to filename based dedup") // TODO probably can get rid of this redundant log
		etag = "_"
	}

	stageFilename = filepath.Join(UpdateStageDir, etag, filename)

	_, err = os.Stat(filepath.Dir(stageFilename))
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(stageFilename), 0o755); err != nil {
			return fmt.Errorf("create ollama dir %s: %v", filepath.Dir(stageFilename), err)
		}
	}

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read body response: %w", err)
	}
	fp, err := os.OpenFile(stageFilename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("write payload %s: %w", stageFilename, err)
	}
	defer fp.Close()
	if n, err := fp.Write(payload); err != nil || n != len(payload) {
		return fmt.Errorf("write payload %s: %d vs %d -- %w", stageFilename, n, len(payload), err)
	}
	slog.Debug("new update downloaded " + stageFilename)

	UpdateDownloaded = true
	return nil
}

func cleanupOldDownloads() {
	files, err := os.ReadDir(UpdateStageDir)
	if err != nil {
		slog.Debug(fmt.Sprintf("failed to list stage dir: %s", err))
		return
	}
	for _, file := range files {
		fullname := filepath.Join(UpdateStageDir, file.Name())
		slog.Debug("cleaning up old download: " + fullname)
		err = os.RemoveAll(fullname)
		if err != nil {
			slog.Warn(fmt.Sprintf("failed to cleanup stale update download %s", err))
		}
	}
}

func StartBackgroundUpdaterChecker(ctx context.Context, cb func(string) error) {
	go func() {
		// Don't blast an update message immediately after startup
		// time.Sleep(30 * time.Second)
		time.Sleep(3 * time.Second)

		for {
			available, resp := IsNewReleaseAvailable(ctx)
			if available {
				err := DownloadNewRelease(ctx, resp)
				if err != nil {
					slog.Error(fmt.Sprintf("failed to download new release: %s", err))
				}
				err = cb(resp.UpdateVersion)
				if err != nil {
					slog.Debug("XXX failed to register update available with tray")
				}
			}
			select {
			case <-ctx.Done():
				slog.Debug("XXX stopping background update checker")
				return
			default:
				time.Sleep(60 * 60 * time.Second)
			}
		}
	}()
}

func DoUpgrade() error {
	files, err := filepath.Glob(filepath.Join(UpdateStageDir, "*", "*.exe")) // TODO generalize for multiplatform
	if err != nil {
		return fmt.Errorf("failed to lookup downloads: %s", err)
	}
	if len(files) == 0 {
		return fmt.Errorf("no update downloads found")
	} else if len(files) > 1 {
		// Shouldn't happen
		slog.Warn(fmt.Sprintf("multiple downloads found %v", files))
	}
	installerExe := files[0]

	slog.Info("starting upgrade with " + installerExe)
	slog.Info("upgrade log file " + UpgradeLogFile)

	installArgs := []string{
		"/CLOSEAPPLICATIONS",                    // Quit the tray app if it's still running
		"/LOG=" + filepath.Base(UpgradeLogFile), // Only relative seems reliable, so set pwd
		// "/FORCECLOSEAPPLICATIONS", // Force close the tray app - might be needed
	}
	// In debug mode, let the installer show to aid in troubleshooting if something goes wrong
	if debug := os.Getenv("OLLAMA_DEBUG"); debug == "" {
		installArgs = append(installArgs,
			"/SP", // Skip the "This will install... Do you wish to continue" prompt
			"/SUPPRESSMSGBOXES",
			"/SILENT",
			"/VERYSILENT",
		)
	}
	slog.Debug(fmt.Sprintf("Upgrade: %s %v", installerExe, installArgs))
	os.Chdir(filepath.Dir(UpgradeLogFile)) //nolint:errcheck
	cmd := exec.Command(installerExe, installArgs...)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("unable to start ollama app %w", err)
	}

	if cmd.Process != nil {
		err = cmd.Process.Release()
		if err != nil {
			slog.Error(fmt.Sprintf("failed to release server process: %s", err))
		}
	} else {
		// TODO - some details about why it didn't start, or is this a pedantic error case?
		return fmt.Errorf("installer process did not start")
	}
	slog.Info("Installer started in background, exiting")

	os.Exit(0)
	// Not reached
	return nil
}
