package fetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gverdugo-dev/ine"
)

func TestRunWritesSkipsAndFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/DATOS_TABLA/1"):
			w.Write([]byte(`[{"COD":"S1","Nombre":"One","Data":[]}]`))
		case strings.HasSuffix(r.URL.Path, "/csv_bdsc/1.csv"):
			w.Write([]byte("a;b\n"))
		default:
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c := ine.New()
	c.BaseURL, c.CSVBaseURL = srv.URL, srv.URL

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "3.json"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	var seen int
	rs, err := Run(context.Background(), c, []string{"1", "2", "3"}, Options{Dir: dir, Formats: []Format{JSON, CSV}, Concurrency: 3}, func(Result) { seen++ })
	if err != nil {
		t.Fatal(err)
	}
	if seen != 6 || len(rs) != 6 {
		t.Fatalf("seen %d, results %d", seen, len(rs))
	}
	want := []Status{Done, Done, Failed, Failed, Skipped, Failed}
	for i, r := range rs {
		if r.Status != want[i] {
			t.Errorf("%s.%s = %s (%s), want %s", r.Table, r.Format, r.Status, r.Error, want[i])
		}
	}
	if rs[0].Series != 1 {
		t.Errorf("series = %d", rs[0].Series)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "1.json"))
	if !strings.Contains(string(b), `"code": "S1"`) {
		t.Errorf("1.json = %s", b)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "3.json")); string(b) != "old" {
		t.Errorf("existing file was overwritten")
	}
	if Failures(rs) != 3 {
		t.Errorf("failures = %d", Failures(rs))
	}
	leftovers, _ := filepath.Glob(filepath.Join(dir, ".ine-*"))
	if len(leftovers) != 0 {
		t.Errorf("temporary files left: %v", leftovers)
	}
}
