package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/techasit5415/microbase/internal/config"
)

const githubRepo = "techasit5415/microbase"

type ghRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func runUpdateCommand(checkOnly bool, targetTag string) error {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", githubRepo)
	if strings.TrimSpace(targetTag) != "" {
		tag := strings.TrimSpace(targetTag)
		if !strings.HasPrefix(tag, "v") {
			tag = "v" + tag
		}
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", githubRepo, tag)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "microbase/"+config.Version)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to query GitHub releases: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub releases returned HTTP %d for %s", resp.StatusCode, apiURL)
	}

	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return fmt.Errorf("decode release metadata: %w", err)
	}

	fmt.Printf("Current version : %s\n", config.Version)
	fmt.Printf("Target release  : %s\n", rel.TagName)

	if checkOnly {
		if rel.TagName == config.Version {
			fmt.Println("You are already on the latest version.")
		} else {
			fmt.Printf("Update available! Run 'microbase update --to %s' to install.\n", rel.TagName)
		}
		return nil
	}

	if rel.TagName == config.Version && targetTag == "" {
		fmt.Println("Already up to date.")
		return nil
	}

	// Find matching asset for current OS/ARCH
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	var assetURL, assetName, checksumURL string
	for _, a := range rel.Assets {
		lower := strings.ToLower(a.Name)
		if strings.Contains(lower, "checksum") {
			checksumURL = a.BrowserDownloadURL
			continue
		}
		if strings.Contains(lower, goos) && strings.Contains(lower, goarch) && !strings.HasSuffix(lower, ".sha256") {
			if goos == "windows" && strings.HasSuffix(lower, ".exe") {
				assetURL = a.BrowserDownloadURL
				assetName = a.Name
			} else if goos != "windows" && !strings.HasSuffix(lower, ".zip") && !strings.HasSuffix(lower, ".tar.gz") {
				assetURL = a.BrowserDownloadURL
				assetName = a.Name
			} else if assetURL == "" {
				assetURL = a.BrowserDownloadURL
				assetName = a.Name
			}
		}
	}

	if assetURL == "" {
		return fmt.Errorf("no prebuilt binary asset found in release %s for %s/%s (build from source with 'go build ./cmd/server')", rel.TagName, goos, goarch)
	}

	fmt.Printf("Downloading %s ...\n", assetName)
	dlResp, err := (&http.Client{Timeout: 5 * time.Minute}).Get(assetURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer dlResp.Body.Close()
	if dlResp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned HTTP %d", dlResp.StatusCode)
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	exePath, _ = filepath.EvalSymlinks(exePath)

	tmpPath := exePath + ".new"
	outFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("create temp binary: %w", err)
	}
	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(outFile, hasher), dlResp.Body); err != nil {
		outFile.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write binary: %w", err)
	}
	outFile.Close()
	actualSHA := hex.EncodeToString(hasher.Sum(nil))

	// Verify checksum if checksums.txt is provided in release
	if checksumURL != "" {
		if csResp, csErr := client.Get(checksumURL); csErr == nil && csResp.StatusCode == http.StatusOK {
			csBytes, _ := io.ReadAll(csResp.Body)
			csResp.Body.Close()
			for _, line := range strings.Split(string(csBytes), "\n") {
				fields := strings.Fields(line)
				if len(fields) >= 2 && strings.TrimPrefix(fields[1], "*") == assetName {
					if !strings.EqualFold(fields[0], actualSHA) {
						_ = os.Remove(tmpPath)
						return fmt.Errorf("checksum mismatch for %s (want %s, got %s)", assetName, fields[0], actualSHA)
					}
					fmt.Printf("Checksum verified (SHA-256: %s...)\n", actualSHA[:16])
					break
				}
			}
		}
	}

	if err := os.Rename(tmpPath, exePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("atomic replace failed: %w", err)
	}
	fmt.Printf("Successfully updated Microbase to %s! Restart the server to apply.\n", rel.TagName)
	return nil
}
