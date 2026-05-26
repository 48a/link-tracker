package producer

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/IBM/sarama"
	"github.com/linkedin/goavro/v2"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/api/botapi"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/kafkaconfig"
)

const avroSchema = `{
  "type": "record",
  "name": "LinkUpdateEvent",
  "namespace": "com.example.notification",
  "fields": [
    { "name": "id", "type": "long", "doc": "Link ID" },
    { "name": "url", "type": "string", "doc": "Link URL" },
    { "name": "description", "type": "string", "doc": "Changes description" },
    { "name": "tgChatIds", "type": {"type": "array", "items": "long"}, "doc": "Chat IDs" }
  ]
}`

type producer struct {
	prod     sarama.SyncProducer
	topic    string
	schemaID uint32
	codec    *goavro.Codec
}

func NewProducer(brokerAddrs []string, kafkaUser, kafkaPassword, kafkaTopic, schemaRegistryURL string) (*producer, error) {
	cfg := kafkaconfig.NewConfig(kafkaUser, kafkaPassword)

	p, err := sarama.NewSyncProducer(brokerAddrs, cfg)
	if err != nil {
		return nil, fmt.Errorf("new sync producer: %w", err)
	}

	codec, err := goavro.NewCodec(avroSchema)
	if err != nil {
		return nil, fmt.Errorf("parse avro schema: %w", err)
	}

	schemaID, err := registerSchema(schemaRegistryURL, kafkaTopic, avroSchema)
	if err != nil {
		return nil, fmt.Errorf("register schema: %w", err)
	}

	return &producer{
		prod:     p,
		topic:    kafkaTopic,
		schemaID: schemaID,
		codec:    codec,
	}, nil
}

func (p *producer) SendUpdate(linkUpdate botapi.LinkUpdate) error {
	tgChatIDs := make([]interface{}, len(linkUpdate.TgChatIDs))
	for i, id := range linkUpdate.TgChatIDs {
		tgChatIDs[i] = id
	}

	native := map[string]interface{}{
		"id":          linkUpdate.ID,
		"url":         linkUpdate.URL,
		"description": linkUpdate.Description,
		"tgChatIds":   tgChatIDs,
	}

	avroData, err := p.codec.BinaryFromNative(nil, native)
	if err != nil {
		return fmt.Errorf("encode avro data: %w", err)
	}

	wireMsg := make([]byte, 5+len(avroData))
	wireMsg[0] = 0
	binary.BigEndian.PutUint32(wireMsg[1:5], p.schemaID)
	copy(wireMsg[5:], avroData)

	msg := &sarama.ProducerMessage{
		Topic: p.topic,
		Value: sarama.ByteEncoder(wireMsg),
	}

	partition, offset, err := p.prod.SendMessage(msg)
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}

	fmt.Printf("> Avro message sent. Partition=%d. Offset=%d. SchemaID=%d\n", partition, offset, p.schemaID)
	return nil
}

func (p *producer) Close() error {
	return p.prod.Close()
}

type srRegisterRequest struct {
	Schema string `json:"schema"`
}

type srRegisterResponse struct {
	ID uint32 `json:"id"`
}

func registerSchema(srURL, topic, schemaStr string) (uint32, error) {
	reqBody, _ := json.Marshal(srRegisterRequest{Schema: schemaStr})
	resp, err := http.Post(fmt.Sprintf("%s/subjects/%s-value/versions", srURL, topic), "application/vnd.schemaregistry.v1+json", bytes.NewBuffer(reqBody))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("schema registry returned status %d: %s", resp.StatusCode, string(body))
	}

	var srResp srRegisterResponse
	if err := json.NewDecoder(resp.Body).Decode(&srResp); err != nil {
		return 0, err
	}
	return srResp.ID, nil
}
