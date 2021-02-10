package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"os"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/prologic/go-gopher"
	"github.com/rivo/tview"
)

// ## Architecture
// A Handler is a function that takes a url and fetches the content. It returns a Page
// which contains all the relavent information. The browser history is a list of Pages.
// Various Render functions write the Page content to a tview TextView.

type ContentType int

const (
	TextType = iota
	GopherDirectory
)

var gopher_to_content_type = map[gopher.ItemType]ContentType{
	gopher.FILE:      TextType,
	gopher.DIRECTORY: GopherDirectory,
}

type Page struct {
	Type    ContentType
	Url     string
	Content string
	Links   []string
}

func GopherHandler(url string) (Page, bool) {
	res, err := gopher.Get(url)
	if err != nil {
		log.Println(err)
		return Page{}, false
	}
	content_type, ok := gopher_to_content_type[res.Type]
	if !ok {
		log.Println("Unrecognized gopher file type")
		return Page{}, false
	}
	var content string
	var links []string
	if content_type == TextType {
		body_txt, err := ioutil.ReadAll(res.Body)
		if err != nil {
			log.Println("Failed to read file body")
			log.Println(err)
			return Page{}, false
		}
		content = string(body_txt)
	} else if content_type == GopherDirectory {
		dir_txt, err := res.Dir.ToText()
		if err != nil {
			log.Println("Error converting GopherDirectory to text:")
			log.Println(err)
			return Page{}, false
		}
		content = string(dir_txt)
		links = gopherMakeLinkMap(&res.Dir)
	}
	return Page{
		Type:    content_type,
		Url:     url,
		Content: content,
		Links:   links,
	}, true
}

func gopherItemToUrl(item *gopher.Item) string {
	url := fmt.Sprintf("gopher://%s:%d/%s%s", item.Host, item.Port, string(item.Type), item.Selector)
	return url
}

func gopherMakeLinkMap(dir *gopher.Directory) []string {
	var link_map []string
	for _, item := range dir.Items {
		if item.Type != gopher.INFO {
			link_map = append(link_map, gopherItemToUrl(item))
		}
	}
	return link_map
}

type PageView struct {
	PageText   *tview.TextView
	StatusLine *tview.TextView
}

func (pageview *PageView) Clear() {
	pageview.PageText.Clear()
	pageview.StatusLine.Clear()
}

func (pageview *PageView) RenderPage(page *Page) {
	pageview.Clear()
	switch page.Type {
	case TextType:
		pageview.RenderTextFile(page)
	case GopherDirectory:
		pageview.RenderGopherDirectory(page)
	default:
		fmt.Fprintf(pageview.PageText, "[red] page type not recognized \"%d\"[white]", page.Type)
		log.Printf("[red] page type not recognized \"%d\"[white]\n", page.Type)
	}
}

func (pageview *PageView) RenderTextFile(page *Page) {
	fmt.Fprintf(pageview.PageText, page.Content)
}

func (pageview *PageView) RenderGopherDirectory(page *Page) {
	textview := pageview.PageText
	link_counter := 1
	n_link_digits := int(math.Max(math.Log10(float64(len(page.Links))), 0)) + 1
	link_format := fmt.Sprintf("[green]%%s [%%%dd][white] ", n_link_digits)
	for _, line := range strings.Split(page.Content, "\n") {
		item, err := gopher.ParseItem(line)
		if err != nil {
			fmt.Fprintln(textview)
			continue
		}
		if item.Type != gopher.INFO {
			fmt.Fprintf(textview, link_format, item.Type.String(), link_counter)
			link_counter += 1
		} else {
			fmt.Fprintf(textview, strings.Repeat(" ", 3+1+2+n_link_digits+1))
		}
		var txt_color string
		switch item.Type {
		case gopher.DIRECTORY:
			txt_color = "[yellow]"
		default:
			txt_color = "[white]"
		}
		fmt.Fprintf(textview, "%s%s\n[white]", txt_color, item.Description)

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
