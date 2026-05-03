package scrapperapi

import (
	"fmt"
)

type ErrTimedOut struct{}

func (ErrTimedOut) Error() string {
	return "request timed out"
}

type ErrUnknownStatusCode struct {
	operation string
}

func (e ErrUnknownStatusCode) Error() string {
	return e.operation + ": unknown status code received"
}

type ErrCantCreateRequest struct {
	operation string
	wrapped   error
}

func (e ErrCantCreateRequest) Error() string {
	return e.operation + ": can't create request"
}

func (e ErrCantCreateRequest) Unwrap() error {
	return e.wrapped
}

type ErrCantDoRequest struct {
	operation string
	wrapped   error
}

func (e ErrCantDoRequest) Error() string {
	return e.operation + ": can't do request"
}

func (e ErrCantDoRequest) Unwrap() error {
	return e.wrapped
}

type ErrCantUnmarshalResponse struct {
	operation string
	wrapped   error
}

func (e ErrCantUnmarshalResponse) Error() string {
	return e.operation + ": can't unmarshal response body"
}

func (e ErrCantUnmarshalResponse) Unwrap() error {
	return e.wrapped
}

type ErrCantReadResponse struct {
	operation string
	wrapped   error
}

func (e ErrCantReadResponse) Error() string {
	return "cant read response body"
}

func (e ErrCantReadResponse) Unwrap() error {
	return e.wrapped
}

type ErrCantMarshalRequest struct {
	operation string
	wrapped   error
}

func (e ErrCantMarshalRequest) Error() string {
	return e.operation + ": can't marshal request"
}

func (e ErrCantMarshalRequest) Unwrap() error {
	return e.wrapped
}

type ErrIdMismatch struct {
	operation string
}

func (e ErrIdMismatch) Error() string {
	return e.operation + ": response chat id mismatch"
}

type ErrLinkMismatch struct {
	operation string
}

func (e ErrLinkMismatch) Error() string {
	return e.operation + ": response link mismatch"
}

type ErrTagsMismatch struct {
	operation string
}

func (e ErrTagsMismatch) Error() string {
	return e.operation + ": response tags mismatch"
}

type ApiError struct {
	operation     string
	StatusCode    int
	ErrorResponse ApiErrorResponse
}

func NewApiError(statusCode int, errorResponse ApiErrorResponse, operation string) ApiError {
	return ApiError{StatusCode: statusCode, ErrorResponse: errorResponse, operation: operation}
}

func (a ApiError) Error() string {
	return fmt.Sprintf("%s: received status code %v and response %#v", a.operation, a.StatusCode, a.ErrorResponse)
}
