package scrapper

import "errors"

var (
	ErrNoSuchJob   = errors.New("no such job")
	ErrCantLoadJob = errors.New("can't load job")
)
