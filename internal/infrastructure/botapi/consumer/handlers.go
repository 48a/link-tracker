package consumer

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/IBM/sarama"
	"github.com/linkedin/goavro/v2"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/api/botapi"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/application/bot"
)

type service interface {
	SendUpdate(sendUpdateInput bot.SendUpdateInput)
}

type Handler struct {
	svc               service
	schemaRegistryURL string
	codecCache        map[uint32]*goavro.Codec
	mu                sync.RWMutex
}

func NewHandler(svc service, schemaRegistryURL string) *Handler {
	return &Handler{
		svc:               svc,
		schemaRegistryURL: schemaRegistryURL,
		codecCache:        make(map[uint32]*goavro.Codec),
	}
}

func (h *Handler) Setup(sarama.ConsumerGroupSession) error {
	return nil
}

func (h *Handler) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

func (h *Handler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for message := range claim.Messages() {
		if len(message.Value) < 5 || message.Value[0] != 0 {
			return errors.New("invalid message format: missing magic byte")
		}

		schemaID := binary.BigEndian.Uint32(message.Value[1:5])
		avroData := message.Value[5:]

		codec, err := h.getCodec(schemaID)
		if err != nil {
			return fmt.Errorf("get schema codec: %w", err)
		}

		native, _, err := codec.NativeFromBinary(avroData)
		if err != nil {
			return fmt.Errorf("unmarshal avro data: %w", err)
		}

		record, ok := native.(map[string]interface{})
		if !ok {
			return fmt.Errorf("expected map[string]interface{}, got %T", native)
		}

		var sendUpdate botapi.LinkUpdate
		var id int64
		if id, ok = record["id"].(int64); ok {
			sendUpdate.ID = id
		}
		var url string
		if url, ok = record["url"].(string); ok {
			sendUpdate.URL = url
		}
		var desc string
		if desc, ok = record["description"].(string); ok {
			sendUpdate.Description = desc
		}

		var tgChatIDsRaw []interface{}
		if tgChatIDsRaw, ok = record["tgChatIds"].([]interface{}); ok {
			for _, idRaw := range tgChatIDsRaw {
				if id, ok = idRaw.(int64); ok {
					sendUpdate.TgChatIDs = append(sendUpdate.TgChatIDs, id)
				}
			}
		}

		h.svc.SendUpdate(bot.SendUpdateInput{
			ID:          sendUpdate.ID,
			URL:         sendUpdate.URL,
			Description: sendUpdate.Description,
			TgChatIDs:   sendUpdate.TgChatIDs,
		})

		session.MarkMessage(message, "")
	}

	return nil
}

type srSchemaResponse struct {
	Schema string `json:"schema"`
}

func (h *Handler) getCodec(id uint32) (*goavro.Codec, error) {
	h.mu.RLock()
	if codec, exists := h.codecCache[id]; exists {
		h.mu.RUnlock()
		return codec, nil
	}
	h.mu.RUnlock()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, fmt.Sprintf("%s/schemas/ids/%d", h.schemaRegistryURL, id), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("schema registry returned status %d", resp.StatusCode)
	}

	var srResp srSchemaResponse
	if err = json.NewDecoder(resp.Body).Decode(&srResp); err != nil {
		return nil, fmt.Errorf("decode response body: %w", err)
	}

	parsedCodec, err := goavro.NewCodec(srResp.Schema)
	if err != nil {
		return nil, fmt.Errorf("new codec: %w", err)
	}

	h.mu.Lock()
	h.codecCache[id] = parsedCodec
	h.mu.Unlock()

	return parsedCodec, nil
}
