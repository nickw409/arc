package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindAsset(t *testing.T) {
	tests := []struct {
		version, goos, goarch string
		want                  string
		wantErr               bool
	}{
		{"1.0.0", "linux", "amd64", "arc_1.0.0_linux_amd64.tar.gz", false},
		{"1.0.0", "darwin", "arm64", "arc_1.0.0_darwin_arm64.tar.gz", false},
		{"2.1.0", "darwin", "amd64", "arc_2.1.0_darwin_amd64.tar.gz", false},
		{"1.0.0", "windows", "amd64", "", true},
		{"1.0.0", "linux", "386", "", true},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s/%s/%s", tt.goos, tt.goarch, tt.version), func(t *testing.T) {
			got, err := findAsset(tt.version, tt.goos, tt.goarch)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVerifyChecksum(t *testing.T) {
	data := []byte("hello world")
	h := sha256.Sum256(data)
	goodChecksum := fmt.Sprintf("%x  arc_1.0.0_linux_amd64.tar.gz\n", h)
	badChecksum := "0000000000000000000000000000000000000000000000000000000000000000  arc_1.0.0_linux_amd64.tar.gz\n"
	missingChecksum := fmt.Sprintf("%x  some_other_file.tar.gz\n", h)

	if err := verifyChecksum("arc_1.0.0_linux_amd64.tar.gz", data, []byte(goodChecksum)); err != nil {
		t.Fatalf("valid checksum rejected: %v", err)
	}

	if err := verifyChecksum("arc_1.0.0_linux_amd64.tar.gz", data, []byte(badChecksum)); err == nil {
		t.Fatal("bad checksum not detected")
	}

	if err := verifyChecksum("arc_1.0.0_linux_amd64.tar.gz", data, []byte(missingChecksum)); err == nil {
		t.Fatal("missing checksum entry not detected")
	}
}

func TestExtractBinary(t *testing.T) {
	content := []byte("fake-arc-binary")
	tarGz := makeTarGz(t, "arc", content)

	got, err := extractBinary(tarGz)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("got %q, want %q", got, content)
	}
}

func TestExtractBinaryNotFound(t *testing.T) {
	tarGz := makeTarGz(t, "not-arc", []byte("data"))
	_, err := extractBinary(tarGz)
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
}

func TestLatestRelease(t *testing.T) {
	rel := release{
		TagName: "v1.2.3",
		Assets: []asset{
			{Name: "arc_1.2.3_linux_amd64.tar.gz", BrowserDownloadURL: "https://example.com/linux"},
			{Name: "checksums.txt", BrowserDownloadURL: "https://example.com/checksums"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != fmt.Sprintf("/repos/%s/%s/releases/latest", repoOwner, repoName) {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(rel)
	}))
	defer srv.Close()

	got, err := latestRelease(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got.TagName != "v1.2.3" {
		t.Fatalf("got tag %q, want v1.2.3", got.TagName)
	}
	if len(got.Assets) != 2 {
		t.Fatalf("got %d assets, want 2", len(got.Assets))
	}
}

func TestUpdateAlreadyUpToDate(t *testing.T) {
	rel := release{
		TagName: "v1.0.0",
		Assets:  []asset{},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(rel)
	}))
	defer srv.Close()

	err := Update("1.0.0", srv.URL)
	if err != nil {
		t.Fatalf("expected no error for up-to-date version, got: %v", err)
	}
}

// fakeReleaseServer spins up an httptest server that mimics the GitHub
// Releases API and serves asset downloads. It returns the server and a
// cleanup function.
func fakeReleaseServer(t *testing.T, version string, binaryContent []byte) *httptest.Server {
	t.Helper()
	tarGz := makeTarGz(t, "arc", binaryContent)
	checksum := sha256.Sum256(tarGz)
	// Build checksum lines for all platform combinations so tests can
	// request any GOOS/GOARCH.
	var checksumLines []string
	for _, os := range []string{"linux", "darwin"} {
		for _, arch := range []string{"amd64", "arm64"} {
			name := fmt.Sprintf("arc_%s_%s_%s.tar.gz", version, os, arch)
			checksumLines = append(checksumLines, fmt.Sprintf("%x  %s", checksum, name))
		}
	}
	checksumBody := []byte(strings.Join(checksumLines, "\n") + "\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == fmt.Sprintf("/repos/%s/%s/releases/latest", repoOwner, repoName):
			// Build asset list for all platforms
			var assets []asset
			for _, os := range []string{"linux", "darwin"} {
				for _, arch := range []string{"amd64", "arm64"} {
					name := fmt.Sprintf("arc_%s_%s_%s.tar.gz", version, os, arch)
					assets = append(assets, asset{
						Name:               name,
						BrowserDownloadURL: fmt.Sprintf("%s/download/%s", r.Host, name),
					})
				}
			}
			assets = append(assets, asset{
				Name:               "checksums.txt",
				BrowserDownloadURL: fmt.Sprintf("%s/download/checksums.txt", r.Host),
			})
			// Rewrite URLs to use the test server
			for i := range assets {
				assets[i].BrowserDownloadURL = fmt.Sprintf("http://%s/download/%s", r.Host, assets[i].Name)
			}
			json.NewEncoder(w).Encode(release{
				TagName: "v" + version,
				Assets:  assets,
			})
		case strings.HasPrefix(r.URL.Path, "/download/checksums.txt"):
			w.Write(checksumBody)
		case strings.HasPrefix(r.URL.Path, "/download/arc_"):
			w.Write(tarGz)
		default:
			http.NotFound(w, r)
		}
	}))
	return srv
}

func TestFetchUpdateEndToEnd(t *testing.T) {
	binaryContent := []byte("#!/bin/sh\necho arc v2.0.0\n")
	srv := fakeReleaseServer(t, "2.0.0", binaryContent)
	defer srv.Close()

	binData, newVersion, err := fetchUpdate("1.0.0", srv.URL, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if newVersion != "2.0.0" {
		t.Fatalf("got version %q, want 2.0.0", newVersion)
	}
	if !bytes.Equal(binData, binaryContent) {
		t.Fatalf("binary content mismatch: got %q", binData)
	}
}

func TestFetchUpdateAlreadyCurrent(t *testing.T) {
	srv := fakeReleaseServer(t, "1.0.0", []byte("binary"))
	defer srv.Close()

	binData, _, err := fetchUpdate("1.0.0", srv.URL, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if binData != nil {
		t.Fatal("expected nil binData for already-current version")
	}
}

func TestFetchUpdateBadChecksum(t *testing.T) {
	// Serve a valid tarball but with wrong checksums
	binaryContent := []byte("binary")
	tarGz := makeTarGz(t, "arc", binaryContent)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "releases/latest"):
			json.NewEncoder(w).Encode(release{
				TagName: "v2.0.0",
				Assets: []asset{
					{Name: "arc_2.0.0_linux_amd64.tar.gz", BrowserDownloadURL: fmt.Sprintf("http://%s/download/arc_2.0.0_linux_amd64.tar.gz", r.Host)},
					{Name: "checksums.txt", BrowserDownloadURL: fmt.Sprintf("http://%s/download/checksums.txt", r.Host)},
				},
			})
		case strings.HasSuffix(r.URL.Path, "checksums.txt"):
			// Wrong checksum
			w.Write([]byte("0000000000000000000000000000000000000000000000000000000000000000  arc_2.0.0_linux_amd64.tar.gz\n"))
		default:
			w.Write(tarGz)
		}
	}))
	defer srv.Close()

	_, _, err := fetchUpdate("1.0.0", srv.URL, "linux", "amd64")
	if err == nil {
		t.Fatal("expected checksum error")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAtomicReplace(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "arc")
	if err := os.WriteFile(target, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}

	newContent := []byte("new-binary-content")
	if err := atomicReplace(target, newContent); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, newContent) {
		t.Fatalf("got %q, want %q", got, newContent)
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0755 {
		t.Fatalf("got permissions %o, want 0755", info.Mode().Perm())
	}
}

func TestIsGitHubHost(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://api.github.com/repos/foo/bar", true},
		{"https://github.com/foo/bar/releases/download/v1/file.tar.gz", true},
		{"https://uploads.github.com/something", true},
		{"http://api.github.com/repos/foo/bar", false},  // no HTTPS
		{"https://evil.com/api.github.com", false},       // wrong host
		{"https://notgithub.com/foo", false},              // wrong host
		{"https://api.github.com.evil.com/foo", false},    // subdomain trick
		{"not-a-url", false},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			if got := isGitHubHost(tt.url); got != tt.want {
				t.Fatalf("isGitHubHost(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

// makeTarGz creates a tar.gz archive containing a single file.
func makeTarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	if err := tw.WriteHeader(&tar.Header{
		Name:     name,
		Size:     int64(len(content)),
		Mode:     0755,
		Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
