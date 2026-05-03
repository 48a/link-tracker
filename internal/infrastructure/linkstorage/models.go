package linkstorage

type AddLinkInput struct {
	URL  string
	Tags []string
}

type DeleteLinkInput struct {
	URL string
}
