package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
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

	cmp := compareSemver(rel.TagName, config.Version)
	if checkOnly {
		if cmp <= 0 {
			fmt.Printf("You are already up to date (current %s >= latest %s).\n", config.Version, rel.TagName)
		} else {
			fmt.Printf("Update available! Run 'microbase update --to %s' to install.\n", rel.TagName)
		}
		return nil
	}

	if targetTag == "" && cmp <= 0 {
		fmt.Printf("Already up to date (current %s >= latest %s). Use '--to %s' to downgrade.\n", config.Version, rel.TagName, rel.TagName)
		return nil
	}

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
			assetURL = a.BrowserDownloadURL
			assetName = a.Name
			break
		}
	}

	if assetURL == "" {
		return fmt.Errorf("no prebuilt asset found in release %s for %s/%s", rel.TagName, goos, goarch)
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

	archiveBytes, err := io.ReadAll(dlResp.Body)
	if err != nil {
		return fmt.Errorf("read download: %w", err)
	}
	sum := sha256.Sum256(archiveBytes)
	actualSHA := hex.EncodeToString(sum[:])

	// Verify checksum against checksums.txt if published
	if checksumURL != "" {
		if csResp, csErr := client.Get(checksumURL); csErr == nil && csResp.StatusCode == http.StatusOK {
			csBytes, _ := io.ReadAll(csResp.Body)
			csResp.Body.Close()
			for _, line := range strings.Split(string(csBytes), "\n") {
				fields := strings.Fields(line)
				if len(fields) >= 2 && strings.TrimPrefix(fields[1], "*") == assetName {
					if !strings.EqualFold(fields[0], actualSHA) {
						return fmt.Errorf("checksum mismatch for %s (want %s, got %s)", assetName, fields[0], actualSHA)
					}
					fmt.Printf("Checksum verified (SHA-256: %s...)\n", actualSHA[:16])
					break
				}
			}
		}
	}

	// Extract binary from .zip or .tar.gz, or use raw bytes directly
	binBytes, err := extractBinary(assetName, archiveBytes)
	if err != nil {
		return err
	}
	if !looksLikeExecutable(binBytes) {
		return fmt.Errorf("downloaded payload from %s is not a valid executable binary", assetName)
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	exePath, _ = filepath.EvalSymlinks(exePath)

	tmpPath := exePath + ".new"
	if err := os.WriteFile(tmpPath, binBytes, 0o755); err != nil {
		return fmt.Errorf("write temp binary: %w", err)
	}
	if err := os.Rename(tmpPath, exePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("atomic replace failed: %w", err)
	}
	fmt.Printf("Successfully updated Microbase to %s! Restart the server to apply.\n", rel.TagName)
	return nil
}

func extractBinary(assetName string, data []byte) ([]byte, error) {
	lower := strings.ToLower(assetName)
	wantBase := "microbase"
	if runtime.GOOS == "windows" {
		wantBase = "microbase.exe"
	}

	if strings.HasSuffix(lower, ".zip") {
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, fmt.Errorf("open zip %s: %w", assetName, err)
		}
		for _, f := range zr.File {
			if f.FileInfo().IsDir() {
				continue
			}
			base := strings.ToLower(filepath.Base(f.Name))
			if base == wantBase || (runtime.GOOS != "windows" && (base == "microbase" || base == "server")) {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return io.ReadAll(rc)
			}
		}
		return nil, fmt.Errorf("binary %q not found inside %s", wantBase, assetName)
	}

	if strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz") {
		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("open gzip %s: %w", assetName, err)
		}
		defer gz.Close()
		tr := tar.NewReader(gz)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			base := strings.ToLower(filepath.Base(hdr.Name))
			if base == wantBase || (runtime.GOOS != "windows" && (base == "microbase" || base == "server")) {
				return io.ReadAll(tr)
			}
		}
		return nil, fmt.Errorf("binary %q not found inside %s", wantBase, assetName)
	}

	return data, nil
}

func looksLikeExecutable(b []byte) bool {
	if len(b) < 4 {
		return false
	}
	// ELF (Linux), PE/MZ (Windows), Mach-O (macOS)
	if bytes.Equal(b[:4], []byte{0x7f, 'E', 'L', 'F'}) {
		return true
	}
	if b[0] == 'M' && b[1] == 'Z' {
		return true
	}
	if (b[0] == 0xcf && b[1] == 0xfa && b[2] == 0xed && b[3] == 0xfe) ||
		(b[0] == 0xfe && b[1] == 0xed && b[2] == 0xfa && b[3] == 0xcf) ||
		(b[0] == 0xca && b[1] == 0xfe && b[2] == 0xba && b[3] == 0xbe) {
		return true
	}
	return false
}

// compareSemver compares two vX.Y.Z tags: returns 1 if a > b, -1 if a < b, 0 if equal.
func compareSemver(a, b string) int {
	parse := func(s string) [3]int {
		s = strings.TrimPrefix(strings.TrimSpace(s), "v")
		if idx := strings.IndexAny(s, "-+"); idx >= 0 {
			s = s[:idx]
		}
		parts := strings.Split(s, ".")
		var out [3]int
		for i := 0; i < 3 && i < len(parts); i++ {
			out[i], _ = strconv.Atoi(parts[i])
		}
		return out
	}
	va, vb := parse(a), parse(b)
	for i := 0; i < 3; i++ {
		if va[i] > vb[i] {
			return 1
		}
		if va[i] < vb[i] {
			return -1
		}
	}
	return 0
}
