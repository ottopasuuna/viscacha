package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/url"

	"github.com/prologic/go-gopher"
)

func GopherHandler(url string) (Page, bool) {
	res, err := gopher.Get(url)
	if err != nil {
		log.Println(err)
		return Page{}, false
	}
	content_type, ok := Gopher_to_content_type[res.Type]
	if !ok {
		log.Println("Unrecognized gopher file type")
		return Page{}, false
	}
	var content string
	var links []*Link
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

func GopherQueryUrl(link *Link, search_term string) (string, error) {
	// This is pretty gross...
	link_url, err := url.Parse(link.Url)
	if err != nil {
		return "", err
	}
	path := "/1/" + link_url.Path[3:]
	query_url := fmt.Sprintf("%s://%s%s%%09%s",
		link_url.Scheme, link_url.Host, path, search_term)
	return query_url, nil
}

func gopherItemToUrl(item *gopher.Item) string {
	url := fmt.Sprintf("gopher://%s:%d/%s%s", item.Host, item.Port, string(item.Type), item.Selector)
	return url
}

func gopherMakeLinkMap(dir *gopher.Directory) []*Link {
	var link_map []*Link
	for _, item := range dir.Items {
		if item.Type != gopher.INFO {
			content_type, ok := Gopher_to_content_type[item.Type]
			if !ok {
				content_type = UnknownType
			}
			link_map = append(link_map, &Link{Type: content_type,
				Url: gopherItemToUrl(item)})
		}
	}
	return link_map
}
