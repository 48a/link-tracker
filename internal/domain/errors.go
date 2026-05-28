package domain

type ChatNotExistError struct{}

func (ChatNotExistError) Error() string {
	return "chat doesn't exist"
}

type ChatAlreadyExistError struct{}

func (ChatAlreadyExistError) Error() string {
	return "chat already exist"
}

type AlreadyTrackingError struct{}

func (AlreadyTrackingError) Error() string {
	return "link already being tracked"
}

type ChatOrLinkNotFoundError struct{}

func (ChatOrLinkNotFoundError) Error() string {
	return "link already being tracked"
}
