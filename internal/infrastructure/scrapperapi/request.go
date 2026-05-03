package scrapperapi

type AddLinkRequest struct {
	URL  string   `json:"link"`
	Tags []string `json:"tags"`
}

type DeleteLinkRequest struct {
	URL string `json:"link"`
}
