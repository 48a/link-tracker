package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/api/botapi"
)

type RESTClient struct {
	baseURL string
	cl      *http.Client
	timeout time.Duration
}

type responseData struct {
	body       []byte
	statusCode int
}

func NewRestClient(url string, timeout time.Duration) RESTClient {
	return RESTClient{baseURL: url, cl: http.DefaultClient, timeout: timeout}
}

func (c RESTClient) SendUpdate(linkUpdate botapi.LinkUpdate) error {
	operation := fmt.Sprintf("send link update %#v", linkUpdate)

	body, err := json.Marshal(linkUpdate)
	if err != nil {
		return MarshalRequestError{operation: operation, wrapped: err}
	}

	resp, err := c.restAPIRequest(
		operation,
		"POST",
		c.baseURL+"/updates",
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}

	if resp.statusCode == http.StatusOK {
		return nil
	}

	var responseError botapi.APIErrorResponse
	err = json.Unmarshal(resp.body, &responseError)
	if err != nil {
		return UnmarshalResponseError{operation: operation, wrapped: err}
	}

	if resp.statusCode == http.StatusBadRequest {
		return NewAPIError(http.StatusBadRequest, responseError, operation)
	}
	return UnknownStatusCodeError{operation: operation}
}

func (c RESTClient) restAPIRequest(operation, method, url string, requestBody io.Reader) (responseData, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, url, requestBody)
	if err != nil {
		return responseData{}, CreateRequestError{operation: operation, wrapped: err}
	}

	resp, err := c.cl.Do(req)
	if errors.Is(err, context.DeadlineExceeded) {
		return responseData{}, DoRequestError{operation: operation, wrapped: TimedoutError{}}
	}
	if err != nil {
		return responseData{}, DoRequestError{operation: operation, wrapped: err}
	}
	defer func() { _ = resp.Body.Close() }()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return responseData{}, ReadResponseError{operation: operation, wrapped: err}
	}

	return responseData{body: b, statusCode: resp.StatusCode}, nil
}
