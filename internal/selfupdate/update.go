package selfupdate

import (
	"archive/tar"
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
	"strings"
)

const defaultBaseURL = "https://api.github.com"
const repoOwner = "nickw409"
const repoName = "arc"

type release struct {
	TagName string  `json:"tag_name"`
	Assets  []asset `json:"assets"`
}

type asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	APIURL             string `json:"url"`
}

// Update checks for the latest release and updates the binary if a newer
// version is available. baseURL can be overridden for testing; pass "" to
// use the default GitHub API.
func Update(currentVersion, baseURL string) error {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	binData, newVersion, err := fetchUpdate(currentVersion, baseURL, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	if binData == nil {
		fmt.Println("Already up to date.")
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding executable: %w", err)
	}
	exePath, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("resolving executable path: %w", err)
	}

	if err := atomicReplace(exePath, binData); err != nil {
		return err
	}

	fmt.Printf("Updated arc: %s → %s\n", currentVersion, newVersion)
	return nil
}

// fetchUpdate downloads and verifies a new release. Returns nil binData if
// already up to date. Separated from Update to enable end-to-end testing
// without replacing the running binary.
func fetchUpdate(currentVersion, baseURL, goos, goarch string) (binData []byte, newVersion string, err error) {
	rel, err := latestRelease(baseURL)
	if err != nil {
		return nil, "", err
	}

	latest := strings.TrimPrefix(rel.TagName, "v")
	if latest == currentVersion {
		return nil, latest, nil
	}

	assetName, err := findAsset(latest, goos, goarch)
	if err != nil {
		return nil, "", err
	}

	var tarAsset, checksumAsset *asset
	for i := range rel.Assets {
		switch rel.Assets[i].Name {
		case assetName:
			tarAsset = &rel.Assets[i]
		case "checksums.txt":
			checksumAsset = &rel.Assets[i]
		}
	}
	if tarAsset == nil {
		return nil, "", fmt.Errorf("asset %s not found in release %s", assetName, rel.TagName)
	}
	if checksumAsset == nil {
		return nil, "", fmt.Errorf("checksums.txt not found in release %s", rel.TagName)
	}

	checksumData, err := downloadAsset(checksumAsset)
	if err != nil {
		return nil, "", fmt.Errorf("downloading checksums: %w", err)
	}

	assetData, err := downloadAsset(tarAsset)
	if err != nil {
		return nil, "", fmt.Errorf("downloading asset: %w", err)
	}

	if err := verifyChecksum(assetName, assetData, checksumData); err != nil {
		return nil, "", err
	}

	binData, err = extractBinary(assetData)
	if err != nil {
		return nil, "", err
	}

	return binData, latest, nil
}

// atomicReplace writes data to a temp file in the same directory as target,
// then renames it over target. Same-filesystem rename is atomic on POSIX.
func atomicReplace(target string, data []byte) error {
	dir := filepath.Dir(target)
	tmp, err := os.CreateTemp(dir, "arc-update-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("setting permissions: %w", err)
	}
	if err := os.Rename(tmpPath, target); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("replacing binary: %w", err)
	}
	return nil
}

func latestRelease(baseURL string) (*release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", baseURL, repoOwner, repoName)
	resp, err := authedGet(url)
	if err != nil {
		return nil, fmt.Errorf("fetching latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("parsing release JSON: %w", err)
	}
	return &rel, nil
}

func findAsset(version, goos, goarch string) (string, error) {
	switch goos {
	case "linux", "darwin":
	default:
		return "", fmt.Errorf("unsupported OS: %s", goos)
	}
	switch goarch {
	case "amd64", "arm64":
	default:
		return "", fmt.Errorf("unsupported architecture: %s", goarch)
	}
	return fmt.Sprintf("arc_%s_%s_%s.tar.gz", version, goos, goarch), nil
}

func verifyChecksum(assetName string, assetData, checksumData []byte) error {
	h := sha256.Sum256(assetData)
	actual := hex.EncodeToString(h[:])

	for _, line := range strings.Split(string(checksumData), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == assetName {
			if parts[0] != actual {
				return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", assetName, parts[0], actual)
			}
			return nil
		}
	}
	return fmt.Errorf("no checksum found for %s", assetName)
}

func extractBinary(tarGzData []byte) ([]byte, error) {
	gr, err := gzip.NewReader(bytes.NewReader(tarGzData))
	if err != nil {
		return nil, fmt.Errorf("opening gzip: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading tar: %w", err)
		}
		if filepath.Base(hdr.Name) == "arc" && hdr.Typeflag == tar.TypeReg {
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, fmt.Errorf("reading binary from tar: %w", err)
			}
			return data, nil
		}
	}
	return nil, fmt.Errorf("arc binary not found in archive")
}

// downloadAsset downloads a release asset. For private repos (when a token is
// available), it uses the GitHub API URL with Accept: application/octet-stream.
// For public repos it uses the browser download URL directly.
func downloadAsset(a *asset) ([]byte, error) {
	token := githubToken()
	if token != "" && a.APIURL != "" {
		req, err := http.NewRequest("GET", a.APIURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "token "+token)
		req.Header.Set("Accept", "application/octet-stream")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("download %s returned %d", a.APIURL, resp.StatusCode)
		}
		return io.ReadAll(resp.Body)
	}
	return downloadBytes(a.BrowserDownloadURL)
}

func downloadBytes(url string) ([]byte, error) {
	resp, err := authedGet(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s returned %d", url, resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// githubToken returns a token from GITHUB_TOKEN or GH_TOKEN if set.
func githubToken() string {
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t
	}
	return os.Getenv("GH_TOKEN")
}

// authedGet performs an HTTP GET, adding an Authorization header if a
// GitHub token is available. This allows updates from private repos.
func authedGet(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if token := githubToken(); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
	return http.DefaultClient.Do(req)
}
