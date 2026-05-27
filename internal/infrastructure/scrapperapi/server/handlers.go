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

const (
	invalidChatMessage  = "invalid chat id"
	unknownErrorMessage = "unknown error occurred"
	chatNotExistMessage = "chat not exist"
)

type service interface {
	RegisterChat(chatID int64) error
	DeleteChat(chatID int64) error
	GetLinks(chatID int64) ([]domain.Link, error)
	AddLink(chatID int64, addLinkRequest scrapper.AddLinkInput) (domain.Link, error)
	DeleteLink(chatID int64, deleteLinkRequest scrapper.DeleteLinkInput) (domain.Link, error)
}

type Handler struct {
	svc    service
	logger *slog.Logger
}

func NewHandler(svc service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

func (h *Handler) RegisterChat(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.response(w, http.StatusBadRequest, setAPIErrorCode(scrapperapi.APIErrorResponse{
			Description: invalidChatMessage,
		}, http.StatusBadRequest))
		return
	}

	if err = h.svc.RegisterChat(id); err != nil {
		if errors.As(err, new(domain.ChatAlreadyExistError)) {
			h.response(w, http.StatusConflict, setAPIErrorCode(scrapperapi.APIErrorResponse{
				Description: "chat already exist",
			}, http.StatusConflict))
		} else {
			h.response(w, http.StatusInternalServerError, setAPIErrorCode(scrapperapi.APIErrorResponse{
				Description: unknownErrorMessage,
			}, http.StatusInternalServerError))
		}
		return
	}

	h.response(w, http.StatusOK, []byte{})
}

func (h *Handler) DeleteChat(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.response(w, http.StatusBadRequest, setAPIErrorCode(scrapperapi.APIErrorResponse{
			Description: invalidChatMessage,
		}, http.StatusBadRequest))
		return
	}

	if err = h.svc.DeleteChat(id); err != nil {
		if errors.As(err, new(domain.ChatNotExistError)) {
			h.response(w, http.StatusNotFound, setAPIErrorCode(scrapperapi.APIErrorResponse{
				Description: chatNotExistMessage,
			}, http.StatusNotFound))
		} else {
			h.response(w, http.StatusInternalServerError, setAPIErrorCode(scrapperapi.APIErrorResponse{
				Description: unknownErrorMessage,
			}, http.StatusInternalServerError))
		}
		return
	}

	h.response(w, http.StatusOK, []byte{})
}

func (h *Handler) GetLinks(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.Header.Get("Tg-Chat-Id"), 10, 64)
	if err != nil {
		h.response(w, http.StatusBadRequest, setAPIErrorCode(scrapperapi.APIErrorResponse{
			Description: invalidChatMessage,
		}, http.StatusBadRequest))
		return
	}

	links, err := h.svc.GetLinks(id)
	if err != nil {
		if errors.As(err, new(domain.ChatNotExistError)) {
			h.response(w, http.StatusNotFound, setAPIErrorCode(scrapperapi.APIErrorResponse{
				Description: chatNotExistMessage,
			}, http.StatusNotFound))
		} else {
			h.response(w, http.StatusInternalServerError, setAPIErrorCode(scrapperapi.APIErrorResponse{
				Description: unknownErrorMessage,
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

func (h *Handler) AddLink(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.Header.Get("Tg-Chat-Id"), 10, 64)
	if err != nil {
		h.response(w, http.StatusBadRequest, setAPIErrorCode(scrapperapi.APIErrorResponse{
			Description: invalidChatMessage,
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
		switch {
		case errors.As(err, new(domain.ChatNotExistError)):
			h.response(w, http.StatusNotFound, setAPIErrorCode(scrapperapi.APIErrorResponse{
				Description: chatNotExistMessage,
			}, http.StatusNotFound))

		case errors.As(err, new(domain.AlreadyTrackingError)):
			h.response(w, http.StatusConflict, setAPIErrorCode(scrapperapi.APIErrorResponse{
				Description: "link is already being tracked",
			}, http.StatusConflict))

		default:
			h.response(w, http.StatusInternalServerError, setAPIErrorCode(scrapperapi.APIErrorResponse{
				Description: unknownErrorMessage,
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

func (h *Handler) DeleteLink(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.Header.Get("Tg-Chat-Id"), 10, 64)
	if err != nil {
		h.response(w, http.StatusBadRequest, setAPIErrorCode(scrapperapi.APIErrorResponse{
			Description: invalidChatMessage,
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
		if errors.As(err, new(domain.ChatOrLinkNotFoundError)) {
			h.response(w, http.StatusNotFound, setAPIErrorCode(scrapperapi.APIErrorResponse{
				Description: "chat not exist or link not found",
			}, http.StatusNotFound))
		} else {
			h.response(w, http.StatusInternalServerError, setAPIErrorCode(scrapperapi.APIErrorResponse{
				Description: unknownErrorMessage,
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

func (h *Handler) response(w http.ResponseWriter, httpStatus int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)

	err := json.NewEncoder(w).Encode(data)
	if err != nil {
		h.logger.Error("marshal response", slog.String("error", err.Error()))
	}
}

func (h *Handler) readBody(w http.ResponseWriter, body io.ReadCloser, dst any) bool {
	requestBody, err := io.ReadAll(body)
	if err != nil {
		h.response(w, http.StatusInternalServerError, setAPIErrorCode(scrapperapi.APIErrorResponse{
			Description: "can't read request body",
		}, http.StatusInternalServerError))
		return false
	}

	err = json.Unmarshal(requestBody, dst)
	if err != nil {
		h.response(w, http.StatusBadRequest, setAPIErrorCode(scrapperapi.APIErrorResponse{
			Description: "invalid request in body",
		}, http.StatusBadRequest))
		return false
	}
	return true
}

func setAPIErrorCode(apiError scrapperapi.APIErrorResponse, httpStatus int) scrapperapi.APIErrorResponse {
	apiError.Code = strconv.Itoa(httpStatus)
	return apiError
}
