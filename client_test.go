package ine

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

const seriesFixture = `[{"COD":"IPC290751", "Nombre":"Nacional. Índice general. Índice. ", "T3_Unidad":"Índice", "T3_Escala":" ",
 "MetaData":[{"Id":16473, "T3_Variable":"Totales Territoriales", "Nombre":"Nacional", "Codigo":"00"}],
 "Data":[{"Fecha":"2026-10-01T00:00:00.000+02:00", "T3_TipoDato":"Definitivo", "T3_Periodo":"M10", "Anyo":2026, "Valor":105.651},
         {"Fecha":"2026-09-01T00:00:00.000+02:00", "T3_TipoDato":"Provisional", "T3_Periodo":"M09", "Anyo":2026, "Valor":null}]}]`

func testClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New()
	c.BaseURL = srv.URL
	c.WebBaseURL = srv.URL
	c.Retries = 2
	return c
}

func TestTableDataDecodesFriendlyForm(t *testing.T) {
	var gotPath, gotQuery string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		w.Write([]byte(seriesFixture))
	})
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	got, err := c.TableData(context.Background(), "24077", Query{Last: 2, From: from})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/ES/DATOS_TABLA/24077" {
		t.Errorf("path = %q", gotPath)
	}
	if gotQuery != "date=20240101%3A&nult=2&tip=AM" {
		t.Errorf("query = %q", gotQuery)
	}
	if len(got) != 1 {
		t.Fatalf("series = %d", len(got))
	}
	s := got[0]
	if s.Code != "IPC290751" || s.Name != "Nacional. Índice general. Índice" || s.Unit != "Índice" || s.Scale != "" {
		t.Errorf("series header = %+v", s)
	}
	if len(s.Variables) != 1 || s.Variables[0].Variable != "Totales Territoriales" {
		t.Errorf("variables = %+v", s.Variables)
	}
	if len(s.Data) != 2 || s.Data[0].Value == nil || *s.Data[0].Value != 105.651 || s.Data[1].Value != nil {
		t.Errorf("data = %+v", s.Data)
	}
	if s.Data[0].Date.UTC() != time.Date(2026, 9, 30, 22, 0, 0, 0, time.UTC) {
		t.Errorf("date = %v", s.Data[0].Date)
	}
}

func TestTablesParsesLastModified(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"Id":24077,"Nombre":"Índice general","Codigo":"NAC","FK_Periodicidad":1,"Anyo_Periodo_ini":"1961","FechaRef_fin":"null","Ultima_Modificacion":"Fri Nov 13 09:00:00 CET 2026"}]`))
	})
	got, err := c.Tables(context.Background(), "IPC")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 11, 13, 8, 0, 0, 0, time.UTC)
	if len(got) != 1 || got[0].ID != 24077 || !got[0].LastModified.Equal(want) {
		t.Errorf("tables = %+v", got)
	}
}

func TestHTMLErrorPageIsNotFound(t *testing.T) {
	var calls atomic.Int32
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("<html>500</html>"))
	})
	_, err := c.Operation(context.Background(), "ZZZZ")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("not found was retried %d times", calls.Load())
	}
}

func TestTransientErrorsAreRetried(t *testing.T) {
	var calls atomic.Int32
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(`{"Id":25,"Codigo":"IPC","Nombre":"IPC"}`))
	})
	op, err := c.Operation(context.Background(), "IPC")
	if err != nil {
		t.Fatal(err)
	}
	if op.ID != 25 || op.Code != "IPC" || calls.Load() != 2 {
		t.Errorf("op = %+v after %d calls", op, calls.Load())
	}
}

func TestTableCSVPath(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/jaxi/files/tpx/es/csv_bdsc/50902.csv" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Write([]byte("a;b\n1;2\n"))
	})
	body, err := c.TableCSV(context.Background(), "50902")
	if err != nil {
		t.Fatal(err)
	}
	body.Close()
}

func TestTableCSVUnknownIsNotFound(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	if _, err := c.TableCSV(context.Background(), "1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestVolumeRestrictionIsTooLarge(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status" : "No puede mostrarse por restricciones de volumen"}`))
	})
	_, err := c.TableData(context.Background(), "80107", Query{Last: 1})
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v", err)
	}
}
