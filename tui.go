package main

import (
	"fmt"
	"io"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func renderHeader(frame *tview.Frame) {
	frame.Clear()
	frame.AddText(fmt.Sprintf("servers     %v", serversCount), true, tview.AlignLeft, tcell.ColorWhite)
	frame.AddText(fmt.Sprintf("shares      %v", sharesCount), true, tview.AlignLeft, tcell.ColorWhite)
	frame.AddText(fmt.Sprintf("folders     %v", foldersCount), true, tview.AlignLeft, tcell.ColorWhite)
	frame.AddText(fmt.Sprintf("files       %v", filesCount), true, tview.AlignLeft, tcell.ColorWhite)

	frame.
		AddText("smbwatch v0.1", true, tview.AlignRight, tcell.ColorWhite).
		AddText(fmt.Sprintf("commit %v", commitHash), true, tview.AlignRight, tcell.ColorWhite).
		AddText("telekom security", true, tview.AlignRight, tcell.ColorWhite)
}

func renderTui() (*tview.Application, io.Writer) {
	var app = tview.NewApplication()
	tview.Styles.PrimitiveBackgroundColor = tcell.ColorDefault

	body := tview.NewTextView().
		SetWrap(true).
		SetDynamicColors(true)
	body.SetChangedFunc(func() {
		if body.HasFocus() {
			app.Draw()
		}
	})

	frame := tview.NewFrame(body).SetBorders(2, 2, 2, 2, 4, 4)
	renderHeader(frame)

	go func() {
		for {
			app.QueueUpdateDraw(func() {
				renderHeader(frame)
			})

			time.Sleep(100 * time.Millisecond)
		}
	}()

	app.SetRoot(frame, true).SetFocus(frame)

	return app, body
}
