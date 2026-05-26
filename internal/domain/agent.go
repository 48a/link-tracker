package domain

type RawUpdate struct {
	ID          int64   `json:"id"`
	Description string  `json:"description"`
	Author      string  `json:"author"`
	TgChatIDs   []int64 `json:"tgChatIds"`
}

type ProcessedUpdate struct {
	ID          int64   `json:"id"`
	Description string  `json:"description"`
	TgChatIDs   []int64 `json:"tgChatIds"`
	Priority    string  `json:"priority"`
}
