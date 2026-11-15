package ine

import (
	"encoding/json"
	"strings"
	"time"
)

// Operation is a statistical operation of the INE, such as the IPC or the EPA.
type Operation struct {
	ID          int    `json:"id"`
	Code        string `json:"code"`
	IOECode     string `json:"ioeCode,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url,omitempty"`
}

// UnmarshalJSON reads the Tempus3 field names.
func (o *Operation) UnmarshalJSON(b []byte) error {
	var w struct {
		ID          int    `json:"Id"`
		Code        string `json:"Codigo"`
		IOECode     string `json:"Cod_IOE"`
		Name        string `json:"Nombre"`
		Description string `json:"Descripcion"`
		URL         string `json:"Url"`
	}
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	*o = Operation(w)
	return nil
}

// Table is one table of an operation. Its ID is what DATOS_TABLA and the CSV
// download take.
type Table struct {
	ID int `json:"id"`
	// Code is the table group code, not unique (for example "NAC").
	Code          string    `json:"code,omitempty"`
	Name          string    `json:"name"`
	PeriodicityID int       `json:"periodicityId,omitempty"`
	FirstYear     string    `json:"firstYear,omitempty"`
	LastModified  time.Time `json:"lastModified,omitempty"`
}

type wireTable struct {
	ID            int    `json:"Id"`
	Name          string `json:"Nombre"`
	Code          string `json:"Codigo"`
	PeriodicityID int    `json:"FK_Periodicidad"`
	FirstYear     string `json:"Anyo_Periodo_ini"`
	LastModified  string `json:"Ultima_Modificacion"`
}

var madrid = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		return time.FixedZone("CET", 3600)
	}
	return loc
}()

func (w wireTable) table() Table {
	t := Table{ID: w.ID, Code: w.Code, Name: w.Name, PeriodicityID: w.PeriodicityID, FirstYear: w.FirstYear}
	t.LastModified = parseModified(w.LastModified)
	return t
}

// parseModified reads dates like "Fri Nov 13 09:00:00 CET 2026". Parsing in
// Europe/Madrid makes Go resolve the CET and CEST abbreviations.
func parseModified(s string) time.Time {
	t, err := time.ParseInLocation("Mon Jan 2 15:04:05 MST 2006", s, madrid)
	if err != nil {
		return time.Time{}
	}
	return t
}

// Series is a time series with its metadata and its data points.
type Series struct {
	Code      string     `json:"code"`
	Name      string     `json:"name"`
	Unit      string     `json:"unit,omitempty"`
	Scale     string     `json:"scale,omitempty"`
	Variables []Variable `json:"variables,omitempty"`
	Data      []Point    `json:"data"`
}

// Variable is one dimension value that defines a series, such as
// "Totales Territoriales: Nacional".
type Variable struct {
	ID       int    `json:"id"`
	Variable string `json:"variable"`
	Name     string `json:"name"`
	Code     string `json:"code,omitempty"`
}

// Point is one period of a series. Value is nil when the INE publishes no
// figure (secret or not available).
type Point struct {
	Date   time.Time `json:"date"`
	Year   int       `json:"year"`
	Period string    `json:"period"`
	Kind   string    `json:"kind,omitempty"`
	Value  *float64  `json:"value"`
}

// UnmarshalJSON reads the friendly (tip=AM) Tempus3 field names.
func (s *Series) UnmarshalJSON(b []byte) error {
	var w struct {
		Code     string `json:"COD"`
		Name     string `json:"Nombre"`
		Unit     string `json:"T3_Unidad"`
		Scale    string `json:"T3_Escala"`
		MetaData []struct {
			ID       int    `json:"Id"`
			Variable string `json:"T3_Variable"`
			Name     string `json:"Nombre"`
			Code     string `json:"Codigo"`
		} `json:"MetaData"`
		Data []struct {
			Date   string   `json:"Fecha"`
			Kind   string   `json:"T3_TipoDato"`
			Period string   `json:"T3_Periodo"`
			Year   int      `json:"Anyo"`
			Value  *float64 `json:"Valor"`
		} `json:"Data"`
	}
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	*s = Series{
		Code:  w.Code,
		Name:  strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(w.Name), ".")),
		Unit:  strings.TrimSpace(w.Unit),
		Scale: strings.TrimSpace(w.Scale),
		Data:  make([]Point, len(w.Data)),
	}
	for _, m := range w.MetaData {
		s.Variables = append(s.Variables, Variable(m))
	}
	for i, d := range w.Data {
		date, _ := time.Parse("2006-01-02T15:04:05.000-07:00", d.Date)
		s.Data[i] = Point{Date: date, Year: d.Year, Period: d.Period, Kind: d.Kind, Value: d.Value}
	}
	return nil
}
