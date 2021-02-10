package main

import (
	"github.com/prologic/go-gopher"
)

type ContentType int

const (
	TextType = iota
	GopherDirectory
)

var Gopher_to_content_type = map[gopher.ItemType]ContentType{
	gopher.FILE:      TextType,
	gopher.DIRECTORY: GopherDirectory,
}

type Page struct {
	Type    ContentType
	Url     string
	Content string
	Links   []string
}
