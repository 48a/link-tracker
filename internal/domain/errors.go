package domain

type ErrChatNotExist struct{}

func (ErrChatNotExist) Error() string {
	return "chat doesn't exist"
}

type ErrChatAlreadyExist struct{}

func (ErrChatAlreadyExist) Error() string {
	return "chat already exist"
}

type ErrAlreadyTracking struct{}

func (ErrAlreadyTracking) Error() string {
	return "link already being tracked"
}

type ErrChatOrLinkNotFound struct{}

func (ErrChatOrLinkNotFound) Error() string {
	return "link already being tracked"
}
