package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func tarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "README.md", Mode: 0o644, Size: 2, Typeflag: tar.TypeReg})
	tw.Write([]byte("hi"))
	tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg})
	tw.Write(content)
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func TestNewer(t *testing.T) {
	cases := []struct {
		tag, current string
		want         bool
	}{
		{"v0.3.0", "0.2.0", true},
		{"v0.3.0", "v0.3.0", false},
		{"v0.2.9", "v0.10.0", false},
		{"v1.0.0", "dev", true},
		{"v1.0.0", "v0.0.0-20261115165452-6e16ffa5efd6+dirty", true},
		{"garbage", "0.1.0", false},
	}
	for _, c := range cases {
		if got := Newer(c.tag, c.current); got != c.want {
			t.Errorf("Newer(%q, %q) = %v", c.tag, c.current, got)
		}
	}
}

func TestLatestAndDownload(t *testing.T) {
	bin := []byte("new ine binary")
	archive := tarGz(t, "ine", bin)
	sum := sha256.Sum256(archive)
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/releases/tag/v0.3.0", http.StatusFound)
	})
	mux.HandleFunc("/releases/download/v0.3.0/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(hex.EncodeToString(sum[:]) + "  ine_linux_arm64.tar.gz\n"))
	})
	mux.HandleFunc("/releases/download/v0.3.0/ine_linux_arm64.tar.gz", func(w http.ResponseWriter, r *http.Request) {
		w.Write(archive)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	u := &Updater{BaseURL: srv.URL, HTTP: srv.Client(), GOOS: "linux", GOARCH: "arm64"}
	ctx := context.Background()
	tag, err := u.Latest(ctx)
	if err != nil || tag != "v0.3.0" {
		t.Fatalf("Latest = %q, %v", tag, err)
	}
	got, err := u.Download(ctx, tag)
	if err != nil || !bytes.Equal(got, bin) {
		t.Fatalf("Download = %q, %v", got, err)
	}

	u.GOARCH = "amd64"
	if _, err := u.Download(ctx, tag); err == nil {
		t.Fatal("expected an error for a platform without a build")
	}
}

func TestDownloadRejectsBadChecksum(t *testing.T) {
	archive := tarGz(t, "ine", []byte("x"))
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/download/v1.0.0/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("0000  ine_darwin_arm64.tar.gz\n"))
	})
	mux.HandleFunc("/releases/download/v1.0.0/ine_darwin_arm64.tar.gz", func(w http.ResponseWriter, r *http.Request) {
		w.Write(archive)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	u := &Updater{BaseURL: srv.URL, HTTP: srv.Client(), GOOS: "darwin", GOARCH: "arm64"}
	if _, err := u.Download(context.Background(), "v1.0.0"); err == nil {
		t.Fatal("expected a checksum error")
	}
}

func TestReplace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ine")
	if err := os.WriteFile(path, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Replace(path, []byte("new")); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	st, _ := os.Stat(path)
	if string(b) != "new" || st.Mode().Perm() != 0o755 {
		t.Fatalf("content %q mode %v", b, st.Mode())
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".ine-update-*"))
	if len(leftovers) != 0 {
		t.Fatalf("temporary files left: %v", leftovers)
	}
}
