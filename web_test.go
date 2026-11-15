package ine

import (
	"context"
	"net/http"
	"testing"
)

const resultsPage = `<a href="/dynt3/inebase/index.htm?padre=10&amp;capsel=11">Ganancia</a>
<a href="/jaxiT3/Tabla.htm?t=500&amp;L=0">Tabla suelta</a>`

const indexPage = `<h1 class="inebase_titulo" id="c_10"> Ganancia</h1>
<a id="c_11" role="treeitem" class="desplegar" href="?padre=10&amp;capsel=11" aria-expanded="true">Ganancia anual</a>
<a href="/jaxi/Tabla.htm?tpx=70562&amp;L=0" role="treeitem" class="additional titulo t2">Sexo y   grupo de edad</a>
<a id="c_12" role="treeitem" class="desplegar" href="?padre=10&amp;capsel=12" aria-expanded="false">Ganancia mensual</a>`

const chapterPage = `<a id="c_12" role="treeitem" class="desplegar" href="?padre=10&amp;capsel=12">Ganancia mensual</a>
<a href="/jaxi/Tabla.htm?tpx=70570&amp;L=0" class="titulo">Tipo de jornada &amp; sexo</a>
<a href="/jaxi/Tabla.htm?tpx=70562&amp;L=0" class="titulo">Sexo y grupo de edad</a>`

func TestWebTablesCrawlsChapters(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path + "?" + r.URL.RawQuery {
		case "/dyngs/INEbase/operacion.htm?c=Estadistica_C&cid=1&menu=resultados":
			w.Write([]byte(resultsPage))
		case "/dynt3/inebase/index.htm?padre=10&capsel=11":
			w.Write([]byte(indexPage))
		case "/dynt3/inebase/index.htm?padre=10&capsel=12":
			w.Write([]byte(chapterPage))
		default:
			t.Errorf("unexpected request %s", r.URL)
			w.WriteHeader(http.StatusNoContent)
		}
	})
	got, err := c.WebTables(context.Background(), Operation{Code: "SPD", Name: "Salarios", URL: "/dyngs/INEbase/operacion.htm?c=Estadistica_C&cid=1"})
	if err != nil {
		t.Fatal(err)
	}
	want := []Table{
		{ID: 500, Name: "Tabla suelta", Group: "Salarios", FromWeb: true},
		{ID: 70562, Name: "Sexo y grupo de edad", Group: "Ganancia anual", FromWeb: true},
		{ID: 70570, Name: "Tipo de jornada & sexo", Group: "Ganancia mensual", FromWeb: true},
	}
	if len(got) != len(want) {
		t.Fatalf("tables = %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("table %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestAllTablesFallsBackToWebOnlyWhenAPIIsEmpty(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ES/TABLAS_OPERACION/7":
			w.Write([]byte(`[]`))
		case "/ES/TABLAS_OPERACION/8":
			w.Write([]byte(`[{"Id":1,"Nombre":"API"}]`))
		case "/op":
			w.Write([]byte(`<a href="/jaxiT3/Tabla.htm?t=9">Web</a>`))
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})
	ctx := context.Background()
	web, err := c.AllTables(ctx, Operation{ID: 7, URL: "/op"})
	if err != nil || len(web) != 1 || web[0].ID != 9 || !web[0].FromWeb {
		t.Fatalf("web fallback = %+v, %v", web, err)
	}
	api, err := c.AllTables(ctx, Operation{ID: 8, URL: "/op"})
	if err != nil || len(api) != 1 || api[0].ID != 1 || api[0].FromWeb {
		t.Fatalf("api = %+v, %v", api, err)
	}
	none, err := c.AllTables(ctx, Operation{ID: 7})
	if err != nil || len(none) != 0 {
		t.Fatalf("no web page = %+v, %v", none, err)
	}
}
