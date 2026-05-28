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

const (
	tgChatHeaderKey = "Tg-Chat-Id"
)

type Client struct {
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

func NewClient(cfg Config) Client {
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

	return Client{
		baseURL: cfg.BaseURL,
		cl:      http.DefaultClient,
		timeout: cfg.Timeout,
		cfg:     cfg,
		cb:      gobreaker.NewCircuitBreaker[responseData](cbSettings),
	}
}

func isRetryable(statusCode int) bool {
	return statusCode >= 500 && statusCode <= 599
}

func (c Client) RegisterChat(ctx context.Context, id int64) error {
	return c.doChatRequest(ctx, id, fmt.Sprintf("register chat %v", id), http.MethodPost, http.StatusConflict)
}

func (c Client) DeleteChat(ctx context.Context, id int64) error {
	return c.doChatRequest(ctx, id, fmt.Sprintf("delete chat %v", id), http.MethodDelete, http.StatusNotFound)
}

func (c Client) GetLinks(ctx context.Context, id int64) (scrapperapi.ListLinksResponse, error) {
	operation := fmt.Sprintf("get links %v", id)
	resp, err := c.restAPIRequest(
		ctx,
		operation,
		"GET",
		c.baseURL+"/links",
		[]headerField{
			{key: tgChatHeaderKey, value: strconv.FormatInt(id, 10)},
		},
		nil,
	)
	if err != nil {
		return scrapperapi.ListLinksResponse{}, err
	}

	if resp.statusCode == http.StatusOK {
		var result scrapperapi.ListLinksResponse
		err = json.Unmarshal(resp.body, &result)
		if err != nil {
			return scrapperapi.ListLinksResponse{}, UnmarshalResponseError{operation: operation, wrapped: err}
		}
		return result, nil
	}

	var responseError scrapperapi.APIErrorResponse
	err = json.Unmarshal(resp.body, &responseError)
	if err != nil {
		return scrapperapi.ListLinksResponse{}, UnmarshalResponseError{operation: operation, wrapped: err}
	}

	switch resp.statusCode {
	case http.StatusBadRequest:
		return scrapperapi.ListLinksResponse{}, NewAPIError(http.StatusBadRequest, responseError, operation)
	case http.StatusNotFound:
		return scrapperapi.ListLinksResponse{}, NewAPIError(http.StatusNotFound, responseError, operation)
	case http.StatusServiceUnavailable:
		return scrapperapi.ListLinksResponse{}, NewAPIError(http.StatusServiceUnavailable, responseError, operation)
	}
	return scrapperapi.ListLinksResponse{}, UnknownStatusCodeError{operation: operation}
}

func (c Client) AddLink(ctx context.Context, id int64, addRequest scrapperapi.AddLinkRequest) error {
	operation := fmt.Sprintf("add link %#v to %v", addRequest, id)

	body, err := json.Marshal(addRequest)
	if err != nil {
		return MarshalRequestError{operation: operation, wrapped: err}
	}

	resp, err := c.restAPIRequest(
		ctx,
		operation,
		"POST",
		c.baseURL+"/links",
		[]headerField{
			{key: tgChatHeaderKey, value: strconv.FormatInt(id, 10)},
		},
		body,
	)
	if err != nil {
		return err
	}

	if resp.statusCode == http.StatusOK {
		var linkResponse scrapperapi.LinkResponse
		err = json.Unmarshal(resp.body, &linkResponse)
		if err != nil {
			return UnmarshalResponseError{operation: operation, wrapped: err}
		}
		return c.verifyResponse(linkResponse, addRequest.URL, false, addRequest.Tags, operation)
	}

	var responseError scrapperapi.APIErrorResponse
	err = json.Unmarshal(resp.body, &responseError)
	if err != nil {
		return UnmarshalResponseError{operation: operation, wrapped: err}
	}

	switch resp.statusCode {
	case http.StatusBadRequest:
		return NewAPIError(http.StatusBadRequest, responseError, operation)
	case http.StatusNotFound:
		return NewAPIError(http.StatusNotFound, responseError, operation)
	case http.StatusConflict:
		return NewAPIError(http.StatusConflict, responseError, operation)
	case http.StatusServiceUnavailable:
		return NewAPIError(http.StatusServiceUnavailable, responseError, operation)
	}
	return UnknownStatusCodeError{operation: operation}
}

func (c Client) DeleteLink(ctx context.Context, id int64, deleteRequest scrapperapi.DeleteLinkRequest) error {
	operation := fmt.Sprintf("delete link %#v to %v", deleteRequest, id)

	body, err := json.Marshal(deleteRequest)
	if err != nil {
		return MarshalRequestError{operation: operation, wrapped: err}
	}

	resp, err := c.restAPIRequest(
		ctx,
		operation,
		"DELETE",
		c.baseURL+"/links",
		[]headerField{
			{key: tgChatHeaderKey, value: strconv.FormatInt(id, 10)},
		},
		body,
	)
	if err != nil {
		return err
	}

	if resp.statusCode == http.StatusOK {
		var linkResponse scrapperapi.LinkResponse
		err = json.Unmarshal(resp.body, &linkResponse)
		if err != nil {
			return UnmarshalResponseError{operation: operation, wrapped: err}
		}
		return c.verifyResponse(linkResponse, deleteRequest.URL, true, []string{}, operation)
	}

	var responseError scrapperapi.APIErrorResponse
	err = json.Unmarshal(resp.body, &responseError)
	if err != nil {
		return UnmarshalResponseError{operation: operation, wrapped: err}
	}

	switch resp.statusCode {
	case http.StatusBadRequest:
		return NewAPIError(http.StatusBadRequest, responseError, operation)
	case http.StatusNotFound:
		return NewAPIError(http.StatusNotFound, responseError, operation)
	case http.StatusServiceUnavailable:
		return NewAPIError(http.StatusServiceUnavailable, responseError, operation)
	}
	return UnknownStatusCodeError{operation: operation}
}

func (Client) verifyResponse(linkResponse scrapperapi.LinkResponse, link string, skipTags bool, tags []string, operation string) error {
	if linkResponse.URL != link {
		return LinkMismatchError{operation: operation}
	}
	if !skipTags && slices.Compare(linkResponse.Tags, tags) != 0 {
		return TagsMismatchError{operation: operation}
	}
	return nil
}

func (c Client) restAPIRequest(ctx context.Context, operation, method, url string, headerFields []headerField, requestBody []byte) (responseData, error) {
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
					return retry.Unrecoverable(CreateRequestError{operation: operation, wrapped: err})
				}

				for _, field := range headerFields {
					req.Header.Add(field.key, field.value)
				}

				res, err := c.cl.Do(req)
				if err != nil {
					if errors.Is(err, context.DeadlineExceeded) {
						return DoRequestError{operation: operation, wrapped: TimedoutError{}}
					}
					return fmt.Errorf("do request: %w", err)
				}
				defer func() { _ = res.Body.Close() }()

				b, err := io.ReadAll(res.Body)
				if err != nil {
					return retry.Unrecoverable(ReadResponseError{operation: operation, wrapped: err})
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

		if err != nil {
			return lastResp, fmt.Errorf("retry do: %w", err)
		}
		return lastResp, nil
	})

	if cbErr != nil {
		if errors.Is(cbErr, gobreaker.ErrOpenState) || errors.Is(cbErr, gobreaker.ErrTooManyRequests) {
			return c.fallbackResponse()
		}
		return responseData{}, fmt.Errorf("circuit breaker: %w", cbErr)
	}

	return resp, nil
}

func (c Client) fallbackResponse() (responseData, error) {
	return responseData{
		body:       []byte(`{"description":"service temporarily unavailable (Circuit Breaker OPEN)","code":"http.StatusServiceUnavailable"}`),
		statusCode: http.StatusServiceUnavailable,
	}, nil
}

func (c Client) doChatRequest(ctx context.Context, id int64, operation, method string, targetErrorStatus int) error {
	resp, err := c.restAPIRequest(
		ctx,
		operation,
		method,
		c.baseURL+"/tg-chat/"+strconv.FormatInt(id, 10),
		[]headerField{},
		nil,
	)
	if err != nil {
		return err
	}

	if resp.statusCode == http.StatusOK {
		return nil
	}

	var responseError scrapperapi.APIErrorResponse
	err = json.Unmarshal(resp.body, &responseError)
	if err != nil {
		return UnmarshalResponseError{operation: operation, wrapped: err}
	}

	switch resp.statusCode {
	case http.StatusBadRequest:
		return NewAPIError(http.StatusBadRequest, responseError, operation)
	case http.StatusServiceUnavailable:
		return NewAPIError(http.StatusServiceUnavailable, responseError, operation)
	case targetErrorStatus:
		return NewAPIError(targetErrorStatus, responseError, operation)
	}
	return UnknownStatusCodeError{operation: operation}
}
