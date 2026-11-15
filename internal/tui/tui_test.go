package tui

import (
	"testing"

	"github.com/gverdugo-dev/ine"
)

func TestWordFilterNeedsEveryWordIgnoringAccents(t *testing.T) {
	targets := []string{
		fold("IPH Índice de Precios Hoteleros"),
		fold("IPC Índice de Precios de Consumo (IPC)"),
		fold("ECE Encuesta de Consumos Energéticos"),
	}
	got := wordFilter("Precios CONSUMO", targets)
	if len(got) != 1 || got[0].Index != 1 {
		t.Fatalf("ranks = %+v", got)
	}
	if got := wordFilter("indice", targets); len(got) != 2 {
		t.Fatalf("accent folding: ranks = %+v", got)
	}
}

func TestSelectionKeepsOrderAndToggles(t *testing.T) {
	s := &selection{byID: map[int]ine.Table{}}
	s.toggle(ine.Table{ID: 3})
	s.toggle(ine.Table{ID: 1})
	s.toggle(ine.Table{ID: 2})
	s.toggle(ine.Table{ID: 1})
	if len(s.order) != 2 || s.order[0] != 3 || s.order[1] != 2 || s.has(1) {
		t.Fatalf("order = %v", s.order)
	}
}
