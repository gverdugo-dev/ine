// Package ine is a client for the JSON API (Tempus3) of the Instituto Nacional
// de Estadistica, the Spanish statistics office.
//
// The API needs no key. Its URLs have the shape
// https://servicios.ine.es/wstempus/js/{lang}/{FUNCTION}/{input}?{params}.
// The client always asks for the friendly form (tip=AM), so dates come as
// strings and every series carries its unit and its variables.
package ine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the root of the Tempus3 JSON API.
const DefaultBaseURL = "https://servicios.ine.es/wstempus/js"

// DefaultCSVBaseURL is the root of the per table CSV downloads.
const DefaultCSVBaseURL = "https://www.ine.es/jaxiT3/files/t"

// ErrNotFound is returned when the INE answers with an HTML error page,
// which is what it does for an unknown table, series or operation.
var ErrNotFound = errors.New("ine: not found")

// Client talks to the INE API. The zero value is not usable: build it with New.
type Client struct {
	BaseURL    string
	CSVBaseURL string
	// Lang is "ES" or "EN".
	Lang string
	HTTP *http.Client
	// Retries is how many times a request is repeated after a transient
	// failure (network error, 429 or 5xx other than the HTML not found page).
	Retries int
	// UserAgent is sent with every request.
	UserAgent string
}

// New returns a client with sensible defaults.
func New() *Client {
	return &Client{
		BaseURL:    DefaultBaseURL,
		CSVBaseURL: DefaultCSVBaseURL,
		Lang:       "ES",
		HTTP:       &http.Client{Timeout: 60 * time.Second},
		Retries:    3,
		UserAgent:  "ine-go-client",
	}
}

// Query narrows the periods returned by a data call. The zero value asks for
// every period, which can be large for long series.
type Query struct {
	// Last keeps only the last N periods (nult).
	Last int
	// From and To bound the period (date=YYYYMMDD:YYYYMMDD). Either can be
	// zero.
	From, To time.Time
}

func (q Query) values() url.Values {
	v := url.Values{}
	v.Set("tip", "AM")
	if q.Last > 0 {
		v.Set("nult", strconv.Itoa(q.Last))
	}
	if !q.From.IsZero() || !q.To.IsZero() {
		v.Set("date", formatDate(q.From)+":"+formatDate(q.To))
	}
	return v
}

func formatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("20060102")
}

// Operations lists every statistical operation the API publishes.
func (c *Client) Operations(ctx context.Context) ([]Operation, error) {
	var out []Operation
	err := c.getJSON(ctx, "OPERACIONES_DISPONIBLES", "", nil, &out)
	return out, err
}

// Operation returns one operation by its code (for example "IPC"), its
// numeric id or its IOE code.
func (c *Client) Operation(ctx context.Context, code string) (Operation, error) {
	var out Operation
	err := c.getJSON(ctx, "OPERACION", code, nil, &out)
	return out, err
}

// Tables lists the tables of an operation.
func (c *Client) Tables(ctx context.Context, operation string) ([]Table, error) {
	var wire []wireTable
	if err := c.getJSON(ctx, "TABLAS_OPERACION", operation, nil, &wire); err != nil {
		return nil, err
	}
	out := make([]Table, len(wire))
	for i, w := range wire {
		out[i] = w.table()
	}
	return out, nil
}

// TableData returns every series of a table with its data points.
func (c *Client) TableData(ctx context.Context, table string, q Query) ([]Series, error) {
	var out []Series
	err := c.getJSON(ctx, "DATOS_TABLA", table, q.values(), &out)
	return out, err
}

// SeriesData returns one series by its code (for example "IPC251852").
func (c *Client) SeriesData(ctx context.Context, series string, q Query) (Series, error) {
	var out Series
	err := c.getJSON(ctx, "DATOS_SERIE", series, q.values(), &out)
	return out, err
}

// TableCSV opens the CSV the INE publishes for a table: semicolon separated,
// comma as decimal mark, UTF-8 with a BOM. The caller closes the body.
func (c *Client) TableCSV(ctx context.Context, table string) (io.ReadCloser, error) {
	u := fmt.Sprintf("%s/%s/csv_bdsc/%s.csv", c.CSVBaseURL, strings.ToLower(c.Lang), url.PathEscape(table))
	resp, err := c.do(ctx, u)
	if err != nil {
		return nil, err
	}
	if strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
		resp.Body.Close()
		return nil, fmt.Errorf("%w: table %s", ErrNotFound, table)
	}
	return resp.Body, nil
}

// RawJSON returns the untouched JSON body of any Tempus3 function, for the
// calls this package does not model.
func (c *Client) RawJSON(ctx context.Context, function, input string, params url.Values) (json.RawMessage, error) {
	var out json.RawMessage
	err := c.getJSON(ctx, function, input, params, &out)
	return out, err
}

func (c *Client) getJSON(ctx context.Context, function, input string, params url.Values, dst any) error {
	u := c.BaseURL + "/" + c.Lang + "/" + function
	if input != "" {
		u += "/" + url.PathEscape(input)
	}
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	resp, err := c.do(ctx, u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("ine: read %s: %w", function, err)
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" || trimmed == "null" {
		return fmt.Errorf("%w: %s %s", ErrNotFound, function, input)
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("ine: decode %s %s: %w", function, input, err)
	}
	return nil
}

// do performs a GET with retries. An HTML answer with 404 or 500 is the API's
// way of saying the input does not exist, so it is not retried.
func (c *Client) do(ctx context.Context, u string) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			wait := time.Duration(1<<(attempt-1)) * 500 * time.Millisecond
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", c.UserAgent)
		resp, err := c.HTTP.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = err
			continue
		}
		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}
		html := strings.Contains(resp.Header.Get("Content-Type"), "text/html")
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		// The CSV download answers 204 for an unknown table.
		if resp.StatusCode == http.StatusNoContent ||
			html && (resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusInternalServerError) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, u)
		}
		lastErr = fmt.Errorf("ine: %s: status %d", u, resp.StatusCode)
		if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500 {
			return nil, lastErr
		}
	}
	return nil, lastErr
}
