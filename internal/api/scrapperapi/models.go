package scrapperapi

type LinkResponse struct {
	ID   int64    `json:"id"`
	URL  string   `json:"url"`
	Tags []string `json:"tags"`
}

type ListLinksResponse struct {
	Links []LinkResponse `json:"links"`
	Size  int            `json:"size"`
}

type ApiErrorResponse struct {
	Description      string   `json:"description"`
	Code             string   `json:"code"`
	ExceptionName    string   `json:"exceptionName"`
	ExceptionMessage string   `json:"exceptionMessage"`
	Stacktrace       []string `json:"stacktrace"`
}

type AddLinkRequest struct {
	URL  string   `json:"link"`
	Tags []string `json:"tags"`
}

type DeleteLinkRequest struct {
	URL string `json:"link"`
}
