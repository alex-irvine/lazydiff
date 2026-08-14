package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/alex-irvine/lazydiff/agent"
	"github.com/alex-irvine/lazydiff/config"
	"github.com/alex-irvine/lazydiff/delta"
	"github.com/alex-irvine/lazydiff/diff"
	"github.com/alex-irvine/lazydiff/git"
	"github.com/alex-irvine/lazydiff/pr"
	"github.com/alex-irvine/lazydiff/prompt"
	"github.com/alex-irvine/lazydiff/version"
	tea "github.com/charmbracelet/bubbletea"
)

type TreeMode int

const (
	TreeModeWorktree TreeMode = iota
	TreeModeStaged
	TreeModeBranchSelector
	TreeModeBranchDiff
	TreeModePRSelector
	TreeModePRDiff
)

type branchesLoadedMsg struct {
	Branches  []string
	Current   string
	Default   string
	Worktrees map[string]string
}

type branchesErrorMsg struct{ Err error }

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
	SnapshotBranch(context.Context, string) (git.Snapshot, error)
	SnapshotPR(context.Context, int) (git.Snapshot, error)
}

// Mutator is every git operation the commit/PR flows need beyond reading a
// snapshot (which stays on SnapshotLoader). git.Repository satisfies this
// once its StageFile/StagePatch/Commit/Push/CurrentBranch/RemoteURL methods
// exist — no explicit declaration needed.
type Mutator interface {
	CurrentBranch(context.Context) (string, error)
	DefaultBranch(context.Context) (string, error)
	RemoteURL(context.Context, string) (string, error)
	StageFile(ctx context.Context, oldPath, path string) error
	StagePatch(ctx context.Context, patch string) error
	Commit(ctx context.Context, message string) error
	Push(ctx context.Context, remote, branch string) error
}

// PRReviewer is satisfied by *pr.GitHub and fakes in tests. All calls must
// validate github.com per impl.
type PRReviewer interface {
	ListPRs(context.Context, string) ([]pr.PR, error)
	Approve(context.Context, int, string) error
	RequestChanges(context.Context, int, string) error
	Merge(context.Context, int) error
	Close(context.Context, int) error
	DeleteBranch(context.Context, string) error
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
	mutator   Mutator
	opener    pr.Opener
	dialog    *ActionDialog

	snapshot       git.Snapshot
	haveSnap       bool
	mode           git.Mode
	treeMode       TreeMode
	branchSelector *BranchSelector
	prSelector     *PRSelector
	prReviewer     PRReviewer
	pendingPRKey   rune
	confirm        *ConfirmDialog
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
	showUpdateModal bool
	showUpdating    bool
	updateVersion   string
	updateError     error
	updateManual    bool
	updateStatus    string
	send           func(tea.Msg)
	searchActive   bool
	searchQuery    string
	searchFilter   *regexp.Regexp
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
type updateResultMsg struct {
	HasUpdate bool
	Version   string
	Error     error
	Manual    bool
}
type updatePerformedMsg struct{ Error error }

type commitPrepMsg struct {
	Ticket string
	Prompt string
	Err    error
}

type commitDraftMsg struct {
	Ticket string
	Text   string
	Err    error
}

type commitResultMsg struct{ Err error }

type prPrepMsg struct {
	Ticket string
	Prompt string
	Err    error
}

type prDraftMsg struct {
	Ticket string
	Text   string
	Err    error
}

type prResultMsg struct{ Err error }

type prsLoadedMsg struct {
	PRs []pr.PR
	Err error
}

type prDiffLoadedMsg struct {
	Number   int
	Snapshot git.Snapshot
	Err      error
}

type prActionDoneMsg struct {
	Err     error
	Warning string
}

type editorDoneMsg struct{}
type editorErrorMsg struct{ Err error }
type branchFileReadyMsg struct{ Path string }

// Renderer is the small dependency required by Model; delta.Renderer satisfies it.
type Renderer interface {
	Render(context.Context, string, int) delta.Result
}

func NewModel(repo git.Repository, cfg config.Config, loader SnapshotLoader, renderer Renderer, runner agent.Runner, templates prompt.Templates, mutator Mutator, opener pr.Opener, prReviewer PRReviewer) Model {
	return Model{
		repo: repo, cfg: cfg, loader: loader, renderer: renderer, runner: runner, templates: templates,
		mutator: mutator, opener: opener, prReviewer: prReviewer,
		mode: git.WorkingTree, treeMode: TreeModeWorktree, tree: NewTree(nil), focus: FocusTree, activeTab: DetailTab,
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

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.refreshCmd(), checkUpdateCmd(true))
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok && m.searchActive {
		return m.updateSearchKey(keyMsg)
	}
	if keyMsg, ok := msg.(tea.KeyMsg); ok && m.dialog != nil {
		return m.updateDialogKey(keyMsg)
	}
	if keyMsg, ok := msg.(tea.KeyMsg); ok && m.confirm != nil {
		return m.updateConfirmKey(keyMsg)
	}
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
	case branchesLoadedMsg:
		m.branchSelector = NewBranchSelector(message.Branches, message.Current, message.Default, message.Worktrees)
		return m, nil
	case branchesErrorMsg:
		m.status = "branch list: " + message.Err.Error()
		return m, nil
	case prsLoadedMsg:
		if message.Err != nil {
			if m.prSelector != nil {
				m.prSelector.err = message.Err
			} else {
				m.prSelector = NewPRSelector(nil)
				m.prSelector.err = message.Err
			}
			m.status = "pr list error: " + message.Err.Error()
			return m, nil
		}
		m.prSelector = NewPRSelector(message.PRs)
		return m, nil
	case prDiffLoadedMsg:
		if message.Err != nil {
			m.status = "pr diff error: " + message.Err.Error()
			return m, nil
		}
		if m.prSelector != nil {
			m.prSelector.diffCache[message.Number] = message.Snapshot
		}
		changed := m.applySnapshot(message.Snapshot)
		m.treeMode = TreeModePRDiff
		if changed {
			return m, tea.Batch(m.renderSelectedCmd(), tickCmd())
		}
		return m, tickCmd()
	case prActionDoneMsg:
		if message.Err != nil {
			m.status = "pr action error: " + message.Err.Error()
			// Keep whichever dialog is open, with the error attached, so the
			// user can see what failed and retry or cancel instead of the
			// dialog silently vanishing.
			if m.confirm != nil {
				m.confirm.Err = message.Err
			}
			if m.dialog != nil {
				m.dialog.Err = message.Err
			}
			return m, nil
		}
		m.confirm = nil
		m.dialog = nil
		if message.Warning != "" {
			m.status = message.Warning
		} else {
			m.status = "pr action complete"
		}
		return m, nil
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
	case updateResultMsg:
		m.updateStatus = ""
		if message.Error != nil {
			if message.Manual {
				m.status = "update check failed: " + message.Error.Error()
			}
			return m, nil
		}
		if message.HasUpdate {
			m.updateVersion = message.Version
			m.showUpdateModal = true
		} else if message.Manual {
			m.status = "already up to date (" + message.Version + ")"
		}
	case commitPrepMsg:
		if message.Err != nil {
			m.status = "commit prep error: " + message.Err.Error()
			return m, nil
		}
		m.dialog = NewActionDialog(CommitDialog)
		return m, m.runCommitAgentCmd(message.Prompt, message.Ticket)
	case commitDraftMsg:
		if m.dialog == nil || m.dialog.Kind != CommitDialog {
			return m, nil
		}
		text := message.Text
		if message.Err == nil {
			subject, body := splitSubjectBody(text)
			if message.Ticket != "" {
				body = strings.TrimRight(body, "\n") + "\n\nCU-" + message.Ticket
			}
			text = subject + "\n\n" + body
		}
		m.dialog.SetDraft(text, message.Err)
		return m, nil
	case commitResultMsg:
		if message.Err != nil {
			m.status = "commit failed: " + message.Err.Error()
			return m, nil
		}
		m.dialog = nil
		m.status = "committed"
		return m, m.refreshCmd()
	case prPrepMsg:
		if message.Err != nil {
			m.status = "pr prep error: " + message.Err.Error()
			return m, nil
		}
		m.dialog = NewActionDialog(PRDialog)
		return m, m.runPRAgentCmd(message.Prompt, message.Ticket)
	case prDraftMsg:
		if m.dialog == nil || m.dialog.Kind != PRDialog {
			return m, nil
		}
		text := message.Text
		if message.Err == nil {
			title, body := splitSubjectBody(text)
			title = pr.FormatTitle(message.Ticket, title)
			text = title + "\n\n" + body
		}
		m.dialog.SetDraft(text, message.Err)
		return m, nil
	case prResultMsg:
		if message.Err != nil {
			m.status = "pr failed: " + message.Err.Error()
			return m, nil
		}
		m.dialog = nil
		m.status = "opened pull request in browser"
		return m, nil
	case updatePerformedMsg:
		m.showUpdating = false
		if message.Error != nil {
			m.status = "update failed: " + message.Error.Error()
		} else {
			m.status = "update complete! restart lazydiff"
		}
	case branchFileReadyMsg:
		return m, m.openTempEditorCmd(message.Path)
	case editorDoneMsg:
		m.status = "editor closed"
		return m, m.refreshCmd()
	case editorErrorMsg:
		m.status = "editor error: " + message.Err.Error()
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

func (m Model) updateSearchKey(key tea.KeyMsg) (Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.searchActive = false
		m.searchQuery = ""
		m.searchFilter = nil
		return m, nil
	case "enter":
		m.searchActive = false
		return m, nil
	case "n":
		visible := m.visibleNodes()
		for i, n := range visible {
			if n.ID() == m.tree.selectedID && i < len(visible)-1 {
				m.tree.selectNode(visible[i+1])
				break
			}
		}
		return m, nil
	case "N":
		visible := m.visibleNodes()
		for i, n := range visible {
			if n.ID() == m.tree.selectedID && i > 0 {
				m.tree.selectNode(visible[i-1])
				break
			}
		}
		return m, nil
	case "backspace":
		if len(m.searchQuery) > 0 {
			m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
		}
		return m.applySearchFilter(), nil
	default:
		m.searchQuery += key.String()
		return m.applySearchFilter(), nil
	}
}

func (m Model) updateDialogKey(key tea.KeyMsg) (Model, tea.Cmd) {
	action, cmd := m.dialog.Update(key)
	switch action {
	case ActionCancel:
		m.dialog = nil
		return m, nil
	case ActionConfirm:
		return m.confirmDialogCmd()
	case ActionRegenerate:
		return m.regenerateDialogCmd()
	}
	return m, cmd
}

func (m Model) confirmDialogCmd() (Model, tea.Cmd) {
	if m.dialog == nil || !m.dialog.Ready || m.dialog.Err != nil {
		return m, nil
	}
	text := m.dialog.Text()
	switch m.dialog.Kind {
	case CommitDialog:
		mutator := m.mutator
		return m, func() tea.Msg { return commitResultMsg{Err: mutator.Commit(context.Background(), text)} }
	case PRDialog:
		title, body := splitSubjectBody(text)
		return m, m.confirmPRCmd(title, body)
	case RequestChangesDialog:
		// Stay open until prActionDoneMsg arrives (mirrors ClosePRDialog /
		// ApproveDialog / MergeDialog); closed there on success, error
		// attached and dialog kept open on failure.
		if m.prReviewer == nil || m.prSelector == nil || m.prSelector.selectedPR == nil {
			return m, nil
		}
		reviewer := m.prReviewer
		num := m.prSelector.selectedPR.Number
		return m, func() tea.Msg {
			return prActionDoneMsg{Err: reviewer.RequestChanges(context.Background(), num, text)}
		}
	}
	return m, nil
}

func (m Model) updateConfirmKey(key tea.KeyMsg) (Model, tea.Cmd) {
	action, _ := m.confirm.Update(key)
	switch action {
	case ActionCancel:
		m.confirm = nil
		return m, nil
	case ActionConfirm:
		// Stay open until prActionDoneMsg arrives — closing here would hide
		// a failure behind a status-line message that's easy to miss.
		return m, m.confirmPRActionCmd(m.confirm.Kind)
	}
	return m, nil
}

func (m Model) confirmPRActionCmd(kind DialogKind) tea.Cmd {
	if m.prReviewer == nil || m.prSelector == nil || m.prSelector.selectedPR == nil {
		return nil
	}
	reviewer := m.prReviewer
	num := m.prSelector.selectedPR.Number
	head := m.prSelector.selectedPR.HeadRefName
	switch kind {
	case ApproveDialog:
		return func() tea.Msg {
			return prActionDoneMsg{Err: reviewer.Approve(context.Background(), num, "")}
		}
	case MergeDialog:
		return func() tea.Msg {
			return prActionDoneMsg{Err: reviewer.Merge(context.Background(), num)}
		}
	case ClosePRDialog:
		return func() tea.Msg {
			if err := reviewer.Close(context.Background(), num); err != nil {
				return prActionDoneMsg{Err: err}
			}
			// Close succeeded — treat branch deletion as best-effort, but
			// surface a failure as a distinguishable warning rather than
			// swallowing it: a stale remote branch needs manual cleanup.
			if err := reviewer.DeleteBranch(context.Background(), head); err != nil {
				return prActionDoneMsg{Warning: fmt.Sprintf("closed PR #%d but failed to delete branch %q: %v", num, head, err)}
			}
			return prActionDoneMsg{}
		}
	}
	return nil
}

func (m Model) regenerateDialogCmd() (Model, tea.Cmd) {
	if m.dialog == nil {
		return m, nil
	}
	switch m.dialog.Kind {
	case CommitDialog:
		return m, m.startCommitCmd()
	case PRDialog:
		return m, m.startPRCmd()
	case RequestChangesDialog:
		// No-op by design: this is a hand-written comment, not an
		// AI-generated draft — there's nothing to regenerate (ctrl+r
		// regenerate is also omitted from its hint text in renderDialog).
		return m, nil
	}
	return m, nil
}

func (m Model) confirmPRCmd(title, body string) tea.Cmd {
	mutator, opener := m.mutator, m.opener
	return func() tea.Msg {
		ctx := context.Background()
		branch, err := mutator.CurrentBranch(ctx)
		if err != nil {
			return prResultMsg{Err: err}
		}
		if err := mutator.Push(ctx, "origin", branch); err != nil {
			return prResultMsg{Err: err}
		}
		remoteURL, err := mutator.RemoteURL(ctx, "origin")
		if err != nil {
			return prResultMsg{Err: err}
		}
		base, err := mutator.DefaultBranch(ctx)
		if err != nil {
			return prResultMsg{Err: err}
		}
		compareURL, err := pr.CompareURL(remoteURL, base, branch, title, body)
		if err != nil {
			return prResultMsg{Err: err}
		}
		if err := opener.Open(ctx, compareURL); err != nil {
			return prResultMsg{Err: err}
		}
		return prResultMsg{}
	}
}

func (m Model) updateKey(key tea.KeyMsg) (Model, tea.Cmd) {
	// pendingPRKey ('g'-prefix for ga/gr/gm/gd) is only valid for the very
	// next keypress; clear it here unconditionally so it can never leak
	// into an unrelated later keystroke. Only "g" itself re-arms it below.
	pendingG := m.pendingPRKey == 'g'
	m.pendingPRKey = 0
	switch key.String() {
	case "tab":
		m.focus = (m.focus + 1) % 3
	case "shift+tab":
		m.focus = (m.focus + 2) % 3
	case "up", "k":
		if m.focus == FocusTree && m.treeMode == TreeModePRSelector && m.prSelector != nil {
			m.prSelector.Move(-1)
			return m, nil
		}
		if m.focus == FocusTree && m.treeMode == TreeModeBranchSelector && m.branchSelector != nil {
			m.branchSelector.Move(-1)
		} else if m.focus == FocusTree {
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
		if m.focus == FocusTree && m.treeMode == TreeModePRSelector && m.prSelector != nil {
			m.prSelector.Move(1)
			return m, nil
		}
		if m.focus == FocusTree && m.treeMode == TreeModeBranchSelector && m.branchSelector != nil {
			m.branchSelector.Move(1)
		} else if m.focus == FocusTree {
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
	case "enter":
		if m.focus == FocusTree && m.treeMode == TreeModeBranchSelector && m.branchSelector != nil {
			branch := m.branchSelector.Selected()
			if branch != "" {
				m.branchSelector.Select(branch)
				m.treeMode = TreeModeBranchDiff
				return m, m.refreshCmd()
			}
		}
		if m.focus == FocusTree && m.treeMode == TreeModePRSelector && m.prSelector != nil {
			p := m.prSelector.Selected()
			if p != nil {
				m.prSelector.Select(p.Number)
				return m, m.openSelectedPRCmd()
			}
		}
	case " ":
		if m.focus == FocusTree && m.treeMode == TreeModeWorktree {
			m.tree.ToggleCheck()
		}
	case "ctrl+a":
		if m.focus == FocusTree && m.treeMode == TreeModeWorktree {
			m.tree.ToggleCheckAll()
		}
	case "h":
		if m.focus == FocusTree && m.treeMode == TreeModePRDiff {
			m.treeMode = TreeModePRSelector
			return m, nil
		}
		if m.focus == FocusTree && m.treeMode == TreeModePRSelector {
			m.treeMode = TreeModeBranchSelector
			return m, nil
		}
		if m.focus == FocusTree && m.treeMode == TreeModeBranchDiff {
			m.treeMode = TreeModeBranchSelector
			return m, nil
		}
		if m.focus == FocusTree {
			m.tree.CollapseOrParent()
			m.diffScroll = 0
			return m, m.renderSelectedCmd()
		}
	case "l":
		if m.focus == FocusTree && m.treeMode == TreeModePRSelector && m.prSelector != nil {
			p := m.prSelector.Selected()
			if p != nil {
				m.prSelector.Select(p.Number)
				return m, m.openSelectedPRCmd()
			}
		}
		if m.focus == FocusTree && m.treeMode == TreeModeBranchSelector && m.branchSelector != nil {
			branch := m.branchSelector.Selected()
			if branch != "" {
				m.branchSelector.Select(branch)
				m.treeMode = TreeModeBranchDiff
				return m, m.refreshCmd()
			}
		}
		if m.focus == FocusTree {
			m.tree.ExpandOrDescend()
			m.diffScroll = 0
			return m, m.renderSelectedCmd()
		}
	case "[":
		if m.focus == FocusAnalysis {
			if m.activeTab > 0 {
				m.activeTab--
			} else {
				m.activeTab = RequestLogTab
			}
		} else if m.focus == FocusTree {
			switch m.treeMode {
			case TreeModePRSelector:
				m.treeMode = TreeModeBranchSelector
			case TreeModeBranchSelector:
				m.treeMode = TreeModeWorktree
			default:
				m.treeMode = TreeModePRSelector
				if m.prSelector == nil {
					return m, m.loadPRsCmd()
				}
			}
		}
	case "]":
		if m.focus == FocusAnalysis {
			if m.activeTab < RequestLogTab {
				m.activeTab++
			} else {
				m.activeTab = DetailTab
			}
		} else if m.focus == FocusTree {
			switch m.treeMode {
			case TreeModeWorktree:
				m.treeMode = TreeModeBranchSelector
				if m.branchSelector == nil {
					return m, m.loadBranchesCmd()
				}
			case TreeModeBranchSelector:
				m.treeMode = TreeModePRSelector
				if m.prSelector == nil {
					return m, m.loadPRsCmd()
				}
			default:
				m.treeMode = TreeModeWorktree
			}
		}
	case "1":
		m.focus = FocusTree
	case "2":
		m.focus = FocusDiff
	case "3":
		m.focus = FocusAnalysis
	case "a":
		if pendingG && m.focus == FocusTree && m.treeMode == TreeModePRDiff && m.prSelector != nil && m.prSelector.selectedPR != nil {
			num := m.prSelector.selectedPR.Number
			title := m.prSelector.selectedPR.Title
			m.confirm = NewConfirmDialog(ApproveDialog, fmt.Sprintf("Approve PR #%d (%s)", num, title))
			return m, nil
		}
		m.activeTab = OverallTab
		return m, m.startAnalysis(false)
	case "A":
		m.activeTab = DetailTab
		return m, m.startAnalysis(true)
	case "/":
		if m.focus == FocusTree {
			m.searchActive = true
			m.searchQuery = ""
			return m, nil
		}
	case "x":
		m.cancelActive()
	case "c":
		if m.treeMode == TreeModeWorktree && len(m.tree.StagingPlan()) > 0 {
			return m, m.startCommitCmd()
		}
	case "e":
		if m.focus == FocusTree && m.treeMode == TreeModeWorktree {
			if file, _, ok := m.tree.Selected(); ok {
				path := filepath.Join(m.repo.Root, file.Path)
				return m, m.openEditorCmd(path)
			}
		}
		if m.focus == FocusTree && m.treeMode == TreeModeBranchDiff && m.branchSelector != nil && m.branchSelector.selectedBranch != "" {
			if file, _, ok := m.tree.Selected(); ok {
				wtPath, _ := m.branchSelector.WorktreePath(m.branchSelector.selectedBranch)
				return m, m.openBranchFileCmd(m.branchSelector.selectedBranch, file.Path, wtPath)
			}
		}
	case "o":
		if m.treeMode == TreeModePRDiff && m.prSelector != nil && m.prSelector.selectedPR != nil {
			return m, m.openPRInBrowserCmd()
		}
		if m.treeMode == TreeModeBranchDiff && m.branchSelector != nil && m.branchSelector.selectedBranch != "" {
			return m, m.startPRForReviewedBranchCmd()
		}
		return m, m.startPRCmd()
	case "m":
		if pendingG && m.focus == FocusTree && m.treeMode == TreeModePRDiff && m.prSelector != nil && m.prSelector.selectedPR != nil {
			num := m.prSelector.selectedPR.Number
			title := m.prSelector.selectedPR.Title
			m.confirm = NewConfirmDialog(MergeDialog, fmt.Sprintf("Merge PR #%d (%s)", num, title))
		}
		return m, nil
	case "d":
		if pendingG && m.focus == FocusTree && m.treeMode == TreeModePRDiff && m.prSelector != nil && m.prSelector.selectedPR != nil {
			num := m.prSelector.selectedPR.Number
			title := m.prSelector.selectedPR.Title
			head := m.prSelector.selectedPR.HeadRefName
			m.confirm = NewConfirmDialog(ClosePRDialog, fmt.Sprintf("Close PR #%d (%s) + delete branch %s", num, title, head))
		}
		return m, nil
	case "r":
		if pendingG && m.focus == FocusTree && m.treeMode == TreeModePRDiff && m.prSelector != nil && m.prSelector.selectedPR != nil {
			d := NewActionDialog(RequestChangesDialog)
			d.SetDraft("", nil)
			m.dialog = d
			return m, nil
		}
		if m.focus == FocusTree && m.treeMode == TreeModePRSelector {
			return m, m.loadPRsCmd()
		}
		return m, m.refreshCmd()
	case "g":
		if m.focus == FocusTree && m.treeMode == TreeModePRDiff && m.prSelector != nil && m.prSelector.selectedPR != nil {
			m.pendingPRKey = 'g'
			return m, nil
		}
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
	case "u":
		if m.showUpdateModal {
			m.showUpdateModal = false
			m.showUpdating = true
			m.status = "downloading update " + m.updateVersion + "..."
			return m, performUpdateCmd()
		}
		if m.showUpdating {
			return m, nil
		}
		m.updateManual = true
		m.updateStatus = "checking..."
		return m, checkUpdateCmd(false)
	case "n", "y":
		if m.showUpdateModal {
			m.showUpdateModal = false
			if key.String() == "y" {
				m.showUpdating = true
				m.status = "downloading update " + m.updateVersion + "..."
				return m, performUpdateCmd()
			}
		}
	case "?":
		m.showHelp = !m.showHelp
	case "esc":
		if m.showHelp {
			m.showHelp = false
		}
	case "q", "ctrl+c":
		m.cancelActive()
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) refreshCmd() tea.Cmd {
	if m.treeMode == TreeModePRDiff && m.prSelector != nil && m.prSelector.selectedPR != nil {
		// Route through prDiffLoadedMsg (not a bare snapshotMsg) so its
		// handler refreshes prSelector.diffCache[num] too — otherwise a
		// stale cache entry would resurface on the next h → enter.
		loader := m.loader
		num := m.prSelector.selectedPR.Number
		return func() tea.Msg {
			snapshot, err := loader.SnapshotPR(context.Background(), num)
			if err != nil {
				return prDiffLoadedMsg{Number: num, Err: err}
			}
			return prDiffLoadedMsg{Number: num, Snapshot: snapshot}
		}
	}
	if m.treeMode == TreeModeBranchDiff && m.branchSelector != nil && m.branchSelector.selectedBranch != "" {
		loader := m.loader
		branch := m.branchSelector.selectedBranch
		return func() tea.Msg {
			snapshot, err := loader.SnapshotBranch(context.Background(), branch)
			if err != nil {
				return snapshotErrorMsg{Err: err}
			}
			return snapshotMsg{Snapshot: snapshot}
		}
	}
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

func (m Model) openEditorCmd(path string) tea.Cmd {
	cmd := exec.Command("nvim", path)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			return editorErrorMsg{Err: err}
		}
		return editorDoneMsg{}
	})
}

func (m Model) openBranchFileCmd(branch, filePath, worktreePath string) tea.Cmd {
	if worktreePath != "" {
		path := filepath.Join(worktreePath, filePath)
		cmd := exec.Command("nvim", path)
		return tea.ExecProcess(cmd, func(err error) tea.Msg {
			if err != nil {
				return editorErrorMsg{Err: err}
			}
			return editorDoneMsg{}
		})
	}
	repoRoot := m.repo.Root
	return func() tea.Msg {
		tmpFile, err := os.CreateTemp("", "lazydiff-*.go")
		if err != nil {
			return editorErrorMsg{Err: fmt.Errorf("create temp file: %w", err)}
		}
		tmpPath := tmpFile.Name()
		tmpFile.Close()

		gitShow := exec.Command("git", "-C", repoRoot, "show", branch+":"+filePath)
		out, err := gitShow.Output()
		if err != nil {
			os.Remove(tmpPath)
			return editorErrorMsg{Err: fmt.Errorf("git show %s:%s: %w", branch, filePath, err)}
		}
		if err := os.WriteFile(tmpPath, out, 0o644); err != nil {
			os.Remove(tmpPath)
			return editorErrorMsg{Err: fmt.Errorf("write temp file: %w", err)}
		}
		return branchFileReadyMsg{Path: tmpPath}
	}
}

func (m Model) openTempEditorCmd(path string) tea.Cmd {
	return tea.ExecProcess(exec.Command("nvim", path), func(err error) tea.Msg {
		os.Remove(path)
		if err != nil {
			return editorErrorMsg{Err: err}
		}
		return editorDoneMsg{}
	})
}

func (m Model) loadBranchesCmd() tea.Cmd {
	repo := m.repo
	return func() tea.Msg {
		branches, err := repo.Branches(context.Background())
		if err != nil {
			return branchesErrorMsg{Err: err}
		}
		current, err := repo.CurrentBranch(context.Background())
		if err != nil {
			return branchesErrorMsg{Err: err}
		}
		def, err := repo.DefaultBranch(context.Background())
		if err != nil {
			return branchesErrorMsg{Err: err}
		}
		worktrees, _ := repo.Worktrees(context.Background())
		return branchesLoadedMsg{Branches: branches, Current: current, Default: def, Worktrees: worktrees}
	}
}

func (m Model) loadPRsCmd() tea.Cmd {
	reviewer := m.prReviewer
	return func() tea.Msg {
		if reviewer == nil {
			return prsLoadedMsg{Err: fmt.Errorf("pr reviewer unavailable")}
		}
		prs, err := reviewer.ListPRs(context.Background(), "open")
		return prsLoadedMsg{PRs: prs, Err: err}
	}
}

func (m Model) openPRInBrowserCmd() tea.Cmd {
	if m.prSelector == nil || m.prSelector.selectedPR == nil || m.opener == nil {
		return nil
	}
	opener := m.opener
	url := m.prSelector.selectedPR.URL
	return func() tea.Msg {
		return prResultMsg{Err: opener.Open(context.Background(), url)}
	}
}

func (m Model) openSelectedPRCmd() tea.Cmd {
	if m.prSelector == nil || m.prSelector.selectedPR == nil {
		return nil
	}
	num := m.prSelector.selectedPR.Number
	if cached, ok := m.prSelector.diffCache[num]; ok {
		return func() tea.Msg { return prDiffLoadedMsg{Number: num, Snapshot: cached} }
	}
	loader := m.loader
	return func() tea.Msg {
		snapshot, err := loader.SnapshotPR(context.Background(), num)
		if err != nil {
			return prDiffLoadedMsg{Number: num, Err: err}
		}
		return prDiffLoadedMsg{Number: num, Snapshot: snapshot}
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
		raw = "--- a/" + file.DisplayPath() + "\n+++ b/" + file.DisplayPath() + "\n" + hunk.RawDiff()
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

func (m Model) startCommitCmd() tea.Cmd {
	mutator, loader, cfg, templates, plan := m.mutator, m.loader, m.cfg, m.templates, m.tree.StagingPlan()
	repoRoot := m.repo.Root
	return func() tea.Msg {
		ctx := context.Background()
		for _, action := range plan {
			var err error
			if len(action.PartialHunks) == 0 {
				err = mutator.StageFile(ctx, action.File.OldPath, action.File.Path)
			} else {
				err = mutator.StagePatch(ctx, diff.BuildPatch(action.File, action.PartialHunks))
			}
			if err != nil {
				return commitPrepMsg{Err: err}
			}
		}
		staged, err := loader.Snapshot(ctx, git.Staged)
		if err != nil {
			return commitPrepMsg{Err: err}
		}
		branch, err := mutator.CurrentBranch(ctx)
		if err != nil {
			return commitPrepMsg{Err: err}
		}
		ticket, err := pr.ExtractTicket(cfg.PR.TicketPattern, branch)
		if err != nil {
			return commitPrepMsg{Err: err}
		}
		rendered, err := templates.RenderCommitMessage(prompt.Context{
			Repository: repoRoot,
			Mode:       staged.Mode.String(),
			StagedDiff: staged.RawDiff,
			Ticket:     ticket,
		})
		if err != nil {
			return commitPrepMsg{Err: err}
		}
		return commitPrepMsg{Ticket: ticket, Prompt: rendered}
	}
}

func (m Model) runCommitAgentCmd(renderedPrompt, ticket string) tea.Cmd {
	runner, repoRoot := m.runner, m.repo.Root
	return func() tea.Msg {
		var output strings.Builder
		err := runner.Run(context.Background(), agent.Request{RepoRoot: repoRoot, Prompt: renderedPrompt}, func(event agent.Event) {
			if event.Kind == agent.Output {
				if output.Len() > 0 {
					output.WriteByte('\n')
				}
				output.WriteString(event.Text)
			}
		})
		return commitDraftMsg{Ticket: ticket, Text: output.String(), Err: err}
	}
}

func (m Model) startPRCmd() tea.Cmd {
	mutator, loader, cfg, templates := m.mutator, m.loader, m.cfg, m.templates
	repoRoot := m.repo.Root
	return func() tea.Msg {
		ctx := context.Background()
		branch, err := mutator.CurrentBranch(ctx)
		if err != nil {
			return prPrepMsg{Err: err}
		}
		base, err := mutator.DefaultBranch(ctx)
		if err != nil {
			return prPrepMsg{Err: err}
		}
		if branch == base {
			return prPrepMsg{Err: fmt.Errorf("cannot open a pull request from the default branch %q", base)}
		}
		snapshot, err := loader.Snapshot(ctx, git.Branch)
		if err != nil {
			return prPrepMsg{Err: err}
		}
		ticket, err := pr.ExtractTicket(cfg.PR.TicketPattern, branch)
		if err != nil {
			return prPrepMsg{Err: err}
		}
		rendered, err := templates.RenderPRDescription(prompt.Context{
			Repository: repoRoot,
			Branch:     branch,
			BaseBranch: base,
			BranchDiff: snapshot.RawDiff,
			Ticket:     ticket,
		})
		if err != nil {
			return prPrepMsg{Err: err}
		}
		return prPrepMsg{Ticket: ticket, Prompt: rendered}
	}
}

func (m Model) startPRForReviewedBranchCmd() tea.Cmd {
	mutator, loader, cfg, templates := m.mutator, m.loader, m.cfg, m.templates
	branch := m.branchSelector.selectedBranch
	repoRoot := m.repo.Root
	return func() tea.Msg {
		ctx := context.Background()
		base, err := mutator.DefaultBranch(ctx)
		if err != nil {
			return prPrepMsg{Err: err}
		}
		if branch == base {
			return prPrepMsg{Err: fmt.Errorf("cannot open a pull request from the default branch %q", base)}
		}
		snapshot, err := loader.SnapshotBranch(ctx, branch)
		if err != nil {
			return prPrepMsg{Err: err}
		}
		ticket, err := pr.ExtractTicket(cfg.PR.TicketPattern, branch)
		if err != nil {
			return prPrepMsg{Err: err}
		}
		rendered, err := templates.RenderPRDescription(prompt.Context{
			Repository: repoRoot,
			Branch:     branch,
			BaseBranch: base,
			BranchDiff: snapshot.RawDiff,
			Ticket:     ticket,
		})
		if err != nil {
			return prPrepMsg{Err: err}
		}
		return prPrepMsg{Ticket: ticket, Prompt: rendered}
	}
}

func (m Model) runPRAgentCmd(renderedPrompt, ticket string) tea.Cmd {
	runner, repoRoot := m.runner, m.repo.Root
	return func() tea.Msg {
		var output strings.Builder
		err := runner.Run(context.Background(), agent.Request{RepoRoot: repoRoot, Prompt: renderedPrompt}, func(event agent.Event) {
			if event.Kind == agent.Output {
				if output.Len() > 0 {
					output.WriteByte('\n')
				}
				output.WriteString(event.Text)
			}
		})
		return prDraftMsg{Ticket: ticket, Text: output.String(), Err: err}
	}
}

func splitSubjectBody(text string) (string, string) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", ""
	}
	lines := strings.SplitN(trimmed, "\n", 2)
	subject := strings.TrimSpace(lines[0])
	body := ""
	if len(lines) > 1 {
		body = strings.TrimSpace(lines[1])
	}
	return subject, body
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

func checkUpdateCmd(auto bool) tea.Cmd {
	return func() tea.Msg {
		hasUpdate, versionStr, err := version.CheckForUpdate()
		if err != nil {
			return updateResultMsg{Error: err, Manual: !auto}
		}
		return updateResultMsg{HasUpdate: hasUpdate, Version: versionStr, Manual: !auto}
	}
}

func performUpdateCmd() tea.Cmd {
	return func() tea.Msg {
		err := version.PerformUpdate()
		return updatePerformedMsg{Error: err}
	}
}

func (m TreeMode) String() string {
	switch m {
	case TreeModeWorktree:
		return "worktree"
	case TreeModeStaged:
		return "staged"
	case TreeModeBranchSelector:
		return "branch selector"
	case TreeModeBranchDiff:
		return "branch diff"
	case TreeModePRSelector:
		return "pr selector"
	case TreeModePRDiff:
		return "pr diff"
	default:
		return "unknown"
	}
}

func (m Model) applySearchFilter() Model {
	if !m.searchActive || m.searchQuery == "" {
		m.searchFilter = nil
		return m
	}
	re, err := regexp.Compile("(?i)" + m.searchQuery)
	if err != nil {
		m.searchFilter = nil
		m.status = "search: " + err.Error()
		return m
	}
	m.searchFilter = re
	return m
}

func (m Model) visibleNodes() []*TreeNode {
	if m.searchFilter == nil {
		return m.tree.Rows()
	}
	all := m.tree.Rows()
	var result []*TreeNode
	for _, n := range all {
		if m.searchFilter.MatchString(nodeSearchLabel(n)) {
			result = append(result, n)
		}
	}
	return result
}

func nodeSearchLabel(n *TreeNode) string {
	if n.File != nil {
		return n.File.DisplayPath()
	}
	return n.Label
}
