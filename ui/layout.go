package ui

type Rect struct {
	X, Y, W, H int
}

type Layout struct {
	Tree, Diff, Analysis, Status Rect
}

func ComputeLayout(width, height int) Layout {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	statusH := 1
	bodyH := height - statusH
	if bodyH < 0 {
		bodyH = 0
	}
	treeW := width * 28 / 100
	if treeW < 20 && width >= 20 {
		treeW = 20
	}
	if treeW > width/3 {
		treeW = width / 3
	}
	if width < 20 {
		treeW = width
	}
	rightW := width - treeW
	if width < 80 {
		return Layout{
			Tree:     Rect{X: 0, Y: 0, W: treeW, H: bodyH / 3},
			Diff:     Rect{X: 0, Y: bodyH / 3, W: width, H: bodyH / 2},
			Analysis: Rect{X: 0, Y: bodyH/3 + bodyH/2, W: width, H: bodyH - bodyH/3 - bodyH/2},
			Status:   Rect{X: 0, Y: bodyH, W: width, H: statusH},
		}
	}
	diffH := bodyH * 57 / 100
	return Layout{
		Tree:     Rect{X: 0, Y: 0, W: treeW, H: bodyH},
		Diff:     Rect{X: treeW, Y: 0, W: rightW, H: diffH},
		Analysis: Rect{X: treeW, Y: diffH, W: rightW, H: bodyH - diffH},
		Status:   Rect{X: 0, Y: bodyH, W: width, H: statusH},
	}
}
