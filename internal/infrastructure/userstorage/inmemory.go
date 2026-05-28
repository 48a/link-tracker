package userstorage

type UserStorage struct {
	chatState    map[int64]int
	requestState map[int64]Request
}

func NewUserStorage() *UserStorage {
	return &UserStorage{chatState: make(map[int64]int), requestState: make(map[int64]Request)}
}

func (u *UserStorage) GetUserState(userID int64) (int, bool) {
	value, ok := u.chatState[userID]
	return value, ok
}

func (u *UserStorage) SetUserState(userID int64, newState int) {
	u.chatState[userID] = newState
}

func (u *UserStorage) SetRequestURL(userID int64, url string) {
	value := u.requestState[userID]
	value.URL = url
	u.requestState[userID] = value
}

func (u *UserStorage) SetRequestTags(userID int64, tags []string) {
	value := u.requestState[userID]
	value.Tags = tags
	u.requestState[userID] = value
}

func (u *UserStorage) GetRequest(chatID int64) Request {
	return u.requestState[chatID]
}
