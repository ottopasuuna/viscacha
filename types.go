package main

import (
	"github.com/prologic/go-gopher"
)

type ContentType int

const (
	TextType = iota
	GopherDirectory
	GopherQuery
	UnknownType
)

var Gopher_to_content_type = map[gopher.ItemType]ContentType{
	gopher.FILE:        TextType,
	gopher.DIRECTORY:   GopherDirectory,
	gopher.INDEXSEARCH: GopherQuery,
}

type Link struct {
	Type ContentType
	Url  string
}

type Page struct {
	Type         ContentType
	Url          string
	Content      string
	Links        []*Link
	ScrollOffset int
}
