package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
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
	if len(manager.page_history) == 0 {
		return nil
	}
	return manager.page_history[manager.history_index]
}

type Client struct {
	pageView       *PageView
	historyManager *HistoryManager
}

func (client *Client) GotoUrl(url string) {
	client.SaveScroll()
	page, success := GopherHandler(url)
	if !success {
		log.Println("Failed to get gopher url")
	}
	client.pageView.RenderPage(&page)
	client.historyManager.Navigate(&page)
}

func (client *Client) SaveScroll() {
	page := client.historyManager.CurrentPage()
	if page != nil {
		row, _ := client.pageView.PageText.GetScrollOffset()
		page.ScrollOffset = row
	}
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
	var init_url = flag.Arg(0)
	fmt.Println(init_url)
	if init_url == "" {
		init_url = HOME_PAGE
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

	messageLine := tview.NewTextView().
		SetDynamicColors(true)
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

	BuildCommandLine := func(label string, handler func(commandLine *tview.InputField, key tcell.Key)) {
		commandLine := tview.NewInputField().
			SetLabel(label)
		commandLine.SetDoneFunc(func(key tcell.Key) {
			handler(commandLine, key)
			grid_layout.RemoveItem(commandLine)
			grid_layout.AddItem(messageLine, 2, 0, 1, 1, 0, 0, false)
			app.SetFocus(textView)
		})
		grid_layout.RemoveItem(messageLine)
		grid_layout.AddItem(commandLine, 2, 0, 1, 1, 0, 0, true)
		app.SetFocus(commandLine)
	}

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
	historyManager := HistoryManager{}

	client := Client{
		pageView:       &pageView,
		historyManager: &historyManager,
	}

	client.GotoUrl(init_url)

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
			client.SaveScroll()
			prev_page := historyManager.Back()
			if prev_page != nil {
				pageView.RenderPage(prev_page)
			} else {
				log.Println("Already at first page")
			}
			return nil
		case 'l':
			client.SaveScroll()
			next_page := historyManager.Forward()
			if next_page != nil {
				pageView.RenderPage(next_page)
			} else {
				log.Println("Already at last page")
			}
			return nil
		case ':':
			// Open command line
			BuildCommandLine(": ", func(commandLine *tview.InputField, key tcell.Key) {
				if key == tcell.KeyEnter {
					// Dispatch command
					commandString := commandLine.GetText()
					cmd := strings.Split(commandString, " ")[0]
					if link_num, err := strconv.ParseInt(cmd, 10, 32); err == nil {
						current_page := historyManager.CurrentPage()
						url := current_page.Links[link_num-1].Url
						client.GotoUrl(url)
					} else {
						if url, err := url.Parse(commandString); err == nil {
							switch url.Scheme {
							case "gopher":
								client.GotoUrl(commandString)
							default:
							}
						} else {
							switch cmd {
							default:
								log.Printf("[red]Not a valid command: \"%s\"[white]\n", cmd)
							}
						}
					}
				}
			})
			return nil
		case '\\': // Log view page
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
				current_page := historyManager.CurrentPage()
				if len(current_page.Links) >= i {
					link := current_page.Links[i-1]
					if link.Type == GopherQuery {
						// get input
						BuildCommandLine("Query: ", func(commandLine *tview.InputField, key tcell.Key) {
							// This is pretty gross...
							search_term := commandLine.GetText()
							link_url, err := url.Parse(link.Url)
							if err != nil {
								return
							}
							path := "/1/" + link_url.Path[3:]
							query_url := fmt.Sprintf("%s://%s%s%%09%s",
								link_url.Scheme, link_url.Host, path, search_term)
							client.GotoUrl(query_url)
						})

					} else {
						client.GotoUrl(link.Url)
					}
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
