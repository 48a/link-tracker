package client

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

	"github.com/avast/retry-go/v4"
	"github.com/sony/gobreaker/v2"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/api/scrapperapi"
)

type client struct {
	baseURL string
	cl      *http.Client
	timeout time.Duration
	cfg     Config
	cb      *gobreaker.CircuitBreaker[responseData]
}

type responseData struct {
	body       []byte
	statusCode int
}

type headerField struct {
	key   string
	value string
}

func NewClient(cfg Config) client {
	cbSettings := gobreaker.Settings{
		Name:        "scrapper-client",
		MaxRequests: cfg.CBMinRequests,
		Interval:    0,
		Timeout:     cfg.CBOpenWindow,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= cfg.CBMinRequests && failureRatio >= cfg.CBRatioThreshold
		},
	}

	return client{
		baseURL: cfg.BaseURL,
		cl:      http.DefaultClient,
		timeout: cfg.Timeout,
		cfg:     cfg,
		cb:      gobreaker.NewCircuitBreaker[responseData](cbSettings),
	}
}

func (_ client) verifyResponse(linkResponse scrapperapi.LinkResponse, link string, skipTags bool, tags []string, operation string) error {
	if linkResponse.URL != link {
		return ErrLinkMismatch{operation: operation}
	}
	if !skipTags && slices.Compare(linkResponse.Tags, tags) != 0 {
		return ErrTagsMismatch{operation: operation}
	}
	return nil
}

func isRetryable(statusCode int) bool {
	return statusCode >= 500 && statusCode <= 599
}

func (c client) restApiRequest(ctx context.Context, operation, method, url string, headerFields []headerField, requestBody []byte) (responseData, error) {
	resp, cbErr := c.cb.Execute(func() (responseData, error) {
		var lastResp responseData

		err := retry.Do(
			func() error {
				reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
				defer cancel()

				var bodyReader io.Reader
				if requestBody != nil {
					bodyReader = bytes.NewReader(requestBody)
				}

				req, err := http.NewRequestWithContext(reqCtx, method, url, bodyReader)
				if err != nil {
					return retry.Unrecoverable(ErrCantCreateRequest{operation: operation, wrapped: err})
				}

				for _, field := range headerFields {
					req.Header.Add(field.key, field.value)
				}

				res, err := c.cl.Do(req)
				if err != nil {
					if errors.Is(err, context.DeadlineExceeded) {
						return ErrCantDoRequest{operation: operation, wrapped: ErrTimedOut{}}
					}
					return err
				}
				defer res.Body.Close()

				b, err := io.ReadAll(res.Body)
				if err != nil {
					return retry.Unrecoverable(ErrCantReadResponse{operation: operation, wrapped: err})
				}

				lastResp = responseData{body: b, statusCode: res.StatusCode}

				if !isRetryable(res.StatusCode) {
					return nil
				}

				return fmt.Errorf("server responded with retryable status: %d", res.StatusCode)
			},
			retry.Context(ctx),
			retry.Attempts(c.cfg.RetryAttempts),
			retry.Delay(c.cfg.RetryDelay),
			retry.DelayType(retry.FixedDelay),
		)

		return lastResp, err
	})

	if cbErr != nil {
		if errors.Is(cbErr, gobreaker.ErrOpenState) || errors.Is(cbErr, gobreaker.ErrTooManyRequests) {
			return c.fallbackResponse()
		}
		return responseData{}, cbErr
	}

	return resp, nil
}

func (c client) fallbackResponse() (responseData, error) {
	return responseData{
		body:       []byte(`{"description":"service temporarily unavailable (Circuit Breaker OPEN)","code":"503"}`),
		statusCode: http.StatusServiceUnavailable,
	}, nil
}

func (c client) RegisterChat(ctx context.Context, id int64) error {
	operation := fmt.Sprintf("register chat %v", id)
	resp, err := c.restApiRequest(
		ctx,
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

	var responseError scrapperapi.ApiErrorResponse
	err = json.Unmarshal(resp.body, &responseError)
	if err != nil {
		return ErrCantUnmarshalResponse{operation: operation, wrapped: err}
	}

	switch resp.statusCode {
	case 400:
		return NewApiError(400, responseError, operation)
	case 409:
		return NewApiError(409, responseError, operation)
	case 503:
		return NewApiError(503, responseError, operation)
	}
	return ErrUnknownStatusCode{operation: operation}
}

func (c client) DeleteChat(ctx context.Context, id int64) error {
	operation := fmt.Sprintf("delete chat %v", id)
	resp, err := c.restApiRequest(
		ctx,
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

	var responseError scrapperapi.ApiErrorResponse
	err = json.Unmarshal(resp.body, &responseError)
	if err != nil {
		return ErrCantUnmarshalResponse{operation: operation, wrapped: err}
	}

	switch resp.statusCode {
	case 400:
		return NewApiError(400, responseError, operation)
	case 404:
		return NewApiError(404, responseError, operation)
	case 503:
		return NewApiError(503, responseError, operation)
	}
	return ErrUnknownStatusCode{operation: operation}
}

func (c client) GetLinks(ctx context.Context, id int64) (scrapperapi.ListLinksResponse, error) {
	operation := fmt.Sprintf("get links %v", id)
	resp, err := c.restApiRequest(
		ctx,
		operation,
		"GET",
		c.baseURL+"/links",
		[]headerField{
			{key: "Tg-Chat-Id", value: strconv.FormatInt(id, 10)},
		},
		nil,
	)
	if err != nil {
		return scrapperapi.ListLinksResponse{}, err
	}

	if resp.statusCode == 200 {
		var result scrapperapi.ListLinksResponse
		err = json.Unmarshal(resp.body, &result)
		if err != nil {
			return scrapperapi.ListLinksResponse{}, ErrCantUnmarshalResponse{operation: operation, wrapped: err}
		}
		return result, nil
	}

	var responseError scrapperapi.ApiErrorResponse
	err = json.Unmarshal(resp.body, &responseError)
	if err != nil {
		return scrapperapi.ListLinksResponse{}, ErrCantUnmarshalResponse{operation: operation, wrapped: err}
	}

	switch resp.statusCode {
	case 400:
		return scrapperapi.ListLinksResponse{}, NewApiError(400, responseError, operation)
	case 404:
		return scrapperapi.ListLinksResponse{}, NewApiError(404, responseError, operation)
	case 503:
		return scrapperapi.ListLinksResponse{}, NewApiError(503, responseError, operation)
	}
	return scrapperapi.ListLinksResponse{}, ErrUnknownStatusCode{operation: operation}
}

func (c client) AddLink(ctx context.Context, id int64, addRequest scrapperapi.AddLinkRequest) error {
	operation := fmt.Sprintf("add link %#v to %v", addRequest, id)

	body, err := json.Marshal(addRequest)
	if err != nil {
		return ErrCantMarshalRequest{operation: operation, wrapped: err}
	}

	resp, err := c.restApiRequest(
		ctx,
		operation,
		"POST",
		c.baseURL+"/links",
		[]headerField{
			{key: "Tg-Chat-Id", value: strconv.FormatInt(id, 10)},
		},
		body,
	)
	if err != nil {
		return err
	}

	if resp.statusCode == 200 {
		var linkResponse scrapperapi.LinkResponse
		err = json.Unmarshal(resp.body, &linkResponse)
		if err != nil {
			return ErrCantUnmarshalResponse{operation: operation, wrapped: err}
		}
		return c.verifyResponse(linkResponse, addRequest.URL, false, addRequest.Tags, operation)
	}

	var responseError scrapperapi.ApiErrorResponse
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
	case 503:
		return NewApiError(503, responseError, operation)
	}
	return ErrUnknownStatusCode{operation: operation}
}

func (c client) DeleteLink(ctx context.Context, id int64, deleteRequest scrapperapi.DeleteLinkRequest) error {
	operation := fmt.Sprintf("delete link %#v to %v", deleteRequest, id)

	body, err := json.Marshal(deleteRequest)
	if err != nil {
		return ErrCantMarshalRequest{operation: operation, wrapped: err}
	}

	resp, err := c.restApiRequest(
		ctx,
		operation,
		"DELETE",
		c.baseURL+"/links",
		[]headerField{
			{key: "Tg-Chat-Id", value: strconv.FormatInt(id, 10)},
		},
		body,
	)
	if err != nil {
		return err
	}

	if resp.statusCode == 200 {
		var linkResponse scrapperapi.LinkResponse
		err = json.Unmarshal(resp.body, &linkResponse)
		if err != nil {
			return ErrCantUnmarshalResponse{operation: operation, wrapped: err}
		}
		return c.verifyResponse(linkResponse, deleteRequest.URL, true, []string{}, operation)
	}

	var responseError scrapperapi.ApiErrorResponse
	err = json.Unmarshal(resp.body, &responseError)
	if err != nil {
		return ErrCantUnmarshalResponse{operation: operation, wrapped: err}
	}

	switch resp.statusCode {
	case 400:
		return NewApiError(400, responseError, operation)
	case 404:
		return NewApiError(404, responseError, operation)
	case 503:
		return NewApiError(503, responseError, operation)
	}
	return ErrUnknownStatusCode{operation: operation}
}
