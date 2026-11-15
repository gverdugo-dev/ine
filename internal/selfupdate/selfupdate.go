// Package selfupdate replaces the running ine binary with the one from a
// GitHub release, checking it against the release's checksums.txt.
package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Updater knows where the releases live. The zero value is not usable: build
// it with New.
type Updater struct {
	// BaseURL is the repository page, for example https://github.com/owner/ine.
	BaseURL string
	HTTP    *http.Client
	GOOS    string
	GOARCH  string
}

// New returns an updater for the public ine repository.
func New() *Updater {
	return &Updater{
		BaseURL: "https://github.com/gverdugo-dev/ine",
		HTTP:    &http.Client{},
		GOOS:    runtime.GOOS,
		GOARCH:  runtime.GOARCH,
	}
}

// Latest returns the tag of the latest release. It reads the redirect of
// /releases/latest instead of the REST API, which has a low anonymous quota.
func (u *Updater) Latest(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.BaseURL+"/releases/latest", nil)
	if err != nil {
		return "", err
	}
	client := *u.HTTP
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	loc := resp.Header.Get("Location")
	i := strings.LastIndex(loc, "/tag/")
	if i < 0 {
		return "", fmt.Errorf("no latest release found (status %d)", resp.StatusCode)
	}
	return loc[i+len("/tag/"):], nil
}

// Newer reports whether tag is a newer version than current. A current
// version that is not a release (dev, a pseudo-version) is always older.
func Newer(tag, current string) bool {
	t, ok := parse(tag)
	if !ok {
		return false
	}
	c, ok := parse(current)
	if !ok {
		return true
	}
	for i := range t {
		if t[i] != c[i] {
			return t[i] > c[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// archiveName is the GoReleaser name_template of .goreleaser.yaml.
func (u *Updater) archiveName() string {
	if u.GOOS == "windows" {
		return fmt.Sprintf("ine_%s_%s.zip", u.GOOS, u.GOARCH)
	}
	return fmt.Sprintf("ine_%s_%s.tar.gz", u.GOOS, u.GOARCH)
}

// Download fetches the binary of a release for this platform and checks it
// against checksums.txt.
func (u *Updater) Download(ctx context.Context, tag string) ([]byte, error) {
	base := u.BaseURL + "/releases/download/" + tag + "/"
	name := u.archiveName()
	sums, err := u.get(ctx, base+"checksums.txt")
	if err != nil {
		return nil, err
	}
	want := ""
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && f[1] == name {
			want = f[0]
		}
	}
	if want == "" {
		return nil, fmt.Errorf("release %s has no build for %s/%s", tag, u.GOOS, u.GOARCH)
	}
	archive, err := u.get(ctx, base+name)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(archive)
	if hex.EncodeToString(sum[:]) != want {
		return nil, fmt.Errorf("checksum mismatch for %s", name)
	}
	if u.GOOS == "windows" {
		return fromZip(archive, "ine.exe")
	}
	return fromTarGz(archive, "ine")
}

func (u *Updater) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := u.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: status %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func fromTarGz(b []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if filepath.Base(h.Name) == name && h.Typeflag == tar.TypeReg {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("%s not found in the archive", name)
}

func fromZip(b []byte, name string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if filepath.Base(f.Name) == name {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("%s not found in the archive", name)
}

// ErrNotWritable is returned when the binary's directory cannot be written.
var ErrNotWritable = errors.New("cannot write the ine binary")

// Replace writes bin over the executable at path. It writes a temporary file
// next to it and renames it, so a failure never leaves a broken binary. On
// Windows the running executable cannot be overwritten, so it is moved aside
// first.
func Replace(path string, bin []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".ine-update-*")
	if err != nil {
		return fmt.Errorf("%w in %s: %v", ErrNotWritable, dir, err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(bin); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o755); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		old := path + ".old"
		os.Remove(old)
		if err := os.Rename(path, old); err != nil {
			return fmt.Errorf("%w: %v", ErrNotWritable, err)
		}
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("%w: %v", ErrNotWritable, err)
	}
	return nil
}
