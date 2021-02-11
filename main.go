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

// Keeps track of page history and navigation
type HistoryManager struct {
	page_history  []*Page
	history_index int
}

// Navigates to a new page. All previous pages in the history are kept,
// but pages forward in the history are dropped
func (manager *HistoryManager) Navigate(page *Page) {
	if len(manager.page_history) == 0 { // initial page
		manager.page_history = []*Page{page}
		manager.history_index = 0
	} else {
		manager.page_history = append(manager.page_history[:manager.history_index+1], page)
		manager.history_index += 1
	}
}

// Move backwards in the history
// Returns nil if on the first page
func (manager *HistoryManager) Back() *Page {
	var prev_page *Page
	if manager.history_index > 0 {
		manager.history_index -= 1
		prev_page = manager.page_history[manager.history_index]
	} else {
		prev_page = nil
	}
	return prev_page
}

// Move forwards in the history.
// Returns nil if on the last page
func (manager *HistoryManager) Forward() *Page {
	var next_page *Page
	if manager.history_index < len(manager.page_history)-1 {
		manager.history_index += 1
		next_page = manager.page_history[manager.history_index]
	} else {
		next_page = nil
	}
	return next_page
}

// Get the current page
func (manager *HistoryManager) CurrentPage() *Page {
	return manager.page_history[manager.history_index]
}

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
	historyManager := HistoryManager{}
	historyManager.Navigate(&page)

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
			row, _ := pageView.PageText.GetScrollOffset()
			historyManager.CurrentPage().ScrollOffset = row
			prev_page := historyManager.Back()
			if prev_page != nil {
				pageView.RenderPage(prev_page)
			} else {
				log.Println("Already at first page")
			}
			return nil
		case 'l':
			row, _ := pageView.PageText.GetScrollOffset()
			historyManager.CurrentPage().ScrollOffset = row
			next_page := historyManager.Forward()
			if next_page != nil {
				pageView.RenderPage(next_page)
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
					current_page := historyManager.CurrentPage()
					row, _ := pageView.PageText.GetScrollOffset()
					current_page.ScrollOffset = row
					url := current_page.Links[i-1]
					page, success := GopherHandler(url)
					if !success {
						log.Println("Failed to get gopher url")
					}
					pageView.RenderPage(&page)
					historyManager.Navigate(&page)
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
