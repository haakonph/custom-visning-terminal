// Package updater checks, downloads, and installs releases.
package updater

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const repository = "https://api.github.com/repos/haakonph/custom-visning-terminal"
const maxArchive = 256 << 20

type Asset struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}
type Release struct {
	Tag        string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}
type Client struct {
	token string
	http  *http.Client
	base  string
}

func New(token string) *Client {
	// Authorization is attached only to the initial API request. Release assets
	// redirect to signed storage URLs, which must never receive the token.
	return &Client{token: token, base: repository, http: &http.Client{
		Timeout: 2 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			req.Header.Del("Authorization")
			if len(via) >= 10 || req.URL.Scheme != "https" {
				return errors.New("unsafe release redirect")
			}
			return nil
		},
	}}
}

func (c *Client) get(ctx context.Context, endpoint, accept string, limit int64, out io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+endpoint, nil)
	if err != nil {
		return errors.New("could not prepare release request")
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", accept)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("release request failed; check network access")
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case 401:
		return errors.New("update token rejected; check CUSTOM_VISNING_UPDATE_TOKEN (or GITHUB_TOKEN/gh when no dedicated token is set)")
	case 403, 429:
		return errors.New("release access denied or rate limited; check update token permissions and try again later")
	case 404:
		return errors.New("release or asset unavailable; check repository access and published releases")
	default:
		return fmt.Errorf("release request returned HTTP %d", resp.StatusCode)
	}
	n, err := io.Copy(out, io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return errors.New("could not read release download")
	}
	if n > limit {
		return errors.New("release download exceeds size limit")
	}
	return nil
}

func (c *Client) Latest(ctx context.Context) (Release, error) {
	var data strings.Builder
	if err := c.get(ctx, "/releases/latest", "application/vnd.github+json", 2<<20, &data); err != nil {
		return Release{}, err
	}
	var release Release
	if err := json.Unmarshal([]byte(data.String()), &release); err != nil || release.Tag == "" || release.Draft || release.Prerelease {
		return Release{}, errors.New("invalid stable release metadata")
	}
	return release, nil
}

func (r Release) Archive(goos, arch string) (Asset, error) {
	platform := map[string]string{"darwin": "Darwin", "linux": "Linux", "windows": "Windows"}[goos]
	name := "custom-visning_" + platform + "_" + arch + ".tar.gz"
	for _, asset := range r.Assets {
		if platform != "" && asset.Name == name && asset.ID > 0 {
			return asset, nil
		}
	}
	return Asset{}, fmt.Errorf("no release archive for %s/%s; current releases support macOS arm64", goos, arch)
}

// Download publishes the archive only after checksum verification. Existing
// files are never overwritten, and partial or rejected downloads are removed.
func (c *Client) Download(ctx context.Context, release Release, asset Asset, dir string) (string, error) {
	if asset.ID <= 0 || filepath.Base(asset.Name) != asset.Name || asset.Name == "." || asset.Name == "" {
		return "", errors.New("invalid release asset")
	}
	var checksum Asset
	for _, a := range release.Assets {
		if a.Name == "checksums.txt" && a.ID > 0 {
			checksum = a
			break
		}
	}
	if checksum.ID == 0 {
		return "", errors.New("release has no checksums.txt")
	}
	var sums strings.Builder
	if err := c.get(ctx, fmt.Sprintf("/releases/assets/%d", checksum.ID), "application/octet-stream", 1<<20, &sums); err != nil {
		return "", err
	}
	var expected string
	for _, line := range strings.Split(sums.String(), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == asset.Name {
			if expected != "" {
				return "", errors.New("ambiguous release checksum")
			}
			expected = strings.ToLower(fields[0])
		}
	}
	decoded, err := hex.DecodeString(expected)
	if err != nil || len(decoded) != sha256.Size {
		return "", errors.New("release archive has no valid SHA-256 checksum")
	}
	file, err := os.CreateTemp(dir, ".custom-visning-update-*")
	if err != nil {
		return "", errors.New("could not create update download")
	}
	defer os.Remove(file.Name())
	defer file.Close()
	hash := sha256.New()
	if err := c.get(ctx, fmt.Sprintf("/releases/assets/%d", asset.ID), "application/octet-stream", maxArchive, io.MultiWriter(file, hash)); err != nil {
		return "", err
	}
	if hex.EncodeToString(hash.Sum(nil)) != expected {
		return "", errors.New("release checksum mismatch; download discarded")
	}
	if err := file.Close(); err != nil {
		return "", errors.New("could not finish update download")
	}
	destination := filepath.Join(dir, asset.Name)
	if err := os.Link(file.Name(), destination); err != nil {
		return "", errors.New("could not save verified archive; check directory permissions and remove any existing archive")
	}
	return destination, nil
}

// ReplaceArchive extracts the release binary and atomically replaces target.
func ReplaceArchive(archivePath, target string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return errors.New("could not open verified update")
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return errors.New("invalid release archive")
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var data []byte
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.New("could not read release archive")
		}
		if h.Name == "custom-visning" {
			if !h.FileInfo().Mode().IsRegular() || h.Size <= 0 || h.Size > maxArchive {
				return errors.New("release archive contains an invalid binary")
			}
			data, err = io.ReadAll(io.LimitReader(tr, h.Size+1))
			if err != nil || int64(len(data)) != h.Size {
				return errors.New("could not extract release binary")
			}
		}
	}
	if len(data) == 0 {
		return errors.New("release archive has no custom-visning binary")
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".custom-visning-install-*")
	if err != nil {
		return errors.New("could not create replacement binary")
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0755); err != nil {
		tmp.Close()
		return errors.New("could not set replacement permissions")
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return errors.New("could not write replacement binary")
	}
	if err := tmp.Close(); err != nil {
		return errors.New("could not finish replacement binary")
	}
	if err := os.Rename(tmpName, target); err != nil {
		return errors.New("could not replace running binary; check permissions")
	}
	return nil
}
