package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/alex-irvine/lazydiff/agent"
	"github.com/alex-irvine/lazydiff/config"
	"github.com/alex-irvine/lazydiff/delta"
	"github.com/alex-irvine/lazydiff/diff"
	"github.com/alex-irvine/lazydiff/git"
	"github.com/alex-irvine/lazydiff/prompt"
	tea "github.com/charmbracelet/bubbletea"
)

type Focus int

const (
	FocusTree Focus = iota
	FocusDiff
	FocusAnalysis
)

type AnalysisTab int

const (
	DetailTab AnalysisTab = iota
	OverallTab
	RequestLogTab
)

type SnapshotLoader interface {
	Snapshot(context.Context, git.Mode) (git.Snapshot, error)
}

type analysisResult struct {
	Text   string
	Stale  bool
	Active bool
	Error  error
}

type Model struct {
	repo      git.Repository
	cfg       config.Config
	loader    SnapshotLoader
	renderer  Renderer
	runner    agent.Runner
	templates prompt.Templates

	snapshot       git.Snapshot
	haveSnap       bool
	mode           git.Mode
	tree           *TreeModel
	layout         Layout
	termW          int
	termH          int
	focus          Focus
	activeTab      AnalysisTab
	diffScroll     int
	analysisScroll int
	diffText       string
	diffWarn       error
	diffStyled     bool
	results        map[string]*analysisResult
	requests       map[string]context.CancelFunc
	requestSeq     uint64
	status         string
	showHelp       bool
	send           func(tea.Msg)
}

type snapshotMsg struct{ Snapshot git.Snapshot }
type snapshotErrorMsg struct{ Err error }
type deltaMsg struct {
	Content string
	Styled  bool
	Warning error
}
type analysisOutputMsg struct {
	Key  string
	Text string
}
type analysisDoneMsg struct {
	Key   string
	Seq   uint64
	Text  string
	Error error
}
type refreshMsg struct{}
type refreshTickMsg struct{}

// Renderer is the small dependency required by Model; delta.Renderer satisfies it.
type Renderer interface {
	Render(context.Context, string, int) delta.Result
}

func NewModel(repo git.Repository, cfg config.Config, loader SnapshotLoader, renderer Renderer, runner agent.Runner, templates prompt.Templates) Model {
	return Model{
		repo: repo, cfg: cfg, loader: loader, renderer: renderer, runner: runner, templates: templates,
		mode: git.WorkingTree, tree: NewTree(nil), focus: FocusTree, activeTab: DetailTab,
		results: make(map[string]*analysisResult), requests: make(map[string]context.CancelFunc),
		status: "loading repository",
	}
}

type TeaModel struct {
	model Model
}

func NewTeaModel(model Model) *TeaModel { return &TeaModel{model: model} }

func (t *TeaModel) SetSend(send func(tea.Msg)) { t.model.SetSend(send) }

func (t *TeaModel) Init() tea.Cmd { return t.model.Init() }

func (t *TeaModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := t.model.Update(msg)
	t.model = updated
	return t, cmd
}

func (t *TeaModel) View() string { return t.model.View() }

func (m *Model) SetSend(send func(tea.Msg)) { m.send = send }

func (m Model) Init() tea.Cmd { return m.refreshCmd() }

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch message := msg.(type) {
	case tea.WindowSizeMsg:
		m.termW, m.termH = message.Width, message.Height
		m.layout = ComputeLayout(m.termW, m.termH)
		return m, m.refreshCmd()
	case refreshMsg, refreshTickMsg:
		return m, m.refreshCmd()
	case snapshotMsg:
		changed := m.applySnapshot(message.Snapshot)
		if changed {
			return m, tea.Batch(m.renderSelectedCmd(), tickCmd())
		}
		return m, tickCmd()
	case snapshotErrorMsg:
		m.status = "git error: " + message.Err.Error()
		return m, tickCmd()
	case deltaMsg:
		m.diffText, m.diffStyled, m.diffWarn = message.Content, message.Styled, message.Warning
		if message.Warning != nil {
			m.status = "delta fallback: " + message.Warning.Error()
		} else {
			m.status = "delta active"
		}
		return m, nil
	case analysisOutputMsg:
		result := m.results[message.Key]
		if result == nil {
			result = &analysisResult{Active: true}
			m.results[message.Key] = result
		}
		if result.Text != "" {
			result.Text += "\n"
		}
		result.Text += message.Text
		return m, nil
	case analysisDoneMsg:
		if cancel := m.requests[message.Key]; cancel != nil {
			delete(m.requests, message.Key)
		}
		result := m.results[message.Key]
		if result == nil {
			result = &analysisResult{}
			m.results[message.Key] = result
		}
		if message.Text != "" {
			result.Text = message.Text
		}
		result.Active = false
		result.Error = message.Error
		if message.Error != nil && message.Error != context.Canceled {
			m.status = "agent error: " + message.Error.Error()
		}
		if resultKeySnapshot(message.Key) != m.snapshot.ID {
			result.Stale = true
		}
		return m, nil
	case tea.KeyMsg:
		return m.updateKey(message)
	}
	return m, nil
}

func (m *Model) applySnapshot(snapshot git.Snapshot) bool {
	oldID := m.snapshot.ID
	changed := oldID != snapshot.ID
	m.snapshot, m.haveSnap = snapshot, true
	m.tree.SetFiles(snapshot.Files)
	if changed {
		m.diffScroll = 0
		m.analysisScroll = 0
		for key, result := range m.results {
			if resultKeySnapshot(key) == oldID {
				result.Stale = true
			}
		}
	}
	m.status = fmt.Sprintf("%s · %d files", snapshot.Mode, len(snapshot.Files))
	return changed
}

func (m Model) updateKey(key tea.KeyMsg) (Model, tea.Cmd) {
	switch key.String() {
	case "tab":
		m.focus = (m.focus + 1) % 3
	case "shift+tab":
		m.focus = (m.focus + 2) % 3
	case "up", "k":
		if m.focus == FocusTree {
			m.tree.Move(-1)
			m.diffScroll = 0
			return m, m.renderSelectedCmd()
		}
		if m.focus == FocusDiff && m.diffScroll > 0 {
			m.diffScroll--
		}
		if m.focus == FocusAnalysis && m.analysisScroll > 0 {
			m.analysisScroll--
		}
	case "down", "j":
		if m.focus == FocusTree {
			m.tree.Move(1)
			m.diffScroll = 0
			return m, m.renderSelectedCmd()
		}
		if m.focus == FocusDiff {
			m.diffScroll++
		}
		if m.focus == FocusAnalysis {
			m.analysisScroll++
		}
	case " ":
		if m.focus == FocusTree {
			m.tree.Toggle()
			m.diffScroll = 0
			return m, m.renderSelectedCmd()
		}
	case "h":
		if m.focus == FocusTree {
			m.tree.CollapseOrParent()
			m.diffScroll = 0
			return m, m.renderSelectedCmd()
		}
	case "l":
		if m.focus == FocusTree {
			m.tree.ExpandOrDescend()
			m.diffScroll = 0
			return m, m.renderSelectedCmd()
		}
	case "a":
		m.activeTab = OverallTab
		return m, m.startAnalysis(false)
	case "A":
		m.activeTab = DetailTab
		return m, m.startAnalysis(true)
	case "1":
		m.activeTab = DetailTab
	case "2":
		m.activeTab = OverallTab
	case "3":
		m.activeTab = RequestLogTab
	case "c":
		m.cancelActive()
	case "m":
		m.mode = m.mode.Next()
		return m, m.refreshCmd()
	case "r":
		return m, m.refreshCmd()
	case "g":
		if m.focus == FocusDiff {
			m.diffScroll = 0
		}
		if m.focus == FocusAnalysis {
			m.analysisScroll = 0
		}
	case "G":
		if m.focus == FocusDiff {
			m.diffScroll = max(0, len(delta.Lines(m.diffText))-m.layout.Code.H+3)
		}
		if m.focus == FocusAnalysis {
			m.analysisScroll = max(0, len(m.analysisLines())-m.layout.Agent.H+3)
		}
	case "?":
		m.showHelp = !m.showHelp
	case "q", "ctrl+c":
		m.cancelActive()
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) refreshCmd() tea.Cmd {
	loader, mode := m.loader, m.mode
	return func() tea.Msg {
		if loader == nil {
			return snapshotErrorMsg{Err: fmt.Errorf("snapshot loader unavailable")}
		}
		snapshot, err := loader.Snapshot(context.Background(), mode)
		if err != nil {
			return snapshotErrorMsg{Err: err}
		}
		return snapshotMsg{Snapshot: snapshot}
	}
}

func (m Model) renderSelectedCmd() tea.Cmd {
	if !m.haveSnap || m.renderer == nil {
		return nil
	}
	file, hunk, ok := m.tree.Selected()
	if !ok {
		return nil
	}
	raw := m.snapshot.RawDiff
	if hunk != nil {
		raw = hunk.RawDiff()
	} else if file.RawDiff() != "" {
		raw = file.RawDiff()
	}
	width := m.layout.Code.W - 2
	renderer := m.renderer
	return func() tea.Msg {
		result := renderer.Render(context.Background(), raw, width)
		return deltaMsg{Content: result.Content, Styled: result.Styled, Warning: result.Warning}
	}
}

func (m Model) startAnalysis(detail bool) tea.Cmd {
	if !m.haveSnap || m.runner == nil {
		return nil
	}
	file, hunk, ok := m.tree.Selected()
	if !ok {
		return nil
	}
	key := resultKey(m.snapshot.ID, detail, file.ID, hunk)
	ctx, cancel := context.WithCancel(context.Background())
	if old := m.requests[key]; old != nil {
		old()
	}
	m.requests[key] = cancel
	m.requestSeq++
	seq := m.requestSeq
	result := m.results[key]
	if result == nil {
		result = &analysisResult{}
		m.results[key] = result
	}
	result.Text, result.Active, result.Error, result.Stale = "", true, nil, false
	ctxPrompt := prompt.Context{Repository: m.repo.Root, Mode: m.snapshot.Mode.String(), OverallDiff: m.snapshot.RawDiff, Selection: file.DisplayPath(), SelectedDiff: file.RawDiff()}
	if hunk != nil {
		ctxPrompt.Selection += " " + hunk.Header
		ctxPrompt.SelectedDiff = hunk.RawDiff()
	}
	var rendered string
	var err error
	if detail {
		rendered, err = m.templates.RenderDetail(ctxPrompt)
	} else {
		rendered, err = m.templates.RenderOverall(ctxPrompt)
	}
	if err != nil {
		result.Active, result.Error = false, err
		return nil
	}
	runner, send, snapshotID := m.runner, m.send, m.snapshot.ID
	return func() tea.Msg {
		var output strings.Builder
		err := runner.Run(ctx, agent.Request{RepoRoot: m.repo.Root, Prompt: rendered}, func(event agent.Event) {
			if event.Kind == agent.Output {
				if output.Len() > 0 {
					output.WriteByte('\n')
				}
				output.WriteString(event.Text)
				if send != nil {
					send(analysisOutputMsg{Key: key, Text: event.Text})
				}
			} else if send != nil {
				send(analysisOutputMsg{Key: requestLogKey(snapshotID), Text: event.Text})
			}
		})
		return analysisDoneMsg{Key: key, Seq: seq, Text: output.String(), Error: err}
	}
}

func (m *Model) cancelActive() {
	for key, cancel := range m.requests {
		cancel()
		if result := m.results[key]; result != nil {
			result.Active = false
			result.Error = context.Canceled
		}
	}
	m.requests = make(map[string]context.CancelFunc)
}

func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return refreshTickMsg{} })
}

func resultKey(snapshot string, detail bool, fileID string, hunk *diff.Hunk) string {
	tab := "overall"
	if detail {
		tab = "detail"
	}
	target := fileID
	if hunk != nil {
		target += ":" + hunk.ID
	}
	return tab + ":" + snapshot + ":" + target
}

func resultKeySnapshot(key string) string {
	parts := strings.SplitN(key, ":", 3)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

func requestLogKey(snapshot string) string { return "request:" + snapshot + ":log" }
