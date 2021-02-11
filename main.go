package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// ## Architecture
// A Handler is a function that takes a url and fetches the content. It returns a Page
// which contains all the relavent information. The browser history is a list of Pages.
// Various Render functions write the Page content to a tview TextView.

type LogMessageHandler struct {
	Text        string          // Internal buffer of log messages
	MessageLine *tview.TextView // TextView to display the last log to
	LogFile     io.Writer
}

func (handler *LogMessageHandler) Write(p []byte) (n int, err error) {
	handler.Text = handler.Text + string(p)
	if handler.LogFile != nil {
		fmt.Fprintf(handler.LogFile, string(p))
	}
	handler.MessageLine.Clear()
	// Cut off date/time
	if len(p) > 20 {
		p = p[20:]
	}
	fmt.Fprintf(handler.MessageLine, string(p))
	return len(p), nil
}

const DEFAULT_LOG_PATH = "log.log"
const HOME_PAGE = "gopher://gopher.floodgap.com/"

func main() {
	// Parse cli arguments:
	flag.Parse()
	var url = flag.Arg(0)
	fmt.Println(url)
	if url == "" {
		url = HOME_PAGE
	}

	// Build tview Application UI
	app := tview.NewApplication()
	textView := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetChangedFunc(func() {
			app.Draw()
		})
	textView.SetBorder(false)

	statusLine := tview.NewTextView()
	statusLine.SetTextColor(tcell.GetColor("black"))
	statusLine.SetBackgroundColor(tcell.GetColor("white"))

	messageLine := tview.NewTextView()
	messageLine.SetChangedFunc(func() {
		app.Draw()
		time.AfterFunc(3*time.Second, func() {
			messageLine.Clear()
			app.Draw()
		})
	})

	grid_layout := tview.NewGrid().
		SetRows(0, 1, 1).
		SetColumns(0).
		SetBorders(false)

	grid_layout.AddItem(textView, 0, 0, 1, 1, 0, 0, true)
	grid_layout.AddItem(statusLine, 1, 0, 1, 1, 0, 0, false)
	grid_layout.AddItem(messageLine, 2, 0, 1, 1, 0, 0, false)

	pageView := PageView{
		PageText:   textView,
		StatusLine: statusLine,
	}

	// Setup log file handling
	logFile, err := os.Create(DEFAULT_LOG_PATH)
	if err != nil {
		log.Println(err)
	} else {
		defer logFile.Close()
	}
	var logHandler = LogMessageHandler{
		MessageLine: messageLine,
		Text:        "",
		LogFile:     logFile,
	}
	log.SetOutput(&logHandler)

	// Go to a URL
	page, success := GopherHandler(url)
	if !success {
		log.Fatal("Failed to get gopher url")
	}
	pageView.RenderPage(&page)
	statusLine.Clear()
	fmt.Fprintf(statusLine, page.Url)
	page_history := []*Page{&page}
	history_index := 0

	// Set input handler for top level app
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'q':
			app.Stop()
			return nil
		}
		return event
	})

	// Set custom input handler for main view
	textView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'd':
			_, _, _, height := textView.GetRect()
			curr_row, _ := textView.GetScrollOffset()
			textView.ScrollTo(curr_row+height/2, 0)
			return nil
		case 'u':
			_, _, _, height := textView.GetRect()
			curr_row, _ := textView.GetScrollOffset()
			textView.ScrollTo(curr_row-height/2, 0)
			return nil
		case 'h':
			if history_index > 0 {
				row, _ := pageView.PageText.GetScrollOffset()
				page_history[history_index].ScrollOffset = row
				history_index -= 1
				prev_page := page_history[history_index]
				pageView.RenderPage(prev_page)
				statusLine.Clear()
				fmt.Fprintf(statusLine, prev_page.Url)
			} else {
				log.Println("Already at first page")
			}
			return nil
		case 'l':
			if history_index < len(page_history)-1 {
				row, _ := pageView.PageText.GetScrollOffset()
				page_history[history_index].ScrollOffset = row
				history_index += 1
				next_page := page_history[history_index]
				pageView.RenderPage(next_page)
				statusLine.Clear()
				fmt.Fprintf(statusLine, next_page.Url)
			} else {
				log.Println("Already at last page")
			}
			return nil
		case '\\':
			logView := tview.NewTextView().
				SetChangedFunc(func() {
					app.Draw()
				})
			logView.SetBorder(true)
			logView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
				if event.Rune() == '\\' {
					app.SetRoot(grid_layout, true).SetFocus(textView)
					return nil
				}
				return event
			})
			fmt.Fprintf(logView, logHandler.Text)
			app.SetRoot(logView, true).SetFocus(logView)
			return nil
		}

		// Bind number keys to quick select links
		for i := 1; i <= 9; i++ {
			if (event.Rune()) == rune(i+48) {
				if len(page.Links) >= i {
					current_page := page_history[history_index]
					row, _ := pageView.PageText.GetScrollOffset()
					current_page.ScrollOffset = row
					url := current_page.Links[i-1]
					page, success := GopherHandler(url)
					if !success {
						log.Println("Failed to get gopher url")
					}
					pageView.RenderPage(&page)
					statusLine.Clear()
					fmt.Fprintf(statusLine, page.Url)
					page_history = append(page_history[:history_index+1], &page)
					history_index += 1
					return nil
				}

			}

		}
		return event
	})

	if err := app.SetRoot(grid_layout, true).SetFocus(textView).Run(); err != nil {
		panic(err)
	}
}
