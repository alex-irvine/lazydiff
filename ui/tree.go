package ui

import "github.com/alex-irvine/lazydiff/diff"

type TreeNode struct {
	File     *diff.File
	Hunk     *diff.Hunk
	Depth    int
	Expanded bool
}

type TreeModel struct {
	files      []diff.File
	cursor     int
	expanded   map[string]bool
	selectedID string
}

func NewTree(files []diff.File) *TreeModel {
	t := &TreeModel{expanded: make(map[string]bool)}
	t.SetFiles(files)
	return t
}

func (t *TreeModel) SetFiles(files []diff.File) {
	selected := t.selectedID
	t.files = append([]diff.File(nil), files...)
	if t.expanded == nil {
		t.expanded = make(map[string]bool)
	}
	rows := t.rows()
	if len(rows) == 0 {
		t.cursor = 0
		t.selectedID = ""
		return
	}
	for i, row := range rows {
		if row.File.ID == selected || row.Hunk != nil && row.Hunk.ID == selected {
			t.cursor = i
			t.selectedID = selected
			return
		}
	}
	if t.cursor >= len(rows) {
		t.cursor = len(rows) - 1
	}
	t.selectedID = rows[t.cursor].ID()
}

func (t *TreeModel) Move(delta int) {
	rows := t.rows()
	if len(rows) == 0 {
		return
	}
	t.cursor += delta
	if t.cursor < 0 {
		t.cursor = 0
	}
	if t.cursor >= len(rows) {
		t.cursor = len(rows) - 1
	}
	t.selectedID = rows[t.cursor].ID()
}

func (t *TreeModel) Toggle() {
	rows := t.rows()
	if len(rows) == 0 || t.cursor >= len(rows) || rows[t.cursor].Hunk != nil {
		return
	}
	file := rows[t.cursor].File
	t.expanded[file.ID] = !t.expanded[file.ID]
	selected := file.ID
	t.selectedID = selected
	t.SetFiles(t.files)
}

func (t *TreeModel) Selected() (diff.File, *diff.Hunk, bool) {
	rows := t.rows()
	if len(rows) == 0 || t.cursor >= len(rows) {
		return diff.File{}, nil, false
	}
	row := rows[t.cursor]
	if row.Hunk == nil {
		return *row.File, nil, true
	}
	return *row.File, row.Hunk, true
}

func (t *TreeModel) Rows() []TreeNode { return append([]TreeNode(nil), t.rows()...) }

func (t *TreeModel) rows() []TreeNode {
	var rows []TreeNode
	for i := range t.files {
		file := &t.files[i]
		rows = append(rows, TreeNode{File: file, Depth: 0, Expanded: t.expanded[file.ID]})
		if !t.expanded[file.ID] {
			continue
		}
		for j := range file.Hunks {
			hunk := &file.Hunks[j]
			rows = append(rows, TreeNode{File: file, Hunk: hunk, Depth: 1})
		}
	}
	return rows
}

func (n TreeNode) ID() string {
	if n.Hunk != nil {
		return n.Hunk.ID
	}
	if n.File != nil {
		return n.File.ID
	}
	return ""
}
