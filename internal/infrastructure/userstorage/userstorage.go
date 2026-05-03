package userstorage

type Request struct {
	URL  string
	Tags []string
}

type userStorage struct {
	chatState    map[int64]int
	requestState map[int64]Request
}

func NewUserStorage() *userStorage {
	return &userStorage{chatState: make(map[int64]int), requestState: make(map[int64]Request)}
}

func (u *userStorage) GetUserState(userId int64) (int, bool) {
	value, ok := u.chatState[userId]
	return value, ok
}

func (u *userStorage) SetUserState(userId int64, newState int) {
	u.chatState[userId] = newState
}

func (u *userStorage) SetRequestURL(userId int64, URL string) {
	value := u.requestState[userId]
	value.URL = URL
	u.requestState[userId] = value
}

func (u *userStorage) SetRequestTags(userId int64, Tags []string) {
	value := u.requestState[userId]
	value.Tags = Tags
	u.requestState[userId] = value
}

func (u *userStorage) GetRequest(chatId int64) Request {
	return u.requestState[chatId]
}
