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

type restClient struct {
	baseURL string
	cl      *http.Client
	timeout time.Duration
}

type responseData struct {
	body       []byte
	statusCode int
}

func NewRestClient(url string, timeout time.Duration) restClient {
	return restClient{baseURL: url, cl: http.DefaultClient, timeout: timeout}
}

func (c restClient) restApiRequest(operation, method, url string, requestBody io.Reader) (responseData, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, url, requestBody)
	if err != nil {
		return responseData{}, ErrCantCreateRequest{operation: operation, wrapped: err}
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

func (c restClient) SendUpdate(linkUpdate botapi.LinkUpdate) error {
	operation := fmt.Sprintf("send link update %#v", linkUpdate)

	body, err := json.Marshal(linkUpdate)
	if err != nil {
		return ErrCantMarshalRequest{operation: operation, wrapped: err}
	}

	resp, err := c.restApiRequest(
		operation,
		"POST",
		c.baseURL+"/updates",
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}

	if resp.statusCode == 200 {
		return nil
	}

	var responseError botapi.ApiErrorResponse
	err = json.Unmarshal(resp.body, &responseError)
	if err != nil {
		return ErrCantUnmarshalResponse{operation: operation, wrapped: err}
	}

	if resp.statusCode == 400 {
		return NewApiError(400, responseError, operation)
	}
	return ErrUnknownStatusCode{operation: operation}
}
