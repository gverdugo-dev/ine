// Package tui is the interactive face of the ine CLI: browse the operations,
// pick tables from one or several of them and download them all at once.
package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/gverdugo-dev/ine"
	"github.com/gverdugo-dev/ine/fetch"
)

// Options configures the TUI.
type Options struct {
	OutDir   string
	Interval time.Duration
	Jobs     int
}

// Run starts the TUI and blocks until the user quits.
func Run(ctx context.Context, c *ine.Client, opts Options) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	m := newModel(ctx, cancel, c, opts)
	_, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx)).Run()
	if err == tea.ErrProgramKilled && ctx.Err() != nil {
		return nil
	}
	return err
}

type screen int

const (
	screenOps screen = iota
	screenTables
	screenFetch
)

var formatChoices = [][]fetch.Format{{fetch.JSON}, {fetch.CSV}, {fetch.JSON, fetch.CSV}}

// lastChoices are the period windows the user cycles through; 0 means all.
var lastChoices = []int{0, 1, 12, 24, 60}

type selection struct {
	byID  map[int]ine.Table
	order []int
}

func (s *selection) has(id int) bool { _, ok := s.byID[id]; return ok }

func (s *selection) toggle(t ine.Table) {
	if s.has(t.ID) {
		delete(s.byID, t.ID)
		for i, id := range s.order {
			if id == t.ID {
				s.order = append(s.order[:i], s.order[i+1:]...)
				break
			}
		}
		return
	}
	s.byID[t.ID] = t
	s.order = append(s.order, t.ID)
}

type model struct {
	ctx    context.Context
	cancel context.CancelFunc
	client *ine.Client
	opts   Options

	screen        screen
	width, height int
	spinner       spinner.Model
	loading       string
	err           error

	ops     list.Model
	tables  list.Model
	current ine.Operation
	sel     *selection

	formatIdx, lastIdx int

	bar      progress.Model
	events   chan tea.Msg
	total    int
	results  []fetch.Result
	fetching bool
	finished bool
	fetchErr error
}

type opItem struct{ op ine.Operation }

func (i opItem) Title() string       { return i.op.Name }
func (i opItem) Description() string { return i.op.Code + " · id " + strconv.Itoa(i.op.ID) }
func (i opItem) FilterValue() string { return fold(i.op.Code + " " + i.op.Name) }

type tableItem struct {
	t   ine.Table
	sel *selection
}

func (i tableItem) Title() string {
	mark := checkOff
	if i.sel.has(i.t.ID) {
		mark = checkOn
	}
	return mark + " " + i.t.Name
}

func (i tableItem) Description() string {
	d := "  tabla " + strconv.Itoa(i.t.ID)
	if !i.t.LastModified.IsZero() {
		d += " · actualizada " + i.t.LastModified.Format("02/01/2006")
	}
	return d
}

func (i tableItem) FilterValue() string { return fold(strconv.Itoa(i.t.ID) + " " + i.t.Name) }

type (
	opsLoadedMsg struct {
		ops []ine.Operation
		err error
	}
	tablesLoadedMsg struct {
		op     ine.Operation
		tables []ine.Table
		err    error
	}
	fetchResultMsg fetch.Result
	fetchDoneMsg   struct{ err error }
)

var (
	keyToggle   = key.NewBinding(key.WithKeys(" "), key.WithHelp("espacio", "marcar"))
	keyAll      = key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "marcar visibles"))
	keyFormat   = key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "formato"))
	keyLast     = key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "periodos"))
	keyDownload = key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "descargar"))
	keyOpen     = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "ver tablas"))
	keyBack     = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "volver"))
)

func newList(title string, extra ...key.Binding) list.Model {
	d := list.NewDefaultDelegate()
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.Foreground(burgundy).BorderForeground(burgundy)
	d.Styles.SelectedDesc = d.Styles.SelectedDesc.Foreground(teal).BorderForeground(burgundy)
	d.Styles.NormalDesc = d.Styles.NormalDesc.Foreground(muted)
	// The filter runs on an accent-folded value, so its match positions do not
	// line up with the title; highlighting them would underline the wrong runes.
	d.Styles.FilterMatch = lipgloss.NewStyle()
	l := list.New(nil, d, 0, 0)
	l.Title = title
	l.Styles.Title = listTitle
	l.Styles.FilterPrompt = l.Styles.FilterPrompt.Foreground(burgundy)
	l.Styles.FilterCursor = l.Styles.FilterCursor.Foreground(burgundy)
	l.Styles.StatusBar = l.Styles.StatusBar.Foreground(muted)
	l.FilterInput.Prompt = "Buscar: "
	l.Filter = wordFilter
	l.SetStatusBarItemName("elemento", "elementos")
	l.KeyMap.Quit = key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "salir"))
	l.AdditionalShortHelpKeys = func() []key.Binding { return extra }
	l.AdditionalFullHelpKeys = func() []key.Binding { return extra }
	return l
}

func newModel(ctx context.Context, cancel context.CancelFunc, c *ine.Client, opts Options) model {
	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = lipgloss.NewStyle().Foreground(burgundy)
	bar := progress.New(progress.WithGradient("#457E76", "#9E1834"), progress.WithoutPercentage())
	return model{
		ctx: ctx, cancel: cancel, client: c, opts: opts,
		spinner: sp,
		loading: "Cargando operaciones",
		ops:     newList("Operaciones estadísticas", keyOpen),
		tables:  newList("Tablas", keyToggle, keyAll, keyFormat, keyLast, keyDownload, keyBack),
		sel:     &selection{byID: map[int]ine.Table{}},
		lastIdx: 2,
		bar:     bar,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.loadOps())
}

func (m model) loadOps() tea.Cmd {
	return func() tea.Msg {
		ops, err := m.client.Operations(m.ctx)
		return opsLoadedMsg{ops, err}
	}
}

func (m model) loadTables(op ine.Operation) tea.Cmd {
	return func() tea.Msg {
		ts, err := m.client.Tables(m.ctx, op.Code)
		return tablesLoadedMsg{op, ts, err}
	}
}

const headerHeight = 2

func (m *model) resize() {
	h := m.height - headerHeight
	m.ops.SetSize(m.width, h)
	m.tables.SetSize(m.width, h)
	m.bar.Width = min(m.width-8, 72)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.cancel()
			return m, tea.Quit
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case opsLoadedMsg:
		m.loading = ""
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		items := make([]list.Item, len(msg.ops))
		for i, o := range msg.ops {
			items[i] = opItem{o}
		}
		return m, m.ops.SetItems(items)

	case tablesLoadedMsg:
		m.loading = ""
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.current = msg.op
		items := make([]list.Item, len(msg.tables))
		for i, t := range msg.tables {
			items[i] = tableItem{t, m.sel}
		}
		m.tables.Title = msg.op.Code + " · " + msg.op.Name
		m.tables.ResetFilter()
		m.tables.Select(0)
		m.screen = screenTables
		return m, m.tables.SetItems(items)

	case fetchResultMsg:
		m.results = append(m.results, fetch.Result(msg))
		return m, tea.Batch(m.bar.SetPercent(float64(len(m.results))/float64(m.total)), m.next())

	case fetchDoneMsg:
		m.fetching, m.finished, m.fetchErr = false, true, msg.err
		return m, m.bar.SetPercent(1)

	case progress.FrameMsg:
		pm, cmd := m.bar.Update(msg)
		m.bar = pm.(progress.Model)
		return m, cmd
	}

	if m.err != nil {
		if k, ok := msg.(tea.KeyMsg); ok {
			if k.String() == "q" {
				return m, tea.Quit
			}
			m.err = nil
		}
		return m, nil
	}

	switch m.screen {
	case screenOps:
		return m.updateOps(msg)
	case screenTables:
		return m.updateTables(msg)
	default:
		return m.updateFetch(msg)
	}
}

func (m model) updateOps(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && m.ops.FilterState() != list.Filtering && m.loading == "" {
		switch {
		case key.Matches(k, keyOpen):
			if it, ok := m.ops.SelectedItem().(opItem); ok {
				m.loading = "Cargando tablas de " + it.op.Code
				return m, tea.Batch(m.spinner.Tick, m.loadTables(it.op))
			}
		case key.Matches(k, keyDownload) && len(m.sel.order) > 0:
			return m.startFetch()
		}
	}
	var cmd tea.Cmd
	m.ops, cmd = m.ops.Update(msg)
	return m, cmd
}

func (m model) updateTables(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && m.tables.FilterState() != list.Filtering {
		switch {
		case key.Matches(k, keyBack) && m.tables.FilterState() == list.Unfiltered:
			m.screen = screenOps
			return m, nil
		case key.Matches(k, keyToggle):
			if it, ok := m.tables.SelectedItem().(tableItem); ok {
				m.sel.toggle(it.t)
			}
			return m, nil
		case key.Matches(k, keyAll):
			visible := m.tables.VisibleItems()
			allOn := true
			for _, v := range visible {
				if !m.sel.has(v.(tableItem).t.ID) {
					allOn = false
					break
				}
			}
			for _, v := range visible {
				t := v.(tableItem).t
				if m.sel.has(t.ID) == allOn {
					m.sel.toggle(t)
				}
			}
			return m, nil
		case key.Matches(k, keyFormat):
			m.formatIdx = (m.formatIdx + 1) % len(formatChoices)
			return m, nil
		case key.Matches(k, keyLast):
			m.lastIdx = (m.lastIdx + 1) % len(lastChoices)
			return m, nil
		case key.Matches(k, keyDownload):
			if len(m.sel.order) == 0 {
				return m, m.tables.NewStatusMessage(statusMsgSty.Render("Marca alguna tabla con espacio"))
			}
			return m.startFetch()
		}
	}
	var cmd tea.Cmd
	m.tables, cmd = m.tables.Update(msg)
	return m, cmd
}

func (m model) updateFetch(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok || m.fetching {
		return m, nil
	}
	switch k.String() {
	case "q":
		return m, tea.Quit
	case "esc", "enter":
		m.screen = screenTables
		if m.current.Code == "" {
			m.screen = screenOps
		}
		m.finished = false
	case "c":
		m.sel = &selection{byID: map[int]ine.Table{}}
		items := m.tables.Items()
		for i, it := range items {
			ti := it.(tableItem)
			ti.sel = m.sel
			items[i] = ti
		}
		m.tables.SetItems(items)
		m.screen = screenTables
		if m.current.Code == "" {
			m.screen = screenOps
		}
		m.finished = false
	}
	return m, nil
}

func (m model) startFetch() (tea.Model, tea.Cmd) {
	tables := make([]string, len(m.sel.order))
	for i, id := range m.sel.order {
		tables[i] = strconv.Itoa(id)
	}
	formats := formatChoices[m.formatIdx]
	m.screen = screenFetch
	m.fetching, m.finished, m.fetchErr = true, false, nil
	m.results = nil
	m.total = len(tables) * len(formats)
	m.events = make(chan tea.Msg, m.total+1)
	events := m.events
	opts := fetch.Options{
		Dir: m.opts.OutDir, Formats: formats, Query: ine.Query{Last: lastChoices[m.lastIdx]},
		Concurrency: m.opts.Jobs, Interval: m.opts.Interval,
	}
	go func() {
		results, err := fetch.Run(m.ctx, m.client, tables, opts, func(r fetch.Result) { events <- fetchResultMsg(r) })
		if err == nil {
			err = fetch.WriteManifest(opts.Dir, m.client.Lang, results)
		}
		events <- fetchDoneMsg{err}
	}()
	return m, tea.Batch(m.bar.SetPercent(0), m.next())
}

// next waits for the following event of the running download.
func (m model) next() tea.Cmd {
	events := m.events
	return func() tea.Msg { return <-events }
}

func (m model) View() string {
	if m.width == 0 {
		return ""
	}
	header := m.header()
	var body string
	switch {
	case m.err != nil:
		body = panelStyle.Render(failStyle.Render("Error") + "\n\n" + m.err.Error() + "\n\n" + mutedStyle.Render("cualquier tecla para seguir · q salir"))
		body = lipgloss.NewStyle().Padding(1, 2).Render(body)
	case m.loading != "":
		body = lipgloss.NewStyle().Padding(1, 2).Render(m.spinner.View() + " " + m.loading + "…")
	case m.screen == screenOps:
		body = m.ops.View()
	case m.screen == screenTables:
		body = m.tables.View()
	default:
		body = m.fetchView()
	}
	return header + "\n" + body
}

func (m model) header() string {
	brand := brandStyle.Render("INE")
	// The INE CSV is always the whole table, so the period window only applies
	// when JSON is among the formats.
	periods := lastLabel(lastChoices[m.lastIdx])
	if formatChoices[m.formatIdx][0] == fetch.CSV {
		periods = "tabla completa"
	}
	settings := fmt.Sprintf("%d marcadas · %s · %s · → %s",
		len(m.sel.order), formatLabel(formatChoices[m.formatIdx]), periods, m.opts.OutDir)
	name := " Instituto Nacional de Estadística"
	gap := m.width - lipgloss.Width(brand) - lipgloss.Width(name) - lipgloss.Width(settings) - 2
	if gap < 1 {
		name, gap = "", max(1, m.width-lipgloss.Width(brand)-lipgloss.Width(settings)-2)
	}
	bar := barStyle.Width(m.width - lipgloss.Width(brand)).Render(name + strings.Repeat(" ", gap) + settings)
	return lipgloss.JoinHorizontal(lipgloss.Top, brand, bar)
}

func (m model) fetchView() string {
	var b strings.Builder
	title := "Descargando"
	if m.finished {
		title = "Descarga terminada"
	}
	fmt.Fprintf(&b, "%s  %s\n\n", titleStyle.Render(title), mutedStyle.Render(fmt.Sprintf("%d/%d ficheros", len(m.results), m.total)))
	b.WriteString(m.bar.View() + "\n\n")

	rows := max(3, m.height-headerHeight-10)
	start := max(0, len(m.results)-rows)
	for _, r := range m.results[start:] {
		mark := okStyle.Render("✓")
		detail := mutedStyle.Render(humanBytes(r.Bytes))
		switch r.Status {
		case fetch.Skipped:
			mark, detail = mutedStyle.Render("·"), mutedStyle.Render("ya existía")
		case fetch.Failed:
			mark, detail = failStyle.Render("✗"), failStyle.Render(r.Error)
		}
		name := r.Table + "." + string(r.Format)
		if t, ok := m.sel.byID[atoi(r.Table)]; ok {
			name += "  " + mutedStyle.Render(truncate(t.Name, m.width-40))
		}
		fmt.Fprintf(&b, " %s %s  %s\n", mark, name, detail)
	}

	if m.finished {
		failed := fetch.Failures(m.results)
		summary := okStyle.Render(fmt.Sprintf("%d correctas", len(m.results)-failed))
		if failed > 0 {
			summary += mutedStyle.Render(" · ") + failStyle.Render(fmt.Sprintf("%d fallidas", failed))
		}
		if m.fetchErr != nil {
			summary += mutedStyle.Render(" · ") + failStyle.Render(m.fetchErr.Error())
		}
		fmt.Fprintf(&b, "\n%s  %s\n\n%s", summary, mutedStyle.Render("en "+m.opts.OutDir),
			mutedStyle.Render("enter volver · c volver y desmarcar todo · q salir"))
	}
	return lipgloss.NewStyle().Padding(1, 2).Render(b.String())
}

func formatLabel(fs []fetch.Format) string {
	parts := make([]string, len(fs))
	for i, f := range fs {
		parts[i] = string(f)
	}
	return strings.Join(parts, "+")
}

func lastLabel(n int) string {
	switch n {
	case 0:
		return "todos los periodos"
	case 1:
		return "último periodo"
	default:
		return "últimos " + strconv.Itoa(n)
	}
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func truncate(s string, n int) string {
	if n < 8 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// wordFilter keeps the targets that contain every word of the term, ignoring
// case and accents, in their original order. The default fuzzy filter ranks
// long names badly for multi-word searches such as "precios de consumo".
func wordFilter(term string, targets []string) []list.Rank {
	words := strings.Fields(fold(term))
	var out []list.Rank
	for i, t := range targets {
		ok := true
		for _, w := range words {
			if !strings.Contains(t, w) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, list.Rank{Index: i})
		}
	}
	return out
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

// fold lowercases and drops Spanish accents so a search for "indice" finds
// "Índice".
func fold(s string) string {
	return strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n").Replace(strings.ToLower(s))
}
