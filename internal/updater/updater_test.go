package updater

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T, badSum bool) *Client {
	t.Helper()
	archive := "release archive fixture"
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(archive)))
	if badSum {
		sum = strings.Repeat("0", 64)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer dedicated-secret" {
			t.Error("missing dedicated token")
		}
		switch r.URL.Path {
		case "/releases/latest":
			fmt.Fprint(w, `{"tag_name":"main-12.1-abc","assets":[{"id":1,"name":"custom-visning_Darwin_arm64.tar.gz"},{"id":2,"name":"checksums.txt"}]}`)
		case "/releases/assets/1":
			if r.Header.Get("Accept") != "application/octet-stream" {
				t.Error("wrong asset accept header")
			}
			fmt.Fprint(w, archive)
		case "/releases/assets/2":
			fmt.Fprintf(w, "%s  custom-visning_Darwin_arm64.tar.gz\n", sum)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	c := New("dedicated-secret")
	c.base = server.URL
	return c
}

func TestDownloadVerifiedAndNoOverwrite(t *testing.T) {
	c := fixture(t, false)
	release, err := c.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	asset, err := release.Archive("darwin", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := release.Archive("linux", "amd64"); err == nil {
		t.Fatal("unsupported platform accepted")
	}
	dir := t.TempDir()
	path, err := c.Download(context.Background(), release, asset, dir)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "release archive fixture" {
		t.Fatalf("download: %q %v", data, err)
	}
	if _, err := c.Download(context.Background(), release, asset, dir); err == nil {
		t.Fatal("overwrote archive")
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 1 {
		t.Fatalf("temporary files left: %v", files)
	}
}

func TestBadChecksumRemovesDownload(t *testing.T) {
	c := fixture(t, true)
	release, _ := c.Latest(context.Background())
	asset, _ := release.Archive("darwin", "arm64")
	dir := t.TempDir()
	if _, err := c.Download(context.Background(), release, asset, dir); err == nil {
		t.Fatal("bad checksum accepted")
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 0 {
		t.Fatalf("unverified files left: %v", files)
	}
}

func TestErrorsAreSanitized(t *testing.T) {
	for _, status := range []int{401, 403, 404, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				fmt.Fprint(w, "dedicated-secret private response")
			}))
			defer server.Close()
			c := New("dedicated-secret")
			c.base = server.URL
			_, err := c.Latest(context.Background())
			if err == nil || strings.Contains(err.Error(), "dedicated-secret") || strings.Contains(err.Error(), "private response") {
				t.Fatalf("unsafe error: %v", err)
			}
		})
	}
}

func TestRedirectStripsToken(t *testing.T) {
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("token leaked to storage")
		}
		fmt.Fprint(w, "asset")
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer source.Close()
	c := New("dedicated-secret")
	c.base = source.URL
	c.http.Transport = target.Client().Transport
	var out strings.Builder
	if err := c.get(context.Background(), "/asset", "application/octet-stream", 100, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "asset" {
		t.Fatal("missing redirected asset")
	}
}

func TestDownloadBoundsAndCancellation(t *testing.T) {
	c := fixture(t, false)
	var out strings.Builder
	if err := c.get(context.Background(), "/releases/assets/1", "application/octet-stream", 3, &out); err == nil {
		t.Fatal("size limit ignored")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Latest(ctx); err != context.Canceled {
		t.Fatalf("cancellation: %v", err)
	}
	release, _ := c.Latest(context.Background())
	asset, _ := release.Archive("darwin", "arm64")
	asset.Name = "../outside.tar.gz"
	dir := t.TempDir()
	if _, err := c.Download(context.Background(), release, asset, dir); err == nil {
		t.Fatal("path traversal accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, "outside.tar.gz")); !os.IsNotExist(err) {
		t.Fatal("unexpected file")
	}
}

func TestMalformedReleaseAndChecksums(t *testing.T) {
	for _, body := range []string{`{`, `{}`, `{"tag_name":"v1","draft":true}`, `{"tag_name":"v1","prerelease":true}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		c := New("secret")
		c.base = server.URL
		if _, err := c.Latest(context.Background()); err == nil {
			t.Errorf("accepted metadata %s", body)
		}
		server.Close()
	}
	for _, sum := range []string{"", "invalid  archive.tar.gz", strings.Repeat("0", 64) + "  other.tar.gz", strings.Repeat("0", 64) + "  archive.tar.gz\n" + strings.Repeat("0", 64) + "  archive.tar.gz"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/releases/assets/2" {
				t.Error("archive requested before valid checksum")
			}
			fmt.Fprint(w, sum)
		}))
		c := New("secret")
		c.base = server.URL
		release := Release{Assets: []Asset{{ID: 2, Name: "checksums.txt"}}}
		dir := t.TempDir()
		if _, err := c.Download(context.Background(), release, Asset{ID: 1, Name: "archive.tar.gz"}, dir); err == nil {
			t.Errorf("accepted checksum %q", sum)
		}
		files, _ := os.ReadDir(dir)
		if len(files) != 0 {
			t.Error("download left files")
		}
		server.Close()
	}
}

func TestRejectHTTPRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://example.com/asset", http.StatusFound)
	}))
	defer server.Close()
	c := New("secret")
	c.base = server.URL
	var out strings.Builder
	if err := c.get(context.Background(), "/asset", "application/octet-stream", 100, &out); err == nil {
		t.Fatal("insecure redirect accepted")
	}
}
