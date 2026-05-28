package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/api/botapi"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/application/bot"
)

type service interface {
	SendUpdate(sendUpdateInput bot.SendUpdateInput)
}

type Handler struct {
	svc service
}

func NewHandler(svc service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) SendUpdate(w http.ResponseWriter, r *http.Request) {
	sendUpdate := botapi.LinkUpdate{}
	ok := h.readBody(w, r.Body, &sendUpdate)
	if !ok {
		return
	}

	h.svc.SendUpdate(bot.SendUpdateInput{
		ID:          sendUpdate.ID,
		URL:         sendUpdate.URL,
		Description: sendUpdate.Description,
		TgChatIDs:   sendUpdate.TgChatIDs,
	})

	h.response(w, http.StatusOK, []byte{})
}

func (h *Handler) response(w http.ResponseWriter, httpStatus int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)

	err := json.NewEncoder(w).Encode(data)
	if err != nil {
		fmt.Println("failed to marshal response, better logging soon")
	}
}

func (h *Handler) readBody(w http.ResponseWriter, body io.ReadCloser, dst any) bool {
	requestBody, err := io.ReadAll(body)
	if err != nil {
		h.response(w, http.StatusInternalServerError, setAPIErrorCode(botapi.APIErrorResponse{
			Description: "can't read request body",
		}, http.StatusInternalServerError))
		return false
	}

	err = json.Unmarshal(requestBody, dst)
	if err != nil {
		h.response(w, http.StatusBadRequest, setAPIErrorCode(botapi.APIErrorResponse{
			Description: "invalid request in body",
		}, http.StatusBadRequest))
		return false
	}
	return true
}

func setAPIErrorCode(apiError botapi.APIErrorResponse, httpStatus int) botapi.APIErrorResponse {
	apiError.Code = strconv.Itoa(httpStatus)
	return apiError
}
