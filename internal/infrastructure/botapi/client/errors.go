package client

import (
	"fmt"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/api/botapi"
)

type TimedoutError struct{}

func (TimedoutError) Error() string {
	return "request timed out"
}

type UnknownStatusCodeError struct {
	operation string
}

func (e UnknownStatusCodeError) Error() string {
	return e.operation + ": unknown status code received"
}

type CreateRequestError struct {
	operation string
	wrapped   error
}

func (e CreateRequestError) Error() string {
	return e.operation + ": can't create request"
}

func (e CreateRequestError) Unwrap() error {
	return e.wrapped
}

type DoRequestError struct {
	operation string
	wrapped   error
}

func (e DoRequestError) Error() string {
	return e.operation + ": can't do request"
}

func (e DoRequestError) Unwrap() error {
	return e.wrapped
}

type UnmarshalResponseError struct {
	operation string
	wrapped   error
}

func (e UnmarshalResponseError) Error() string {
	return e.operation + ": can't unmarshal response body"
}

func (e UnmarshalResponseError) Unwrap() error {
	return e.wrapped
}

type ReadResponseError struct {
	operation string
	wrapped   error
}

func (e ReadResponseError) Error() string {
	return "cant read response body"
}

func (e ReadResponseError) Unwrap() error {
	return e.wrapped
}

type MarshalRequestError struct {
	operation string
	wrapped   error
}

func (e MarshalRequestError) Error() string {
	return e.operation + ": can't marshal request"
}

func (e MarshalRequestError) Unwrap() error {
	return e.wrapped
}

type IDMismatchError struct {
	operation string
}

func (e IDMismatchError) Error() string {
	return e.operation + ": response chat id mismatch"
}

type LinkMismatchError struct {
	operation string
}

func (e LinkMismatchError) Error() string {
	return e.operation + ": response link mismatch"
}

type TagsMismatchError struct {
	operation string
}

func (e TagsMismatchError) Error() string {
	return e.operation + ": response tags mismatch"
}

type APIError struct {
	operation     string
	StatusCode    int
	ErrorResponse botapi.APIErrorResponse
}

func NewAPIError(statusCode int, errorResponse botapi.APIErrorResponse, operation string) APIError {
	return APIError{StatusCode: statusCode, ErrorResponse: errorResponse, operation: operation}
}

func (a APIError) Error() string {
	return fmt.Sprintf("%s: received status code %v and response %#v", a.operation, a.StatusCode, a.ErrorResponse)
}
