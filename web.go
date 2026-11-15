package ine

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// ErrNoWebPage is returned by WebTables for an operation the API gives no
// INEbase page for.
var ErrNoWebPage = errors.New("ine: the operation has no INEbase page")

// maxWebPages bounds the crawl of one operation, as a guard against an index
// that links in circles.
const maxWebPages = 400

var (
	// Links from an operation's results tab to its INEbase indexes.
	reIndexLink = regexp.MustCompile(`/dynt3/inebase/(?:[a-z]{2}/)?index\.htm\?padre=(\d+)(?:&(?:amp;)?capsel=(\d+))?`)
	// A collapsed or expanded chapter inside an index page.
	reChapter = regexp.MustCompile(`(?s)<a id="c_(\d+)"[^>]*?href="\?padre=(\d+)&(?:amp;)?capsel=\d+"[^>]*>([^<]*)</a>`)
	// A table link, Tempus3 (t=) or PC-Axis served through Tempus3 (tpx=).
	reTable = regexp.MustCompile(`<a href="/jaxi(?:T3)?/Tabla\.htm\?(?:t|tpx)=(\d+)[^"]*"[^>]*>([^<]+)</a>`)
	// The title of an index page.
	reTitle = regexp.MustCompile(`<h1 class="inebase_titulo" id="c_\d+">([^<]*)</h1>`)
)

// WebTables finds the tables of an operation by reading its INEbase pages on
// www.ine.es. It exists because TABLAS_OPERACION leaves out many tables that
// DATOS_TABLA serves: most of the PC-Axis ("tpx") ones and some Tempus3 ones.
// Each table carries the chapter it hangs from in Group.
func (c *Client) WebTables(ctx context.Context, op Operation) ([]Table, error) {
	if op.URL == "" {
		return nil, fmt.Errorf("%w: %s", ErrNoWebPage, op.Code)
	}
	results := op.URL
	if strings.HasPrefix(results, "/") {
		results = c.WebBaseURL + results
	}
	if strings.Contains(results, "operacion.htm") && !strings.Contains(results, "menu=") {
		results += "&menu=resultados"
	}
	page, err := c.getPage(ctx, results)
	if err != nil {
		return nil, err
	}

	w := webCrawl{seen: map[string]bool{}, tables: map[int]bool{}}
	w.collectTables(page, op.Name)
	type job struct{ padre, capsel, group string }
	var queue []job
	for _, m := range reIndexLink.FindAllStringSubmatch(page, -1) {
		queue = append(queue, job{padre: m[1], capsel: m[2]})
	}
	for len(queue) > 0 && w.pages < maxWebPages {
		j := queue[0]
		queue = queue[1:]
		key := j.padre + "/" + j.capsel
		if w.seen[key] {
			continue
		}
		w.seen[key] = true
		u := c.WebBaseURL + "/dynt3/inebase/index.htm?padre=" + j.padre
		if j.capsel != "" {
			u += "&capsel=" + j.capsel
		}
		body, err := c.getPage(ctx, u)
		w.pages++
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return w.out, err
		}
		chapters := reChapter.FindAllStringSubmatch(body, -1)
		group := j.group
		for _, m := range chapters {
			if group == "" && m[1] == j.capsel {
				group = clean(m[3])
			}
		}
		if group == "" {
			if m := reTitle.FindStringSubmatch(body); m != nil {
				group = clean(m[1])
			}
		}
		w.collectTables(body, group)
		for _, m := range chapters {
			if m[2] == j.padre && !w.seen[m[2]+"/"+m[1]] {
				queue = append(queue, job{padre: m[2], capsel: m[1], group: clean(m[3])})
			}
		}
	}
	return w.out, nil
}

type webCrawl struct {
	seen   map[string]bool
	tables map[int]bool
	out    []Table
	pages  int
}

func (w *webCrawl) collectTables(body, group string) {
	for _, m := range reTable.FindAllStringSubmatch(body, -1) {
		id, err := strconv.Atoi(m[1])
		if err != nil || w.tables[id] {
			continue
		}
		w.tables[id] = true
		w.out = append(w.out, Table{ID: id, Name: clean(m[2]), Group: group, FromWeb: true})
	}
}

func clean(s string) string {
	return strings.Join(strings.Fields(html.UnescapeString(s)), " ")
}

func (c *Client) getPage(ctx context.Context, u string) (string, error) {
	resp, err := c.do(ctx, u)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}

// AllTables returns the tables of an operation from the API and, when the API
// lists none, from its INEbase pages. It asks by numeric id because some codes
// (OCE, CTNFSI) are shared by more than one operation.
func (c *Client) AllTables(ctx context.Context, op Operation) ([]Table, error) {
	tables, err := c.Tables(ctx, strconv.Itoa(op.ID))
	if err != nil || len(tables) > 0 {
		return tables, err
	}
	web, err := c.WebTables(ctx, op)
	if errors.Is(err, ErrNoWebPage) {
		return nil, nil
	}
	return web, err
}
