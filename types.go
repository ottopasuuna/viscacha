package main

import (
	"github.com/prologic/go-gopher"
)

type ContentType int

const (
	TextType = iota
	GopherDirectory
	GopherQuery
	ImageType
	BinaryType
	HTMLType
	UnknownType
)

var Gopher_to_content_type = map[gopher.ItemType]ContentType{
	gopher.FILE:        TextType,
	gopher.DIRECTORY:   GopherDirectory,
	gopher.INDEXSEARCH: GopherQuery,
	gopher.GIF:         ImageType,
	gopher.IMAGE:       ImageType,
	gopher.PNG:         ImageType,
	gopher.DOSARCHIVE:  BinaryType,
	gopher.BINARY:      BinaryType,
	gopher.AUDIO:       BinaryType,
	gopher.DOC:         BinaryType,
	gopher.BINHEX:      BinaryType,
	// gopher.HTML:        HTMLType,
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
	Parent       *Page
	LinkIndex    int
}
