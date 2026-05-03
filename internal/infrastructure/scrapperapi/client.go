package scrapperapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"time"
)

type client struct {
	baseURL string
	cl      *http.Client
	timeout time.Duration
}

type responseData struct {
	body       []byte
	statusCode int
}

type headerField struct {
	key   string
	value string
}

func NewClient(url string, timeout time.Duration) client {
	return client{baseURL: url, cl: http.DefaultClient, timeout: timeout}
}

func (_ client) verifyResponse(linkResponse LinkResponse, id int64, link string, skipTags bool, tags []string, operation string) error {
	if linkResponse.ID != id {
		return ErrIdMismatch{operation: operation}
	}
	if linkResponse.URL != link {
		return ErrLinkMismatch{operation: operation}
	}
	if skipTags || slices.Compare(linkResponse.Tags, tags) != 0 {
		return ErrTagsMismatch{operation: operation}
	}
	return nil
}

func (c client) restApiRequest(operation, method, url string, headerFields []headerField, requestBody io.Reader) (responseData, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, url, requestBody)
	if err != nil {
		return responseData{}, ErrCantCreateRequest{operation: operation, wrapped: err}
	}

	for _, field := range headerFields {
		req.Header.Add(field.key, field.value)
	}

	resp, err := c.cl.Do(req)
	if errors.Is(err, context.DeadlineExceeded) {
		return responseData{}, ErrCantDoRequest{operation: operation, wrapped: ErrTimedOut{}}
	}
	if err != nil {
		return responseData{}, ErrCantDoRequest{operation: operation, wrapped: err}
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return responseData{}, ErrCantReadResponse{operation: operation, wrapped: err}
	}

	return responseData{body: b, statusCode: resp.StatusCode}, nil
}

func (c client) RegisterChat(id int64) error {
	operation := fmt.Sprintf("register chat %v", id)
	resp, err := c.restApiRequest(
		operation,
		"POST",
		c.baseURL+"/tg-chat/"+strconv.FormatInt(id, 10),
		[]headerField{},
		nil,
	)
	if err != nil {
		return err
	}

	if resp.statusCode == 200 {
		return nil
	}

	var responseError ApiErrorResponse
	err = json.Unmarshal(resp.body, &responseError)
	if err != nil {
		return ErrCantUnmarshalResponse{operation: operation, wrapped: err}
	}

	switch resp.statusCode {
	case 400:
		return NewApiError(400, responseError, operation)
	case 409:
		return NewApiError(409, responseError, operation)
	}
	return ErrUnknownStatusCode{operation: operation}
}

func (c client) DeleteChat(id int64) error {
	operation := fmt.Sprintf("delete chat %v", id)
	resp, err := c.restApiRequest(
		operation,
		"DELETE",
		c.baseURL+"/tg-chat/"+strconv.FormatInt(id, 10),
		[]headerField{},
		nil,
	)
	if err != nil {
		return err
	}

	if resp.statusCode == 200 {
		return nil
	}

	var responseError ApiErrorResponse
	err = json.Unmarshal(resp.body, &responseError)
	if err != nil {
		return ErrCantUnmarshalResponse{operation: operation, wrapped: err}
	}

	switch resp.statusCode {
	case 400:
		return NewApiError(400, responseError, operation)
	case 404:
		return NewApiError(404, responseError, operation)
	}
	return ErrUnknownStatusCode{operation: operation}
}

func (c client) GetLinks(id int64) (ListLinksResponse, error) {
	operation := fmt.Sprintf("get links %v", id)
	resp, err := c.restApiRequest(
		operation,
		"GET",
		c.baseURL+"/links",
		[]headerField{
			headerField{
				key: "Tg-Chat-Id", value: strconv.FormatInt(id, 10),
			},
		},
		nil,
	)
	if err != nil {
		return ListLinksResponse{}, err
	}

	if resp.statusCode == 200 {
		var result ListLinksResponse
		err = json.Unmarshal(resp.body, &result)
		if err != nil {
			return ListLinksResponse{}, ErrCantUnmarshalResponse{operation: operation, wrapped: err}
		}
		return result, nil
	}

	var responseError ApiErrorResponse
	err = json.Unmarshal(resp.body, &responseError)
	if err != nil {
		return ListLinksResponse{}, ErrCantUnmarshalResponse{operation: operation, wrapped: err}
	}

	switch resp.statusCode {
	case 400:
		return ListLinksResponse{}, NewApiError(400, responseError, operation)
	case 404:
		return ListLinksResponse{}, NewApiError(404, responseError, operation)
	}
	return ListLinksResponse{}, ErrUnknownStatusCode{operation: operation}
}

func (c client) AddLink(id int64, addRequest AddLinkRequest) error {
	operation := fmt.Sprintf("add link %#v to %v", addRequest, id)

	body, err := json.Marshal(addRequest)
	if err != nil {
		return ErrCantMarshalRequest{operation: operation, wrapped: err}
	}

	resp, err := c.restApiRequest(
		operation,
		"POST",
		c.baseURL+"/links",
		[]headerField{
			headerField{
				key: "Tg-Chat-Id", value: strconv.FormatInt(id, 10),
			},
		},
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}

	if resp.statusCode == 200 {
		var linkResponse LinkResponse
		err = json.Unmarshal(resp.body, &linkResponse)
		if err != nil {
			return ErrCantUnmarshalResponse{operation: operation, wrapped: err}
		}
		return c.verifyResponse(linkResponse, id, addRequest.URL, false, addRequest.Tags, operation)
	}

	var responseError ApiErrorResponse
	err = json.Unmarshal(resp.body, &responseError)
	if err != nil {
		return ErrCantUnmarshalResponse{operation: operation, wrapped: err}
	}

	switch resp.statusCode {
	case 400:
		return NewApiError(400, responseError, operation)
	case 404:
		return NewApiError(404, responseError, operation)
	case 409:
		return NewApiError(409, responseError, operation)
	}
	return ErrUnknownStatusCode{operation: operation}
}

func (c client) DeleteLink(id int64, deleteRequest DeleteLinkRequest) error {
	operation := fmt.Sprintf("delete link %#v to %v", deleteRequest, id)

	body, err := json.Marshal(deleteRequest)
	if err != nil {
		return ErrCantMarshalRequest{operation: operation, wrapped: err}
	}

	resp, err := c.restApiRequest(
		operation,
		"DELETE",
		c.baseURL+"/links",
		[]headerField{
			headerField{
				key: "Tg-Chat-Id", value: strconv.FormatInt(id, 10),
			},
		},
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}

	if resp.statusCode == 200 {
		var linkResponse LinkResponse
		err = json.Unmarshal(resp.body, &linkResponse)
		if err != nil {
			return ErrCantUnmarshalResponse{operation: operation, wrapped: err}
		}
		return c.verifyResponse(linkResponse, id, deleteRequest.URL, true, []string{}, operation)
	}

	var responseError ApiErrorResponse
	err = json.Unmarshal(resp.body, &responseError)
	if err != nil {
		return ErrCantUnmarshalResponse{operation: operation, wrapped: err}
	}

	switch resp.statusCode {
	case 400:
		return NewApiError(400, responseError, operation)
	case 404:
		return NewApiError(404, responseError, operation)
	}
	return ErrUnknownStatusCode{operation: operation}
}
