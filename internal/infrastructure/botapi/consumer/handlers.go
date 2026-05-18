package consumer

import (
	"encoding/json"
	"fmt"

	"github.com/IBM/sarama"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/api/botapi"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/application/bot"
)

type service interface {
	SendUpdate(sendUpdateInput bot.SendUpdateInput) error
}

type handler struct {
	svc service
}

func NewHandler(svc service) *handler {
	return &handler{svc: svc}
}

func (h *handler) Setup(sarama.ConsumerGroupSession) error {
	return nil
}

func (h *handler) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

func (h *handler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for message := range claim.Messages() {
		fmt.Printf("> Message received. Key=%s. Value=%s. Partition=%d. Offset=%d\n",
			message.Key, message.Value, message.Partition, message.Offset)

		sendUpdate := botapi.LinkUpdate{}
		err := json.Unmarshal(message.Value, &sendUpdate)
		if err != nil {
			return fmt.Errorf("unmarshal message: %w", err)
		}

		err = h.svc.SendUpdate(bot.SendUpdateInput{
			ID:          sendUpdate.ID,
			URL:         sendUpdate.URL,
			Description: sendUpdate.Description,
			TgChatIDs:   sendUpdate.TgChatIDs,
		})
		if err != nil {
			return fmt.Errorf("send update: %w", err)
		}

		session.MarkMessage(message, "")
	}

	return nil
}
