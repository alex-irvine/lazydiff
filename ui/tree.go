package ui

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/alex-irvine/lazydiff/diff"
)

type TreeNode struct {
	IDValue  string
	Label    string
	Level    int
	Expanded bool
	Children []*TreeNode
	File     *diff.File
	Hunk     *diff.Hunk
}

func (n *TreeNode) ID() string {
	if n.Hunk != nil {
		return n.Hunk.ID
	}
	if n.File != nil {
		return n.File.ID
	}
	return n.IDValue
}

func (n *TreeNode) IsLeaf() bool {
	return len(n.Children) == 0
}

type TreeModel struct {
	roots        []*TreeNode
	flatNodes    []*TreeNode
	cursor       int
	selectedID   string
	scrollOffset int
}

func NewTree(files []diff.File) *TreeModel {
	t := &TreeModel{}
	t.SetFiles(files)
	return t
}

func (t *TreeModel) SetFiles(files []diff.File) {
	selected := t.selectedID
	savedExpanded := t.collectExpandedIDs()

	t.roots = buildTree(files)
	t.applyExpandedIDs(t.roots, savedExpanded)
	t.flatten()

	if selected != "" {
		t.restoreSelection(selected)
	}
	if len(t.flatNodes) > 0 && (t.cursor >= len(t.flatNodes) || t.selectedID == "") {
		t.cursor = 0
		t.selectedID = t.flatNodes[0].ID()
	}
}

func (t *TreeModel) collectExpandedIDs() map[string]bool {
	ids := make(map[string]bool)
	var walk func([]*TreeNode)
	walk = func(nodes []*TreeNode) {
		for _, n := range nodes {
			if n.Expanded && !n.IsLeaf() {
				ids[n.ID()] = true
			}
			if len(n.Children) > 0 {
				walk(n.Children)
			}
		}
	}
	walk(t.roots)
	return ids
}

func (t *TreeModel) applyExpandedIDs(nodes []*TreeNode, ids map[string]bool) {
	for _, n := range nodes {
		if ids[n.ID()] {
			n.Expanded = true
		}
		if len(n.Children) > 0 {
			t.applyExpandedIDs(n.Children, ids)
		}
	}
}

func buildTree(files []diff.File) []*TreeNode {
	dirs := make(map[string][]*diff.File)
	for i := range files {
		f := &files[i]
		dir := filepath.Dir(f.DisplayPath())
		if dir == "." || dir == "/" {
			dir = ""
		}
		dirs[dir] = append(dirs[dir], f)
	}

	dirNames := make([]string, 0, len(dirs))
	for d := range dirs {
		dirNames = append(dirNames, d)
	}
	sort.Strings(dirNames)

	var roots []*TreeNode
	for _, dir := range dirNames {
		fl := dirs[dir]
		sort.Slice(fl, func(i, j int) bool {
			return filepath.Base(fl[i].DisplayPath()) < filepath.Base(fl[j].DisplayPath())
		})

		if dir == "" {
			for _, f := range fl {
				roots = append(roots, makeFileNode(f))
			}
			continue
		}

		parts := strings.Split(dir, string(filepath.Separator))
		roots = ensureDirPath(roots, parts, fl, "")
	}

	return roots
}

func makeFileNode(f *diff.File) *TreeNode {
	node := &TreeNode{
		Label:    filepath.Base(f.DisplayPath()),
		File:     f,
		Expanded: false,
	}
	for i := range f.Hunks {
		h := &f.Hunks[i]
		node.Children = append(node.Children, &TreeNode{
			Label: h.Header,
			File:  f,
			Hunk:  h,
		})
	}
	return node
}

func ensureDirPath(nodes []*TreeNode, parts []string, files []*diff.File, parentPath string) []*TreeNode {
	if len(parts) == 0 {
		for _, f := range files {
			nodes = append(nodes, makeFileNode(f))
		}
		return nodes
	}

	part := parts[0]
	dirPath := parentPath + part + "/"
	dirID := "dir:" + dirPath

	for _, n := range nodes {
		if n.IDValue == dirID {
			n.Children = ensureDirPath(n.Children, parts[1:], files, dirPath)
			return nodes
		}
	}

	dirNode := &TreeNode{
		IDValue:  dirID,
		Label:    part + "/",
		Expanded: true,
	}
	dirNode.Children = ensureDirPath(dirNode.Children, parts[1:], files, dirPath)
	nodes = append(nodes, dirNode)
	return nodes
}

func (t *TreeModel) flatten() {
	t.flatNodes = nil
	for _, root := range t.roots {
		root.Level = 0
		t.flattenNode(root)
	}
	if t.scrollOffset >= len(t.flatNodes) {
		t.scrollOffset = max(0, len(t.flatNodes)-1)
	}
}

func (t *TreeModel) ClampScroll(contentH int) {
	if len(t.flatNodes) == 0 {
		t.scrollOffset = 0
		return
	}
	if t.cursor < t.scrollOffset {
		t.scrollOffset = t.cursor
	}
	if t.cursor >= t.scrollOffset+contentH {
		t.scrollOffset = t.cursor - contentH + 1
	}
}

func (t *TreeModel) flattenNode(node *TreeNode) {
	t.flatNodes = append(t.flatNodes, node)
	if node.Expanded {
		for _, child := range node.Children {
			child.Level = node.Level + 1
			t.flattenNode(child)
		}
	}
}

func (t *TreeModel) Move(delta int) {
	if len(t.flatNodes) == 0 {
		return
	}
	t.cursor += delta
	if t.cursor < 0 {
		t.cursor = 0
	}
	if t.cursor >= len(t.flatNodes) {
		t.cursor = len(t.flatNodes) - 1
	}
	t.selectedID = t.flatNodes[t.cursor].ID()
}

func (t *TreeModel) Toggle() {
	if len(t.flatNodes) == 0 || t.cursor >= len(t.flatNodes) {
		return
	}
	node := t.flatNodes[t.cursor]
	if node.IsLeaf() {
		return
	}
	node.Expanded = !node.Expanded
	selected := node.ID()
	t.flatten()
	t.restoreSelection(selected)
}

func (t *TreeModel) CollapseOrParent() {
	if len(t.flatNodes) == 0 || t.cursor >= len(t.flatNodes) {
		return
	}
	node := t.flatNodes[t.cursor]
	if !node.IsLeaf() && node.Expanded {
		node.Expanded = false
		selected := node.ID()
		t.flatten()
		t.restoreSelection(selected)
		return
	}
	if parent := t.findParent(node); parent != nil {
		t.selectNode(parent)
	}
}

func (t *TreeModel) ExpandOrDescend() {
	if len(t.flatNodes) == 0 || t.cursor >= len(t.flatNodes) {
		return
	}
	node := t.flatNodes[t.cursor]
	if node.IsLeaf() {
		return
	}
	if !node.Expanded {
		node.Expanded = true
		selected := node.ID()
		t.flatten()
		t.restoreSelection(selected)
		return
	}
	if t.cursor+1 < len(t.flatNodes) {
		t.cursor++
		t.selectedID = t.flatNodes[t.cursor].ID()
	}
}

func (t *TreeModel) findParent(target *TreeNode) *TreeNode {
	var search func([]*TreeNode) *TreeNode
	search = func(nodes []*TreeNode) *TreeNode {
		for _, n := range nodes {
			for _, child := range n.Children {
				if child == target {
					return n
				}
			}
			if len(n.Children) > 0 {
				if p := search(n.Children); p != nil {
					return p
				}
			}
		}
		return nil
	}
	return search(t.roots)
}

func (t *TreeModel) selectNode(target *TreeNode) {
	for i, n := range t.flatNodes {
		if n == target {
			t.cursor = i
			t.selectedID = n.ID()
			return
		}
	}
}

func (t *TreeModel) Selected() (diff.File, *diff.Hunk, bool) {
	if len(t.flatNodes) == 0 || t.cursor >= len(t.flatNodes) {
		return diff.File{}, nil, false
	}
	node := t.flatNodes[t.cursor]
	if node.File == nil {
		return diff.File{}, nil, false
	}
	return *node.File, node.Hunk, true
}

func (t *TreeModel) Rows() []*TreeNode {
	return append([]*TreeNode(nil), t.flatNodes...)
}

func (t *TreeModel) restoreSelection(id string) {
	for i, n := range t.flatNodes {
		if n.ID() == id {
			t.cursor = i
			t.selectedID = id
			return
		}
	}
}
