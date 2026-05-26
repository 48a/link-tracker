package server

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/api/scrapperapi"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/application/scrapper"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
)

type service interface {
	RegisterChat(chatID int64) error
	DeleteChat(chatID int64) error
	GetLinks(chatID int64) ([]domain.Link, error)
	AddLink(chatID int64, addLinkRequest scrapper.AddLinkInput) (domain.Link, error)
	DeleteLink(chatID int64, deleteLinkRequest scrapper.DeleteLinkInput) (domain.Link, error)
}

type handler struct {
	svc    service
	logger *slog.Logger
}

func NewHandler(svc service, logger *slog.Logger) *handler {
	return &handler{svc: svc, logger: logger}
}

func (h *handler) RegisterChat(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.response(w, http.StatusBadRequest, setApiErrorCode(scrapperapi.ApiErrorResponse{
			Description: "invalid chat id",
		}, http.StatusBadRequest))
		return
	}

	if err := h.svc.RegisterChat(id); err != nil {
		if errors.As(err, new(domain.ErrChatAlreadyExist)) {
			h.response(w, http.StatusConflict, setApiErrorCode(scrapperapi.ApiErrorResponse{
				Description: "chat already exist",
			}, http.StatusConflict))
		} else {
			h.response(w, http.StatusInternalServerError, setApiErrorCode(scrapperapi.ApiErrorResponse{
				Description: "unknown error occurred",
			}, http.StatusInternalServerError))
		}
		return
	}

	h.response(w, http.StatusOK, []byte{})
}

func (h *handler) DeleteChat(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.response(w, http.StatusBadRequest, setApiErrorCode(scrapperapi.ApiErrorResponse{
			Description: "invalid chat id",
		}, http.StatusBadRequest))
		return
	}

	if err := h.svc.DeleteChat(id); err != nil {
		if errors.As(err, new(domain.ErrChatNotExist)) {
			h.response(w, http.StatusNotFound, setApiErrorCode(scrapperapi.ApiErrorResponse{
				Description: "chat not exist",
			}, http.StatusNotFound))
		} else {
			h.response(w, http.StatusInternalServerError, setApiErrorCode(scrapperapi.ApiErrorResponse{
				Description: "unknown error occurred",
			}, http.StatusInternalServerError))
		}
		return
	}

	h.response(w, http.StatusOK, []byte{})
}

func (h *handler) GetLinks(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.Header.Get("Tg-Chat-Id"), 10, 64)
	if err != nil {
		h.response(w, http.StatusBadRequest, setApiErrorCode(scrapperapi.ApiErrorResponse{
			Description: "invalid chat id",
		}, http.StatusBadRequest))
		return
	}

	links, err := h.svc.GetLinks(id)
	if err != nil {
		if errors.As(err, new(domain.ErrChatNotExist)) {
			h.response(w, http.StatusNotFound, setApiErrorCode(scrapperapi.ApiErrorResponse{
				Description: "chat not exist",
			}, http.StatusNotFound))
		} else {
			h.response(w, http.StatusInternalServerError, setApiErrorCode(scrapperapi.ApiErrorResponse{
				Description: "unknown error occurred",
			}, http.StatusInternalServerError))
		}
		return
	}

	resp := scrapperapi.ListLinksResponse{Links: make([]scrapperapi.LinkResponse, len(links)), Size: len(links)}
	for i := range links {
		resp.Links[i] = scrapperapi.LinkResponse{
			ID:   links[i].ID,
			URL:  links[i].URL,
			Tags: links[i].Tags,
		}
	}

	h.response(w, http.StatusOK, resp)
}

func (h *handler) AddLink(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.Header.Get("Tg-Chat-Id"), 10, 64)
	if err != nil {
		h.response(w, http.StatusBadRequest, setApiErrorCode(scrapperapi.ApiErrorResponse{
			Description: "invalid chat id",
		}, http.StatusBadRequest))
		return
	}

	addRequest := scrapperapi.AddLinkRequest{}
	ok := h.readBody(w, r.Body, &addRequest)
	if !ok {
		return
	}

	link, err := h.svc.AddLink(id, scrapper.AddLinkInput{
		URL:  addRequest.URL,
		Tags: addRequest.Tags,
	})

	if err != nil {
		if errors.As(err, new(domain.ErrChatNotExist)) {
			h.response(w, http.StatusNotFound, setApiErrorCode(scrapperapi.ApiErrorResponse{
				Description: "chat not exist",
			}, http.StatusNotFound))
		} else if errors.As(err, new(domain.ErrAlreadyTracking)) {
			h.response(w, http.StatusConflict, setApiErrorCode(scrapperapi.ApiErrorResponse{
				Description: "link is already being tracked",
			}, http.StatusConflict))
		} else {
			h.response(w, http.StatusInternalServerError, setApiErrorCode(scrapperapi.ApiErrorResponse{
				Description: "unknown error occurred",
			}, http.StatusInternalServerError))
		}
		return
	}

	h.response(w, http.StatusOK, scrapperapi.LinkResponse{
		ID:   link.ID,
		URL:  link.URL,
		Tags: link.Tags,
	})
}

func (h *handler) DeleteLink(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.Header.Get("Tg-Chat-Id"), 10, 64)
	if err != nil {
		h.response(w, http.StatusBadRequest, setApiErrorCode(scrapperapi.ApiErrorResponse{
			Description: "invalid chat id",
		}, http.StatusBadRequest))
		return
	}

	deleteRequest := scrapperapi.DeleteLinkRequest{}
	ok := h.readBody(w, r.Body, &deleteRequest)
	if !ok {
		return
	}

	link, err := h.svc.DeleteLink(id, scrapper.DeleteLinkInput{
		URL: deleteRequest.URL,
	})
	if err != nil {
		if errors.As(err, new(domain.ErrChatOrLinkNotFound)) {
			h.response(w, http.StatusNotFound, setApiErrorCode(scrapperapi.ApiErrorResponse{
				Description: "chat not exist or link not found",
			}, http.StatusNotFound))
		} else {
			h.response(w, http.StatusInternalServerError, setApiErrorCode(scrapperapi.ApiErrorResponse{
				Description: "unknown error occurred",
			}, http.StatusInternalServerError))
		}
		return
	}

	h.response(w, http.StatusOK, scrapperapi.LinkResponse{
		ID:   link.ID,
		URL:  link.URL,
		Tags: link.Tags,
	})
}

func (h *handler) response(w http.ResponseWriter, httpStatus int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)

	err := json.NewEncoder(w).Encode(data)
	if err != nil {
		h.logger.Error("marshal response", slog.String("error", err.Error()))
	}
}

func (h *handler) readBody(w http.ResponseWriter, body io.ReadCloser, dst any) bool {
	requestBody, err := io.ReadAll(body)
	if err != nil {
		h.response(w, http.StatusInternalServerError, setApiErrorCode(scrapperapi.ApiErrorResponse{
			Description: "can't read request body",
		}, http.StatusInternalServerError))
		return false
	}

	err = json.Unmarshal(requestBody, dst)
	if err != nil {
		h.response(w, http.StatusBadRequest, setApiErrorCode(scrapperapi.ApiErrorResponse{
			Description: "invalid request in body",
		}, http.StatusBadRequest))
		return false
	}
	return true
}

func setApiErrorCode(apiError scrapperapi.ApiErrorResponse, httpStatus int) scrapperapi.ApiErrorResponse {
	apiError.Code = strconv.Itoa(httpStatus)
	return apiError
}
