package main

import (
	"fmt"
	"log"
	"math"
	"strings"

	"github.com/prologic/go-gopher"
	"github.com/rivo/tview"
)

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
	fmt.Fprintf(pageview.StatusLine, page.Url)
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
		default:
			txt_color = "[red]"
		}
		fmt.Fprintf(textview, "%s%s\n[white]", txt_color, item.Description)
	}
	pageview.PageText.ScrollTo(page.ScrollOffset, 0)
}
