package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/gverdugo-dev/ine"
	"github.com/gverdugo-dev/ine/fetch"
	"github.com/gverdugo-dev/ine/internal/tui"
)

// common holds the flags every command shares.
type common struct {
	lang    string
	asJSON  bool
	last    int
	from    string
	to      string
	periods bool
}

func newFlags(name string, c *common, periods bool) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.StringVar(&c.lang, "lang", "ES", "language of names: ES or EN")
	fs.BoolVar(&c.asJSON, "json", false, "print JSON instead of a table")
	if periods {
		c.periods = true
		fs.IntVar(&c.last, "last", 0, "only the last N periods (0 = all)")
		fs.StringVar(&c.from, "from", "", "first period, YYYY-MM-DD")
		fs.StringVar(&c.to, "to", "", "last period, YYYY-MM-DD")
	}
	return fs
}

func (c *common) client() *ine.Client {
	cl := ine.New()
	cl.Lang = strings.ToUpper(c.lang)
	return cl
}

func (c *common) query() (ine.Query, error) {
	q := ine.Query{Last: c.last}
	var err error
	if c.from != "" {
		if q.From, err = time.Parse("2006-01-02", c.from); err != nil {
			return q, fmt.Errorf("--from: %w", err)
		}
	}
	if c.to != "" {
		if q.To, err = time.Parse("2006-01-02", c.to); err != nil {
			return q, fmt.Errorf("--to: %w", err)
		}
	}
	return q, nil
}

// parse accepts flags before and after the positional arguments.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func matches(filter string, fields ...string) bool {
	if filter == "" {
		return true
	}
	f := fold(filter)
	for _, s := range fields {
		if strings.Contains(fold(s), f) {
			return true
		}
	}
	return false
}

// fold lowercases and drops Spanish accents so "indice" finds "Índice".
func fold(s string) string {
	r := strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n")
	return r.Replace(strings.ToLower(s))
}

func cmdOps(ctx context.Context, args []string) error {
	var c common
	fs := newFlags("ops", &c, false)
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	ops, err := c.client().Operations(ctx)
	if err != nil {
		return err
	}
	filter := strings.Join(pos, " ")
	var out []ine.Operation
	for _, o := range ops {
		if matches(filter, o.Code, o.Name, o.IOECode) {
			out = append(out, o)
		}
	}
	if c.asJSON {
		return printJSON(out)
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tCODE\tNAME")
	for _, o := range out {
		fmt.Fprintf(tw, "%d\t%s\t%s\n", o.ID, o.Code, o.Name)
	}
	return tw.Flush()
}

func cmdTables(ctx context.Context, args []string) error {
	var c common
	fs := newFlags("tables", &c, false)
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		return fmt.Errorf("tables needs an operation code, for example: ine tables IPC")
	}
	tables, err := c.client().Tables(ctx, pos[0])
	if err != nil {
		return err
	}
	filter := strings.Join(pos[1:], " ")
	var out []ine.Table
	for _, t := range tables {
		if matches(filter, t.Name, strconv.Itoa(t.ID)) {
			out = append(out, t)
		}
	}
	if c.asJSON {
		return printJSON(out)
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tUPDATED\tNAME")
	for _, t := range out {
		updated := ""
		if !t.LastModified.IsZero() {
			updated = t.LastModified.Format("2006-01-02")
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\n", t.ID, updated, t.Name)
	}
	return tw.Flush()
}

func cmdGet(ctx context.Context, args []string) error {
	var c common
	fs := newFlags("get", &c, true)
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return fmt.Errorf("get needs one table id, for example: ine get 50902 --last 3")
	}
	q, err := c.query()
	if err != nil {
		return err
	}
	if q.Last == 0 && q.From.IsZero() && q.To.IsZero() && !c.asJSON {
		q.Last = 1
	}
	series, err := c.client().TableData(ctx, pos[0], q)
	if err != nil {
		return err
	}
	if c.asJSON {
		return printJSON(series)
	}
	return printSeries(os.Stdout, series)
}

func cmdSeries(ctx context.Context, args []string) error {
	var c common
	fs := newFlags("series", &c, true)
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return fmt.Errorf("series needs one series code, for example: ine series IPC251852 --last 12")
	}
	q, err := c.query()
	if err != nil {
		return err
	}
	s, err := c.client().SeriesData(ctx, pos[0], q)
	if err != nil {
		return err
	}
	if c.asJSON {
		return printJSON(s)
	}
	return printSeries(os.Stdout, []ine.Series{s})
}

func printSeries(w io.Writer, series []ine.Series) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "SERIES\tNAME\tPERIOD\tVALUE")
	for _, s := range series {
		for _, p := range s.Data {
			v := "-"
			if p.Value != nil {
				v = strconv.FormatFloat(*p.Value, 'f', -1, 64)
			}
			fmt.Fprintf(tw, "%s\t%s\t%d%s\t%s\n", s.Code, s.Name, p.Year, p.Period, v)
		}
	}
	return tw.Flush()
}

func cmdFetch(ctx context.Context, args []string) error {
	var c common
	fs := newFlags("fetch", &c, true)
	var (
		file     = fs.String("f", "", "file with one table id per line (# starts a comment)")
		out      = fs.String("o", "ine-data", "output directory")
		formats  = fs.String("format", "json", "comma separated formats: json, csv")
		jobs     = fs.Int("j", 4, "parallel downloads")
		interval = fs.Duration("interval", 200*time.Millisecond, "minimum time between requests")
		force    = fs.Bool("force", false, "overwrite files that already exist")
		ops      multi
	)
	fs.Var(&ops, "op", "also download every table of this operation (repeatable)")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	q, err := c.query()
	if err != nil {
		return err
	}
	cl := c.client()

	tables := append([]string{}, pos...)
	if *file != "" {
		fromFile, err := readTableList(*file)
		if err != nil {
			return err
		}
		tables = append(tables, fromFile...)
	}
	for _, op := range ops {
		ts, err := cl.Tables(ctx, op)
		if err != nil {
			return fmt.Errorf("operation %s: %w", op, err)
		}
		for _, t := range ts {
			tables = append(tables, strconv.Itoa(t.ID))
		}
	}
	tables = dedupe(tables)
	if len(tables) == 0 {
		return fmt.Errorf("fetch needs tables: ids as arguments, -f <file> or --op <operation>")
	}

	var fmts []fetch.Format
	for _, f := range strings.Split(*formats, ",") {
		switch f = strings.TrimSpace(strings.ToLower(f)); f {
		case "json", "csv":
			fmts = append(fmts, fetch.Format(f))
		default:
			return fmt.Errorf("--format: unknown format %q", f)
		}
	}

	total := len(tables) * len(fmts)
	n := 0
	progress := func(r fetch.Result) {
		n++
		if c.asJSON {
			return
		}
		line := fmt.Sprintf("[%d/%d] %-8s %s.%s", n, total, r.Status, r.Table, r.Format)
		if r.Error != "" {
			line += "  " + r.Error
		}
		fmt.Fprintln(os.Stderr, line)
	}
	results, err := fetch.Run(ctx, cl, tables, fetch.Options{
		Dir: *out, Formats: fmts, Query: q, Concurrency: *jobs, Interval: *interval, Force: *force,
	}, progress)
	if err != nil {
		return err
	}
	if err := fetch.WriteManifest(*out, cl.Lang, results); err != nil {
		return err
	}
	if c.asJSON {
		if err := printJSON(results); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(os.Stderr, "%d files in %s, %d failed\n", total, *out, fetch.Failures(results))
	}
	if fetch.Failures(results) > 0 {
		return fetch.ErrSomeFailed
	}
	return nil
}

func cmdTUI(ctx context.Context, args []string) error {
	var c common
	fs := newFlags("tui", &c, false)
	out := fs.String("o", "ine-data", "output directory")
	jobs := fs.Int("j", 4, "parallel downloads")
	interval := fs.Duration("interval", 200*time.Millisecond, "minimum time between requests")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	return tui.Run(ctx, c.client(), tui.Options{OutDir: *out, Jobs: *jobs, Interval: *interval})
}

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

func readTableList(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		out = append(out, strings.Fields(line)...)
	}
	return out, sc.Err()
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
