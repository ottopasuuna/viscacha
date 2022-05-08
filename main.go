package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/op/go-logging"
	"github.com/rivo/tview"
)

// ## Architecture
// A Handler is a function that takes a url and fetches the content. It returns a Page
// which contains all the relavent information. The browser history is a list of Pages.
// Various Render functions write the Page content to a tview TextView.

var AppLog = logging.MustGetLogger("viscacha")

var DefaultKeyBindings = map[string]string{
	"j":  "scroll-down",
	"k":  "scroll-up",
	"g":  "scroll-top",
	"G":  "scroll-bottom",
	"d":  "scroll-hpage-down",
	"u":  "scroll-hpage-up",
	"h":  "back",
	"l":  "forward",
	"\\": "show-logs",
	":":  "cmd-prompt",
}

const DEFAULT_LOG_PATH = "log.log"
const DEFAULT_CONFIG_PATH = "config.json"
const HOME_PAGE = "gopher://gopher.floodgap.com/"

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
	PageView          *PageView
	HistoryManager    *HistoryManager
	MessageLine       *tview.TextView
	App               *tview.Application
	GridLayout        *tview.Grid
	LogBuffer         strings.Builder
	cli_lock          sync.Mutex      // For ensuring only one MessageLine input field open at a time
	active_view       tview.Primitive // Keep track of the widget to give focus back to
	loadingLock       sync.Mutex
	commandNameToFunc map[string]func()
	keyBindings       map[string]string
}

func NewClient(userConfig UserConfig) *Client {
	app := tview.NewApplication()

	pageView := NewPageView()
	textView := pageView.PageText
	statusLine := pageView.StatusLine

	messageLine := tview.NewTextView().
		SetDynamicColors(true)
	messageLine.SetChangedFunc(func() {
		app.Draw()
	})
	messageLine.SetBackgroundColor(tcell.ColorDefault)

	gridLayout := tview.NewGrid().
		SetRows(0, 1, 1).
		SetColumns(0).
		SetBorders(false)

	gridLayout.AddItem(textView, 0, 0, 1, 1, 0, 0, true)
	gridLayout.AddItem(statusLine, 1, 0, 1, 1, 0, 0, false)
	gridLayout.AddItem(messageLine, 2, 0, 1, 1, 0, 0, false)

	// TODO: this makes it imposible to type the letter q in any text field...
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'q':
			app.Stop()
			return nil
		}
		return event
	})
	app.SetRoot(gridLayout, true).SetFocus(textView)
	app.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
		screen.Clear()
		return false
	})

	keyBindings := make(map[string]string)
	for key, command := range DefaultKeyBindings {
		keyBindings[key] = command
	}
	if userConfig.Bindings != nil {
		for key, command := range userConfig.Bindings {
			keyBindings[key] = command
		}
	}

	client := Client{
		PageView:       pageView,
		HistoryManager: &HistoryManager{},
		MessageLine:    messageLine,
		App:            app,
		GridLayout:     gridLayout,
		active_view:    pageView.PageText,
		keyBindings:    keyBindings,
	}
	client.initCommandNameMap()
	textView.SetInputCapture(client.PageInputHandler)
	return &client
}

// User configurable settings are stored in here
type UserConfig struct {
	Bindings map[string]string `json: bindings`
	HomePage string            `json: homepage`
}

func ReadConfig(path string) UserConfig {
	var userconfig UserConfig
	content, err := ioutil.ReadFile(path)
	if err != nil {
		AppLog.Errorf("Failed to read config file \"%s\"\n\t%v", path, err)
	}
	err = json.Unmarshal(content, &userconfig)
	if err != nil {
		AppLog.Errorf("Failed to parse config file \"%s\"\n\t%v", path, err)
	}
	return userconfig
}

func (c *Client) initCommandNameMap() {
	c.commandNameToFunc = map[string]func(){
		"scroll-up":         c.CommandScrollUp,
		"scroll-down":       c.CommandScrollDown,
		"scroll-top":        c.CommandScrollTop,
		"scroll-bottom":     c.CommandScrollBottom,
		"scroll-hpage-up":   c.CommandScrollHalfUp,
		"scroll-hpage-down": c.CommandScrollHalfDown,
		"back":              c.CommandBack,
		"forward":           c.CommandForward,
		"up":                c.CommandGoUp,
		"next":              c.CommandGoNext,
		"prev":              c.CommandGoPrev,
		"root":              c.CommandGoToRoot,
		"show-logs":         c.CommandViewLogs,
		"cmd-prompt":        c.CommandCmdPrompt,
	}
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
				c.App.SetFocus(c.active_view)
				c.cli_lock.Unlock()
			})
			c.GridLayout.RemoveItem(c.MessageLine)
			c.GridLayout.AddItem(commandLine, 2, 0, 1, 1, 0, 0, true)
			c.App.SetFocus(commandLine)
		})
	}()
}

func (client *Client) GotoUrl(url string) {
	client.SaveScroll()
	fmt.Fprintln(client.MessageLine, "Loading...")
	client.loadingLock.Lock()
	go func() {
		page, success := GopherHandler(url)
		if !success {
			AppLog.Error("Failed to get gopher url")
		} else if page != nil {
			client.App.QueueUpdateDraw(func() {
				client.PageView.RenderPage(page)
				client.HistoryManager.Navigate(page)
				client.MessageLine.Clear()
			})
		}
		client.loadingLock.Unlock()
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
	binding, is_bound := c.keyBindings[string(event.Rune())]
	if is_bound {
		cmd_func, is_cmd := c.commandNameToFunc[binding]
		if is_cmd {
			cmd_func()
			return nil
		} else {
			AppLog.Error("Not a valid command: \"%s\"", binding)
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
		go func() {
			c.loadingLock.Lock()
			c.loadingLock.Unlock()
			new_page := c.HistoryManager.CurrentPage()
			new_page.Parent = page
			new_page.LinkIndex = link_num
		}()
	} else {
		AppLog.Errorf("No link #%d on the current page", link_num)
	}
}

func (c *Client) CommandCmdPrompt() {
	c.BuildCommandLine(": ", func(commandLine *tview.InputField, key tcell.Key) {
		if key == tcell.KeyEnter {
			// Dispatch command
			commandString := commandLine.GetText()
			cmd := strings.Split(commandString, " ")[0]
			cmd_func, in_cmd_map := c.commandNameToFunc[cmd]
			if in_cmd_map {
				cmd_func()
			} else {
				if link_num, err := strconv.ParseInt(cmd, 10, 32); err == nil {
					current_page := c.HistoryManager.CurrentPage()
					c.FollowLink(current_page, int(link_num))
				} else if url, err := url.Parse(commandString); err == nil && url.Scheme != "" {
					switch url.Scheme {
					case "gopher":
						c.GotoUrl(commandString)
					default:
						AppLog.Errorf("Protocol \"%s\" not supported", url.Scheme)
					}
				} else {
					AppLog.Errorf("Not a valid command: \"%s\"", cmd)
				}
			}
		}
	})
}

func (c *Client) CommandScrollUp() {
	curr_row, _ := c.PageView.PageText.GetScrollOffset()
	scrollDest := curr_row - 1
	if scrollDest <= 0 {
		scrollDest = 0
	}
	c.PageView.PageText.ScrollTo(scrollDest, 0)
	c.PageView.UpdateStatus()
}

func (c *Client) CommandScrollDown() {
	curr_row, _ := c.PageView.PageText.GetScrollOffset()
	scrollDest := curr_row + 1
	bottom := c.PageView.NumLines()
	if scrollDest >= bottom {
		scrollDest = bottom
	}
	c.PageView.PageText.ScrollTo(scrollDest, 0)
	c.PageView.UpdateStatus()
}

func (c *Client) CommandScrollTop() {
	c.PageView.PageText.ScrollToBeginning()
	c.PageView.UpdateStatus()
}

func (c *Client) CommandScrollBottom() {
	c.PageView.PageText.ScrollToEnd()
	c.PageView.UpdateStatus()
}

func (c *Client) CommandScrollHalfDown() {
	_, _, _, height := c.PageView.PageText.GetRect()
	curr_row, _ := c.PageView.PageText.GetScrollOffset()
	scrollDest := curr_row + height/2
	bottom := c.PageView.NumLines()
	if scrollDest >= bottom {
		scrollDest = bottom
	}
	c.PageView.PageText.ScrollTo(scrollDest, 0)
	c.PageView.UpdateStatus()
}

func (c *Client) CommandScrollHalfUp() {
	_, _, _, height := c.PageView.PageText.GetRect()
	curr_row, _ := c.PageView.PageText.GetScrollOffset()
	scrollDest := curr_row - height/2
	if scrollDest <= 0 {
		scrollDest = 0
	}
	c.PageView.PageText.ScrollTo(scrollDest, 0)
	c.PageView.UpdateStatus()
}

func (c *Client) CommandBack() {
	c.SaveScroll()
	prev_page := c.HistoryManager.Back()
	if prev_page != nil {
		c.PageView.RenderPage(prev_page)
	} else {
		AppLog.Info("Already at first page")
	}
}

func (c *Client) CommandForward() {
	c.SaveScroll()
	next_page := c.HistoryManager.Forward()
	if next_page != nil {
		c.PageView.RenderPage(next_page)
	} else {
		AppLog.Info("Already at last page")
	}
}

func (c *Client) CommandViewLogs() {
	logView := tview.NewTextView().
		SetChangedFunc(func() {
			c.App.Draw()
		})
	logView.SetBorder(true)
	logView.SetTitle("Log Messages")
	logView.SetDynamicColors(true)
	logView.SetBackgroundColor(tcell.ColorDefault)
	logView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == '\\' || event.Key() == tcell.KeyEscape {
			c.App.SetRoot(c.GridLayout, true).SetFocus(c.PageView.PageText)
			c.active_view = c.PageView.PageText
			return nil
		}
		return event
	})
	fmt.Fprintf(tview.ANSIWriter(logView), c.LogBuffer.String())
	c.App.SetRoot(logView, true).SetFocus(logView)
	c.active_view = logView
}

func (c *Client) CommandGoToRoot() {
	cur_url := c.HistoryManager.CurrentPage().Url
	parsed_url, err := url.Parse(cur_url)
	if err != nil {
		AppLog.Error(err)
		return
	}
	root_url := fmt.Sprintf("%s://%s", parsed_url.Scheme, parsed_url.Host)
	c.GotoUrl(root_url)
}

func (c *Client) CommandGoNext() {
	cur_page := c.HistoryManager.CurrentPage()
	parent_page := cur_page.Parent
	next_index := cur_page.LinkIndex + 1
	if parent_page != nil && next_index <= len(parent_page.Links) {
		c.FollowLink(parent_page, next_index)
	} else {
		AppLog.Error("No next link in parent page to navigate to")
	}

}

func (c *Client) CommandGoPrev() {
	cur_page := c.HistoryManager.CurrentPage()
	parent_page := cur_page.Parent
	prev_index := cur_page.LinkIndex - 1
	if parent_page != nil && prev_index < 0 {
		c.FollowLink(parent_page, prev_index)
	} else {
		AppLog.Error("No previous link in parent page to navigate to")
	}
}

func GetUpUrl(url_str string) string {
	parsed_url, err := url.Parse(url_str)
	if err != nil {
		AppLog.Error(err)
		return ""
	}
	path := strings.Split(parsed_url.Path, "/")
	if len(path) <= 2 { // 2 because "/1" -> ["", "1"]
		return fmt.Sprintf("%s://%s", parsed_url.Scheme, parsed_url.Host)
	}
	up_path := path[1 : len(path)-1]
	up_path[0] = "1" // Assumes the parent page is a directory. Probably safe?
	up_url := fmt.Sprintf("%s://%s/%s", parsed_url.Scheme, parsed_url.Host, strings.Join(up_path, "/"))
	return up_url
}

func (c *Client) CommandGoUp() {
	cur_url := c.HistoryManager.CurrentPage().Url
	up_url := GetUpUrl(cur_url)
	c.GotoUrl(up_url)
}

func main() {
	// Parse cli arguments:
	flag.Parse()
	var init_url = flag.Arg(0)

	// Parse user config file
	userConfig := ReadConfig(DEFAULT_CONFIG_PATH)

	if init_url == "" {
		home_page := userConfig.HomePage
		if home_page == "" {
			home_page = HOME_PAGE
		}
		init_url = home_page
	}

	// Build tview Application UI
	client := NewClient(userConfig)

	// Setup log file handling
	logFile, err := os.Create(DEFAULT_LOG_PATH)
	if err != nil {
		AppLog.Error(err)
	} else {
		defer logFile.Close()
	}
	buffer_log_backend := logging.NewLogBackend(&client.LogBuffer, "", 0)
	msg_line_log_backend := logging.NewLogBackend(tview.ANSIWriter(client.MessageLine), "", 0)
	file_log_backend := logging.NewLogBackend(logFile, "", 0)
	verbose_log_format := logging.MustStringFormatter(
		`%{color}%{time:15:04:05} %{module} | %{shortfunc} ▶ %{level:.4s} %{id:03x}%{color:reset} %{message}`,
	)
	log_format := logging.MustStringFormatter(
		`%{color}%{time:15:04:05}| %{level:.4s}|%{color:reset} %{message}`,
	)
	msg_line_log_format := logging.MustStringFormatter(
		`%{color}%{message}%{color:reset}`,
	)
	fmt_msg_line_log_backend := logging.NewBackendFormatter(msg_line_log_backend, msg_line_log_format)
	fmt_file_log_backend := logging.NewBackendFormatter(file_log_backend, verbose_log_format)
	fmt_old_log_backend := logging.NewBackendFormatter(buffer_log_backend, log_format)
	logging.SetBackend(fmt_msg_line_log_backend, fmt_file_log_backend, fmt_old_log_backend)

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
