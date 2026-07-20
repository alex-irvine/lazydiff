package ui

type Rect struct {
	X, Y, W, H int
}

type Layout struct {
	Files, Code, Agent, Status Rect
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
	leftW := width * 28 / 100
	if leftW < 20 && width >= 20 {
		leftW = 20
	}
	if leftW > width/3 {
		leftW = width / 3
	}
	if width < 20 {
		leftW = width
	}
	rightW := width - leftW
	if width < 80 {
		filesH := bodyH / 3
		codeH := bodyH / 2
		agentH := bodyH - filesH - codeH
		return Layout{
			Files:  Rect{X: 0, Y: 0, W: width, H: filesH},
			Code:   Rect{X: 0, Y: filesH, W: width, H: codeH},
			Agent:  Rect{X: 0, Y: filesH + codeH, W: width, H: agentH},
			Status: Rect{X: 0, Y: bodyH, W: width, H: statusH},
		}
	}
	filesH := bodyH / 2
	agentH := bodyH - filesH
	return Layout{
		Files:  Rect{X: 0, Y: 0, W: leftW, H: filesH},
		Code:   Rect{X: leftW, Y: 0, W: rightW, H: bodyH},
		Agent:  Rect{X: 0, Y: filesH, W: leftW, H: agentH},
		Status: Rect{X: 0, Y: bodyH, W: width, H: statusH},
	}
}
