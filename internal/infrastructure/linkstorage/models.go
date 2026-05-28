package linkstorage

import "time"

type AddLinkInput struct {
	URL         string
	Tags        []string
	LastUpdated time.Time
}

type DeleteLinkInput struct {
	URL string
}

type Link struct {
	LinkID      int
	ChatID      int64
	URL         string
	Tags        []string
	LastUpdated time.Time
}
