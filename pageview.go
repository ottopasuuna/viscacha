package main

import (
	"fmt"
	"log"
	"math"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/prologic/go-gopher"
	"github.com/rivo/tview"
)

type PageView struct {
	PageText   *tview.TextView
	StatusLine *tview.TextView
	currentUrl string
}

func NewPageView() *PageView {
	textView := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWordWrap(true)
	textView.SetBorder(false)

	statusLine := tview.NewTextView()
	statusLine.SetTextColor(tcell.GetColor("black"))
	statusLine.SetBackgroundColor(tcell.GetColor("white"))
	pageview := &PageView{
		PageText:   textView,
		StatusLine: statusLine,
	}
	return pageview
}

func (pageview *PageView) getPercentScroll() float64 {
	_, _, _, height := pageview.PageText.GetRect()
	row, _ := pageview.PageText.GetScrollOffset()
	viewBottom := row + height
	numLines := len(strings.Split(pageview.PageText.GetText(true), "\n"))
	percentViewed := math.Min(1.0, float64(viewBottom)/float64(numLines))
	return percentViewed * 100
}

func (p *PageView) UpdateStatus() {
	p.StatusLine.Clear()
	pctString := p.getPercentScroll()
	_, _, width, _ := p.StatusLine.GetRect()
	available_for_url := width - 5
	urlString := p.currentUrl
	if len(urlString) > available_for_url {
		urlString = urlString[:available_for_url]
	}
	padding := strings.Repeat(" ", available_for_url-len(urlString))
	fmt.Fprintf(p.StatusLine, "%s%s %3d%%", urlString, padding, int(pctString))
}

func (pageview *PageView) Clear() {
	pageview.PageText.Clear()
	pageview.StatusLine.Clear()
}

func (pageview *PageView) RenderPage(page *Page) {
	pageview.Clear()
	pageview.currentUrl = page.Url
	switch page.Type {
	case TextType:
		pageview.RenderTextFile(page)
	case GopherDirectory:
		pageview.RenderGopherDirectory(page)
	default:
		fmt.Fprintf(pageview.PageText, "[red] page type not recognized \"%d\"[white]", page.Type)
		log.Printf("[red] page type not recognized \"%d\"[white]\n", page.Type)
	}
	pageview.UpdateStatus()
}

func (pageview *PageView) RenderTextFile(page *Page) {
	content := strings.ReplaceAll(page.Content, "%", "%%")
	content = tview.Escape(content)
	fmt.Fprintf(pageview.PageText, content)
	pageview.PageText.ScrollTo(page.ScrollOffset, 0)
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
		case gopher.INFO:
			txt_color = "[white]"
		case gopher.FILE:
			txt_color = "[white]"
		case gopher.DIRECTORY:
			txt_color = "[skyblue]"
		case gopher.INDEXSEARCH:
			txt_color = "[violet]"
		default:
			txt_color = "[red]"
		}
		fmt.Fprintf(textview, "%s%s\n[white]", txt_color, tview.Escape(item.Description))
	}
	pageview.PageText.ScrollTo(page.ScrollOffset, 0)
}
