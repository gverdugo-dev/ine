// Package fetch downloads many INE tables at once, in parallel and with a
// shared request rate, and writes one file per table and format.
package fetch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gverdugo-dev/ine"
)

// Format is an output format for a table.
type Format string

const (
	// JSON writes the normalized series of the table (ine.Series).
	JSON Format = "json"
	// CSV writes the CSV the INE publishes, byte for byte.
	CSV Format = "csv"
)

// Options configures a bulk download.
type Options struct {
	Dir         string
	Formats     []Format
	Query       ine.Query
	Concurrency int
	// Interval is the minimum time between two requests across all workers.
	Interval time.Duration
	// Force overwrites files that already exist. Without it they are skipped.
	Force bool
}

// Status of one file.
type Status string

const (
	Done    Status = "done"
	Skipped Status = "skipped"
	Failed  Status = "failed"
)

// Result describes what happened to one table in one format.
type Result struct {
	Table    string        `json:"table"`
	Format   Format        `json:"format"`
	Path     string        `json:"path,omitempty"`
	Status   Status        `json:"status"`
	Series   int           `json:"series,omitempty"`
	Bytes    int64         `json:"bytes,omitempty"`
	Error    string        `json:"error,omitempty"`
	Duration time.Duration `json:"durationNs"`
}

// Run downloads every table in every format. progress, when not nil, is called
// once per result as it finishes, from a single goroutine. The returned slice
// keeps the input order.
func Run(ctx context.Context, c *ine.Client, tables []string, opts Options, progress func(Result)) ([]Result, error) {
	if len(opts.Formats) == 0 {
		opts.Formats = []Format{JSON}
	}
	if opts.Concurrency < 1 {
		opts.Concurrency = 4
	}
	if err := os.MkdirAll(opts.Dir, 0o755); err != nil {
		return nil, err
	}

	type job struct {
		i      int
		table  string
		format Format
	}
	var jobs []job
	for _, t := range tables {
		for _, f := range opts.Formats {
			jobs = append(jobs, job{len(jobs), t, f})
		}
	}

	results := make([]Result, len(jobs))
	queue := make(chan job)
	done := make(chan Result)
	limiter := newLimiter(opts.Interval)
	defer limiter.stop()

	var wg sync.WaitGroup
	for w := 0; w < opts.Concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range queue {
				r := one(ctx, c, j.table, j.format, opts, limiter)
				results[j.i] = r
				done <- r
			}
		}()
	}
	go func() {
		defer close(queue)
		for _, j := range jobs {
			select {
			case queue <- j:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(done)
	}()
	for r := range done {
		if progress != nil {
			progress(r)
		}
	}
	return results, ctx.Err()
}

func one(ctx context.Context, c *ine.Client, table string, format Format, opts Options, l *limiter) Result {
	start := time.Now()
	path := filepath.Join(opts.Dir, table+"."+string(format))
	r := Result{Table: table, Format: format, Path: path}
	finish := func(err error) Result {
		r.Duration = time.Since(start)
		if err != nil {
			r.Status = Failed
			r.Error = err.Error()
			r.Path = ""
		} else if r.Status == "" {
			r.Status = Done
		}
		return r
	}

	if !opts.Force {
		if st, err := os.Stat(path); err == nil {
			r.Status = Skipped
			r.Bytes = st.Size()
			return finish(nil)
		}
	}
	if err := l.wait(ctx); err != nil {
		return finish(err)
	}

	switch format {
	case JSON:
		series, err := c.TableData(ctx, table, opts.Query)
		if err != nil {
			return finish(err)
		}
		b, err := json.MarshalIndent(series, "", "  ")
		if err != nil {
			return finish(err)
		}
		r.Series = len(series)
		r.Bytes = int64(len(b))
		return finish(writeAtomic(path, func(w io.Writer) error {
			_, err := w.Write(b)
			return err
		}))
	case CSV:
		body, err := c.TableCSV(ctx, table)
		if err != nil {
			return finish(err)
		}
		defer body.Close()
		return finish(writeAtomic(path, func(w io.Writer) error {
			n, err := io.Copy(w, body)
			r.Bytes = n
			return err
		}))
	default:
		return finish(fmt.Errorf("unknown format %q", format))
	}
}

// writeAtomic writes to a temporary file and renames it, so an interrupted
// download never leaves a half file that a later run would skip.
func writeAtomic(path string, write func(io.Writer) error) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".ine-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := write(tmp); err != nil {
		tmp.Close()
		return err
	}
	// CreateTemp makes the file 0600; downloads are ordinary data files.
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// limiter lets one request through per interval, shared by all workers.
type limiter struct {
	ticker *time.Ticker
}

func newLimiter(d time.Duration) *limiter {
	if d <= 0 {
		return &limiter{}
	}
	return &limiter{ticker: time.NewTicker(d)}
}

func (l *limiter) wait(ctx context.Context) error {
	if l.ticker == nil {
		return ctx.Err()
	}
	select {
	case <-l.ticker.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (l *limiter) stop() {
	if l.ticker != nil {
		l.ticker.Stop()
	}
}

// WriteManifest writes manifest.json in dir with the outcome of every file.
func WriteManifest(dir, lang string, results []Result) error {
	b, err := json.MarshalIndent(map[string]any{
		"fetchedAt": time.Now().UTC(), "lang": lang, "results": results,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0o644)
}

// Failures counts the failed results.
func Failures(rs []Result) int {
	n := 0
	for _, r := range rs {
		if r.Status == Failed {
			n++
		}
	}
	return n
}

// ErrSomeFailed is returned by callers that want a non zero exit when any
// table failed.
var ErrSomeFailed = errors.New("some tables failed")
