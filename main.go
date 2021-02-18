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
	"sync"
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
	PageView       *PageView
	HistoryManager *HistoryManager
	MessageLine    *tview.TextView
	App            *tview.Application
	GridLayout     *tview.Grid
	LogHandler     *LogMessageHandler
	cli_lock       sync.Mutex // For ensuring only one MessageLine input field open at a time
}

func NewClient() *Client {
	app := tview.NewApplication()

	pageView := NewPageView()
	textView := pageView.PageText
	statusLine := pageView.StatusLine

	messageLine := tview.NewTextView().
		SetDynamicColors(true)
	messageLine.SetChangedFunc(func() {
		app.Draw()
	})

	gridLayout := tview.NewGrid().
		SetRows(0, 1, 1).
		SetColumns(0).
		SetBorders(false)

	gridLayout.AddItem(textView, 0, 0, 1, 1, 0, 0, true)
	gridLayout.AddItem(statusLine, 1, 0, 1, 1, 0, 0, false)
	gridLayout.AddItem(messageLine, 2, 0, 1, 1, 0, 0, false)

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'q':
			app.Stop()
			return nil
		}
		return event
	})
	app.SetRoot(gridLayout, true).SetFocus(textView)

	client := Client{
		PageView:       pageView,
		HistoryManager: &HistoryManager{},
		MessageLine:    messageLine,
		App:            app,
		GridLayout:     gridLayout,
	}
	textView.SetInputCapture(client.PageInputHandler)
	return &client
}

func (c *Client) BuildCommandLine(label string, handler func(commandLine *tview.InputField, key tcell.Key)) {
	go func() {
		c.cli_lock.Lock()
		c.App.QueueUpdateDraw(func() {
			commandLine := tview.NewInputField().
				SetLabel(label)
			commandLine.SetDoneFunc(func(key tcell.Key) {
				handler(commandLine, key)
				c.GridLayout.RemoveItem(commandLine)
				c.GridLayout.AddItem(c.MessageLine, 2, 0, 1, 1, 0, 0, false)
				c.App.SetFocus(c.PageView.PageText)
			})
			c.GridLayout.RemoveItem(c.MessageLine)
			c.GridLayout.AddItem(commandLine, 2, 0, 1, 1, 0, 0, true)
			c.App.SetFocus(commandLine)
		})
		c.cli_lock.Unlock()
	}()
}

func (client *Client) GotoUrl(url string) {
	client.SaveScroll()
	fmt.Fprintln(client.MessageLine, "Loading...")
	go func() {
		page, success := GopherHandler(url)
		if !success {
			log.Println("Failed to get gopher url")
		}
		client.App.QueueUpdateDraw(func() {
			client.PageView.RenderPage(&page)
			client.HistoryManager.Navigate(&page)
			client.MessageLine.Clear()
		})
	}()
}

func (client *Client) SaveScroll() {
	page := client.HistoryManager.CurrentPage()
	if page != nil {
		row, _ := client.PageView.PageText.GetScrollOffset()
		page.ScrollOffset = row
	}
}

func (c *Client) PageInputHandler(event *tcell.EventKey) *tcell.EventKey {
	if c.MessageLine.GetText(true) != "Loading...\n" {
		c.MessageLine.Clear()
	}
	switch event.Rune() {
	case 'k':
		curr_row, _ := c.PageView.PageText.GetScrollOffset()
		scrollDest := curr_row - 1
		if scrollDest <= 0 {
			scrollDest = 0
		}
		c.PageView.PageText.ScrollTo(scrollDest, 0)
		c.PageView.UpdateStatus()
		return nil
	case 'j':
		curr_row, _ := c.PageView.PageText.GetScrollOffset()
		scrollDest := curr_row + 1
		bottom := c.PageView.NumLines()
		if scrollDest >= bottom {
			scrollDest = bottom
		}
		c.PageView.PageText.ScrollTo(scrollDest, 0)
		c.PageView.UpdateStatus()
		return nil
	case 'g':
		c.PageView.PageText.ScrollToBeginning()
		c.PageView.UpdateStatus()
		return nil
	case 'G':
		c.PageView.PageText.ScrollToEnd()
		c.PageView.UpdateStatus()
		return nil
	case 'd':
		_, _, _, height := c.PageView.PageText.GetRect()
		curr_row, _ := c.PageView.PageText.GetScrollOffset()
		scrollDest := curr_row + height/2
		bottom := c.PageView.NumLines()
		if scrollDest >= bottom {
			scrollDest = bottom
		}
		c.PageView.PageText.ScrollTo(scrollDest, 0)
		c.PageView.UpdateStatus()
		return nil
	case 'u':
		_, _, _, height := c.PageView.PageText.GetRect()
		curr_row, _ := c.PageView.PageText.GetScrollOffset()
		scrollDest := curr_row - height/2
		if scrollDest <= 0 {
			scrollDest = 0
		}
		c.PageView.PageText.ScrollTo(scrollDest, 0)
		c.PageView.UpdateStatus()
		return nil
	case 'h':
		c.SaveScroll()
		prev_page := c.HistoryManager.Back()
		if prev_page != nil {
			c.PageView.RenderPage(prev_page)
		} else {
			log.Println("Already at first page")
		}
		return nil
	case 'l':
		c.SaveScroll()
		next_page := c.HistoryManager.Forward()
		if next_page != nil {
			c.PageView.RenderPage(next_page)
		} else {
			log.Println("Already at last page")
		}
		return nil
	case ':':
		// Open command line
		c.BuildCommandLine(": ", func(commandLine *tview.InputField, key tcell.Key) {
			if key == tcell.KeyEnter {
				// Dispatch command
				commandString := commandLine.GetText()
				cmd := strings.Split(commandString, " ")[0]
				if link_num, err := strconv.ParseInt(cmd, 10, 32); err == nil {
					current_page := c.HistoryManager.CurrentPage()
					c.FollowLink(current_page, int(link_num))
				} else if url, err := url.Parse(commandString); err == nil {
					switch url.Scheme {
					case "gopher":
						c.GotoUrl(commandString)
					default:
						log.Printf("[red]protocol \"%s\" not supported\n", url.Scheme)
					}
				} else {
					switch cmd {
					default:
						log.Printf("[red]Not a valid command: \"%s\"[white]\n", cmd)
					}
				}
			}
		})
		return nil
	case '\\': // Log view page
		if c.LogHandler != nil {
			logView := tview.NewTextView().
				SetChangedFunc(func() {
					c.App.Draw()
				})
			logView.SetBorder(true)
			logView.SetTitle("Log Messages")
			logView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
				if event.Rune() == '\\' {
					c.App.SetRoot(c.GridLayout, true).SetFocus(c.PageView.PageText)
					return nil
				}
				return event
			})
			fmt.Fprintf(logView, c.LogHandler.Text)
			c.App.SetRoot(logView, true).SetFocus(logView)
			return nil
		}
	}

	// Bind number keys to quick select links
	for i := 1; i <= 9; i++ {
		if (event.Rune()) == rune(i+48) {
			current_page := c.HistoryManager.CurrentPage()
			c.FollowLink(current_page, i)
		}
	}
	return event
}

func (c *Client) FollowLink(page *Page, link_num int) {
	if link_num > 0 && int(link_num) <= len(page.Links) {
		link := page.Links[link_num-1]
		if link.Type == GopherQuery {
			// get input
			c.BuildCommandLine("Query: ", func(commandLine *tview.InputField, key tcell.Key) {
				search_term := commandLine.GetText()
				query_url, err := GopherQueryUrl(link, search_term)
				if err != nil {
					return
				}
				c.GotoUrl(query_url)
			})

		} else {
			c.GotoUrl(link.Url)
		}
	} else {
		log.Printf("[red]No link #%d on the current page[white]\n", link_num)
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
	if init_url == "" {
		init_url = HOME_PAGE
	}

	// Build tview Application UI
	client := NewClient()

	// Setup log file handling
	logFile, err := os.Create(DEFAULT_LOG_PATH)
	if err != nil {
		log.Println(err)
	} else {
		defer logFile.Close()
	}
	var logHandler = LogMessageHandler{
		MessageLine: client.MessageLine,
		Text:        "",
		LogFile:     logFile,
	}
	log.SetOutput(&logHandler)
	client.LogHandler = &logHandler

	// Go to a URL
	client.GotoUrl(init_url)
	time.AfterFunc(50*time.Millisecond, func() {
		// Hacks to get UpdateStatus to detect the correct terminal width on startup
		client.App.QueueUpdateDraw(func() {
			client.PageView.UpdateStatus()
		})
	})

	if err := client.App.Run(); err != nil {
		panic(err)
	}
}
