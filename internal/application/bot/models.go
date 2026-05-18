package bot

type SendUpdateInput struct {
	ID          int64
	URL         string
	Description string
	TgChatIDs   []int64
}
