package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/testcontainers/testcontainers-go/modules/kafka"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/application/agent"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/agentkafka"
)

type tgSendMessageReq struct {
	ChatID int64  `json:"chat_id"`
	Text   string `json:"text"`
}

//nolint:gocognit
func TestE2EFlow(t *testing.T) {
	ctx := context.Background()

	tgUpdates := make(chan string, 10)
	tgReplies := make(chan tgSendMessageReq, 10)

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/botTEST_TOKEN/getMe" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"ok": true,
				"result": {
					"id": 123456789,
					"is_bot": true,
					"first_name": "TestBot",
					"username": "TestBot"
				}
			}`))
			return
		}

		if r.URL.Path == "/botTEST_TOKEN/getUpdates" {
			w.Header().Set("Content-Type", "application/json")
			select {
			case update := <-tgUpdates:
				w.Write([]byte(update))
			default:
				w.Write([]byte(`{"ok": true, "result": []}`))
			}
			return
		}

		if r.URL.Path == "/botTEST_TOKEN/sendMessage" {
			var req tgSendMessageReq

			if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &req)
			} else {
				_ = r.ParseForm()
				req.ChatID, _ = strconv.ParseInt(r.FormValue("chat_id"), 10, 64)
				req.Text = r.FormValue("text")
			}

			tgReplies <- req

			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"ok": true}`))
			return
		}

		t.Logf("Mock server received unexpected request: %s %s", r.Method, r.URL.Path)
	}))

	defer mockServer.Close()

	mockServerPort := mockServer.Listener.Addr().(*net.TCPAddr).Port
	internalMockURL := fmt.Sprintf("http://host.docker.internal:%d", mockServerPort)

	netw, err := network.New(ctx)
	if err != nil {
		t.Fatalf("failed to create network: %v", err)
	}
	defer netw.Remove(ctx)

	scrapperDBReq := testcontainers.ContainerRequest{
		Image:        "postgres:17",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "user",
			"POSTGRES_PASSWORD": "password",
			"POSTGRES_DB":       "scrapper_db",
		},
		Networks:       []string{netw.Name},
		NetworkAliases: map[string][]string{netw.Name: {"postgres"}},
		WaitingFor:     wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(2 * time.Minute),
	}
	scrapperDB, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: scrapperDBReq,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start scrapper db: %v", err)
	}
	defer scrapperDB.Terminate(ctx)

	botDBReq := testcontainers.ContainerRequest{
		Image:        "postgres:17",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "user",
			"POSTGRES_PASSWORD": "password",
			"POSTGRES_DB":       "bot_db",
		},
		Networks:       []string{netw.Name},
		NetworkAliases: map[string][]string{netw.Name: {"postgres-1"}},
		WaitingFor:     wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(2 * time.Minute),
	}
	botDB, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: botDBReq,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start bot db: %v", err)
	}
	defer botDB.Terminate(ctx)

	scrapperReq := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    ".",
			Dockerfile: "Dockerfile.scrapper",
		},
		Networks:       []string{netw.Name},
		NetworkAliases: map[string][]string{netw.Name: {"scrapper"}},
		Env: map[string]string{
			"DB_SCRAPPER_HOST":       "postgres",
			"DB_SCRAPPER_PORT":       "5432",
			"DB_SCRAPPER_USERNAME":   "user",
			"DB_SCRAPPER_PASSWORD":   "password",
			"DB_SCRAPPER_NAME":       "scrapper_db",
			"BOT_COMMUNICATION_TYPE": "HTTP",
			"CACHE":                  "FALSE",
			"GITHUB_API_URL":         internalMockURL,
			"STACKEXCHANGE_API_URL":  internalMockURL,
		},
		WaitingFor: wait.ForHTTP("/links").WithPort("8001/tcp").WithStatusCodeMatcher(func(status int) bool {
			return status == http.StatusBadRequest || status == http.StatusOK
		}).WithStartupTimeout(2 * time.Minute),
	}
	scrapperContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: scrapperReq,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start scrapper: %v", err)
	}
	defer scrapperContainer.Terminate(ctx)

	botReq := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    ".",
			Dockerfile: "Dockerfile.bot",
		},
		Networks:       []string{netw.Name},
		NetworkAliases: map[string][]string{netw.Name: {"bot"}},
		Env: map[string]string{
			"DB_BOT_HOST":            "postgres-1",
			"DB_BOT_PORT":            "5432",
			"DB_BOT_USERNAME":        "user",
			"DB_BOT_PASSWORD":        "password",
			"DB_BOT_NAME":            "bot_db",
			"BOT_COMMUNICATION_TYPE": "HTTP",
			"APP_TELEGRAM_TOKEN":     "TEST_TOKEN",
			"SKIP_COMMANDS":          "TRUE",
			"TG_API_BASE_URL":        internalMockURL,
		},
		WaitingFor: wait.ForLog("start polling").WithStartupTimeout(2 * time.Minute),
	}
	botContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: botReq,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start bot: %v", err)
	}
	defer botContainer.Terminate(ctx)

	chatID := int64(1001)

	t.Run("handle /start command", func(t *testing.T) {
		tgUpdates <- fmt.Sprintf(`{"ok": true, "result": [{"update_id": 1, "message": {"message_id": 1, "chat": {"id": %d}, "text": "/start"}}]}`, chatID)

		select {
		case reply := <-tgReplies:
			if reply.ChatID != chatID {
				t.Fatalf("expected chatID %d, got %d", chatID, reply.ChatID)
			}
			if reply.Text != "registered" {
				t.Fatalf("expected text 'registered', got '%s'", reply.Text)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for /start reply")
		}
	})

	t.Run("handle /help command", func(t *testing.T) {
		tgUpdates <- fmt.Sprintf(`{"ok": true, "result": [{"update_id": 2, "message": {"message_id": 2, "chat": {"id": %d}, "text": "/help"}}]}`, chatID)

		select {
		case reply := <-tgReplies:
			if reply.ChatID != chatID {
				t.Fatalf("expected chatID %d, got %d", chatID, reply.ChatID)
			}
			if reply.Text != "help message" {
				t.Fatalf("expected text 'help message', got '%s'", reply.Text)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for /help reply")
		}
	})

	t.Run("handle /track flow", func(t *testing.T) {
		tgUpdates <- fmt.Sprintf(`{"ok": true, "result": [{"update_id": 3, "message": {"message_id": 3, "chat": {"id": %d}, "text": "/track"}}]}`, chatID)

		reply := <-tgReplies
		if reply.Text != "send a link to be tracked" {
			t.Fatalf("expected 'send a link to be tracked', got '%s'", reply.Text)
		}

		tgUpdates <- fmt.Sprintf(`{"ok": true, "result": [{"update_id": 4, "message": {"message_id": 4, "chat": {"id": %d}, "text": "https://github.com/some/repo"}}]}`, chatID)

		reply = <-tgReplies
		if reply.Text != "send link tags, separated by comma" {
			t.Fatalf("expected 'send link tags, separated by comma', got '%s'", reply.Text)
		}

		tgUpdates <- fmt.Sprintf(`{"ok": true, "result": [{"update_id": 5, "message": {"message_id": 5, "chat": {"id": %d}, "text": "golang, test"}}]}`, chatID)

		reply = <-tgReplies
		if reply.Text != "ok, saved" {
			t.Fatalf("expected 'ok, saved', got '%s'", reply.Text)
		}
	})
}

//nolint:gocognit
func TestScrapperKafkaBotFlow(t *testing.T) {
	ctx := context.Background()

	tgUpdates := make(chan string, 10)
	tgReplies := make(chan tgSendMessageReq, 10)

	var githubUpdatesTriggered atomic.Bool

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/botTEST_TOKEN/getMe" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"ok": true, "result": {"id": 123, "is_bot": true, "first_name": "Test", "username": "Test"}}`))
			return
		}

		if r.URL.Path == "/botTEST_TOKEN/getUpdates" {
			w.Header().Set("Content-Type", "application/json")
			select {
			case update := <-tgUpdates:
				w.Write([]byte(update))
			default:
				w.Write([]byte(`{"ok": true, "result": []}`))
			}
			return
		}

		if r.URL.Path == "/botTEST_TOKEN/sendMessage" {
			var req tgSendMessageReq
			if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &req)
			} else {
				_ = r.ParseForm()
				req.ChatID, _ = strconv.ParseInt(r.FormValue("chat_id"), 10, 64)
				req.Text = r.FormValue("text")
			}
			tgReplies <- req

			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"ok": true}`))
			return
		}

		if strings.HasPrefix(r.URL.Path, "/repos/testuser/testrepo/issues") {
			w.Header().Set("Content-Type", "application/json")
			if githubUpdatesTriggered.Load() {
				futureTime := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
				resp := fmt.Sprintf(`[{"title": "Test Kafka Update", "user": {"login": "testuser"}, "body": "Kafka flow works!", "created_at": "%s", "updated_at": "%s"}]`, futureTime, futureTime)
				w.Write([]byte(resp))
			} else {
				w.Write([]byte(`[]`))
			}
			return
		}
	}))
	defer mockServer.Close()

	mockServerPort := mockServer.Listener.Addr().(*net.TCPAddr).Port
	internalMockURL := fmt.Sprintf("http://host.docker.internal:%d", mockServerPort)

	netw, err := network.New(ctx)
	if err != nil {
		t.Fatalf("failed to create network: %v", err)
	}
	defer netw.Remove(ctx)

	kafkaReq := testcontainers.ContainerRequest{
		Image:          "bitnamilegacy/kafka:4.0.0",
		Networks:       []string{netw.Name},
		NetworkAliases: map[string][]string{netw.Name: {"kafka"}},
		ExposedPorts:   []string{"9092/tcp", "9094/tcp"},
		Env: map[string]string{
			"KAFKA_CFG_NODE_ID":                              "0",
			"KAFKA_CFG_PROCESS_ROLES":                        "broker, controller",
			"KAFKA_CFG_CONTROLLER_QUORUM_VOTERS":             "0@kafka:9093",
			"KAFKA_CFG_LISTENERS":                            "BROKER://:9092,CONTROLLER://:9093,INTERNAL://:9094",
			"KAFKA_CFG_LISTENER_SECURITY_PROTOCOL_MAP":       "BROKER:SASL_PLAINTEXT,CONTROLLER:PLAINTEXT,INTERNAL:PLAINTEXT",
			"KAFKA_CFG_ADVERTISED_LISTENERS":                 "BROKER://kafka:9092,INTERNAL://kafka:9094",
			"KAFKA_CFG_INTER_BROKER_LISTENER_NAME":           "BROKER",
			"KAFKA_CFG_CONTROLLER_LISTENER_NAMES":            "CONTROLLER",
			"KAFKA_CFG_SASL_ENABLED_MECHANISMS":              "PLAIN",
			"KAFKA_CLIENT_USERS":                             "user1",
			"KAFKA_CLIENT_PASSWORDS":                         "pass123",
			"KAFKA_CFG_SASL_MECHANISM_INTER_BROKER_PROTOCOL": "PLAIN",
			"KAFKA_CFG_AUTO_CREATE_TOPICS_ENABLE":            "true",
		},
		WaitingFor: wait.ForListeningPort("9092/tcp").WithStartupTimeout(3 * time.Minute),
	}
	kafkaContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: kafkaReq,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start kafka: %v", err)
	}
	defer kafkaContainer.Terminate(ctx)

	srReq := testcontainers.ContainerRequest{
		Image:          "confluentinc/cp-schema-registry:7.5.0",
		Networks:       []string{netw.Name},
		NetworkAliases: map[string][]string{netw.Name: {"schema-registry"}},
		ExposedPorts:   []string{"8081/tcp"},
		Env: map[string]string{
			"SCHEMA_REGISTRY_HOST_NAME":                    "schema-registry",
			"SCHEMA_REGISTRY_KAFKASTORE_BOOTSTRAP_SERVERS": "kafka:9094",
			"SCHEMA_REGISTRY_KAFKASTORE_SECURITY_PROTOCOL": "PLAINTEXT",
		},
		WaitingFor: wait.ForHTTP("/subjects").WithPort("8081/tcp").WithStatusCodeMatcher(func(status int) bool {
			return status == http.StatusOK
		}).WithStartupTimeout(2 * time.Minute),
	}
	srContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: srReq,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start schema registry: %v", err)
	}
	defer srContainer.Terminate(ctx)

	/*
		srHost, _ := srContainer.Host(ctx)
		srPort, _ := srContainer.MappedPort(ctx, "8081")
		internalSRURL := fmt.Sprintf("http://%s:%s", srHost, srPort.Port())
	*/

	scrapperDBReq := testcontainers.ContainerRequest{
		Image:        "postgres:17",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "user",
			"POSTGRES_PASSWORD": "password",
			"POSTGRES_DB":       "scrapper_db",
		},
		Networks:       []string{netw.Name},
		NetworkAliases: map[string][]string{netw.Name: {"postgres"}},
		WaitingFor:     wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(2 * time.Minute),
	}
	scrapperDB, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: scrapperDBReq,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start scrapper db: %v", err)
	}
	defer scrapperDB.Terminate(ctx)

	botDBReq := testcontainers.ContainerRequest{
		Image:        "postgres:17",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "user",
			"POSTGRES_PASSWORD": "password",
			"POSTGRES_DB":       "bot_db",
		},
		Networks:       []string{netw.Name},
		NetworkAliases: map[string][]string{netw.Name: {"postgres-1"}},
		WaitingFor:     wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(2 * time.Minute),
	}
	botDB, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: botDBReq,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start bot db: %v", err)
	}
	defer botDB.Terminate(ctx)

	scrapperReq := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    ".",
			Dockerfile: "Dockerfile.scrapper",
		},
		Networks:       []string{netw.Name},
		NetworkAliases: map[string][]string{netw.Name: {"scrapper"}},
		Env: map[string]string{
			"DB_SCRAPPER_HOST":       "postgres",
			"DB_SCRAPPER_PORT":       "5432",
			"DB_SCRAPPER_USERNAME":   "user",
			"DB_SCRAPPER_PASSWORD":   "password",
			"DB_SCRAPPER_NAME":       "scrapper_db",
			"BOT_COMMUNICATION_TYPE": "KAFKA",
			"KAFKA_USER":             "user1",
			"KAFKA_PASSWORD":         "pass123",
			"KAFKA_BROKER":           "kafka:9092",
			"KAFKA_TOPIC":            "updates",
			"JOB_DURATION":           "2",
			"CACHE":                  "FALSE",
			"GITHUB_API_URL":         internalMockURL,
			"STACKEXCHANGE_API_URL":  internalMockURL,
			"SCHEMA_REGISTRY_URL":    "http://schema-registry:8081",
		},
		WaitingFor: wait.ForHTTP("/links").WithPort("8001/tcp").WithStatusCodeMatcher(func(status int) bool {
			return status == http.StatusBadRequest || status == http.StatusOK
		}).WithStartupTimeout(2 * time.Minute),
	}
	scrapperContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: scrapperReq,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start scrapper: %v", err)
	}
	defer scrapperContainer.Terminate(ctx)

	botReq := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    ".",
			Dockerfile: "Dockerfile.bot",
		},
		Networks:       []string{netw.Name},
		NetworkAliases: map[string][]string{netw.Name: {"bot"}},
		Env: map[string]string{
			"DB_BOT_HOST":            "postgres-1",
			"DB_BOT_PORT":            "5432",
			"DB_BOT_USERNAME":        "user",
			"DB_BOT_PASSWORD":        "password",
			"DB_BOT_NAME":            "bot_db",
			"BOT_COMMUNICATION_TYPE": "KAFKA",
			"KAFKA_USER":             "user1",
			"KAFKA_PASSWORD":         "pass123",
			"KAFKA_BROKER":           "kafka:9092",
			"KAFKA_TOPIC":            "updates",
			"KAFKA_CONSUMER_GROUP":   "e2e-test-group",
			"APP_TELEGRAM_TOKEN":     "TEST_TOKEN",
			"SKIP_COMMANDS":          "TRUE",
			"TG_API_BASE_URL":        internalMockURL,
			"SCHEMA_REGISTRY_URL":    "http://schema-registry:8081",
		},
		WaitingFor: wait.ForLog("start polling").WithStartupTimeout(2 * time.Minute),
	}
	botContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: botReq,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start bot: %v", err)
	}
	defer botContainer.Terminate(ctx)

	chatID := int64(3003)

	t.Run("Kafka Integration Flow", func(t *testing.T) {
		tgUpdates <- fmt.Sprintf(`{"ok": true, "result": [{"update_id": 1, "message": {"message_id": 1, "chat": {"id": %d}, "text": "/start"}}]}`, chatID)
		if reply := <-tgReplies; reply.Text != "registered" {
			t.Fatalf("expected 'registered', got '%s'", reply.Text)
		}

		tgUpdates <- fmt.Sprintf(`{"ok": true, "result": [{"update_id": 2, "message": {"message_id": 2, "chat": {"id": %d}, "text": "/track"}}]}`, chatID)
		if reply := <-tgReplies; reply.Text != "send a link to be tracked" {
			t.Fatalf("expected 'send a link to be tracked', got '%s'", reply.Text)
		}

		tgUpdates <- fmt.Sprintf(`{"ok": true, "result": [{"update_id": 3, "message": {"message_id": 3, "chat": {"id": %d}, "text": "https://github.com/testuser/testrepo"}}]}`, chatID)
		if reply := <-tgReplies; reply.Text != "send link tags, separated by comma" {
			t.Fatalf("expected 'send link tags...', got '%s'", reply.Text)
		}

		tgUpdates <- fmt.Sprintf(`{"ok": true, "result": [{"update_id": 4, "message": {"message_id": 4, "chat": {"id": %d}, "text": "golang"}}]}`, chatID)
		if reply := <-tgReplies; reply.Text != "ok, saved" {
			t.Fatalf("expected 'ok, saved', got '%s'", reply.Text)
		}

		githubUpdatesTriggered.Store(true)

		select {
		case reply := <-tgReplies:
			if !strings.HasPrefix(reply.Text, "update to link") {
				t.Fatalf("expected kafka update notification, got: %s", reply.Text)
			}
			if !strings.Contains(reply.Text, "Test Kafka Update") {
				t.Fatalf("expected update payload in text, got: %s", reply.Text)
			}
		case <-time.After(15 * time.Second):
			t.Fatal("timeout waiting for kafka update notification")
		}
	})
}

//nolint:gocognit
func TestAgentKafkaIntegration(t *testing.T) {
	ctx := context.Background()

	kafkaContainer, err := kafka.Run(ctx,
		"confluentinc/confluent-local:7.5.0",
		kafka.WithClusterID("test-cluster"),
	)
	if err != nil {
		t.Fatalf("failed to start kafka: %v", err)
	}
	defer kafkaContainer.Terminate(ctx)

	brokers, err := kafkaContainer.Brokers(ctx)
	if err != nil || len(brokers) == 0 {
		t.Fatalf("failed to get brokers: %v", err)
	}
	brokerAddr := brokers[0]
	t.Logf("Kafka is successfully running at: %s", brokerAddr)

	saramaCfg := sarama.NewConfig()
	saramaCfg.Producer.Return.Successes = true
	saramaCfg.Producer.Return.Errors = true
	saramaCfg.Consumer.Offsets.Initial = sarama.OffsetOldest

	producer, err := sarama.NewSyncProducer([]string{brokerAddr}, saramaCfg)
	if err != nil {
		t.Fatalf("new sync producer: %v", err)
	}
	defer producer.Close()

	agentCtx, cancelAgent := context.WithCancel(ctx)
	defer cancelAgent()

	processorCfg := agent.Config{
		StopWords:       []string{"spam"},
		ExcludedAuthors: []string{"bad_author"},
		MinLength:       10,
		SumThreshold:    500,
	}
	processor := agent.NewProcessor(processorCfg)

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	worker := agentkafka.NewWorker(processor, producer, "link.processed-updates", logger)

	consumerGroup, err := sarama.NewConsumerGroup([]string{brokerAddr}, "test-group", saramaCfg)
	if err != nil {
		t.Fatalf("new consumer group: %v", err)
	}
	defer consumerGroup.Close()

	go func() {
		for {
			if err = consumerGroup.Consume(agentCtx, []string{"link.raw-updates"}, worker); err != nil {
				return
			}
			if agentCtx.Err() != nil {
				return
			}
		}
	}()

	testConsumer, err := sarama.NewConsumer([]string{brokerAddr}, saramaCfg)
	if err != nil {
		t.Fatalf("new test consumer: %v", err)
	}
	defer testConsumer.Close()

	time.Sleep(3 * time.Second)

	partConsumer, err := testConsumer.ConsumePartition("link.processed-updates", 0, sarama.OffsetNewest)
	if err != nil {
		t.Fatalf("consume partition: %v", err)
	}
	defer partConsumer.Close()

	t.Run("receive valid message", func(t *testing.T) {
		validMsg := `{"id": 12345, "description": "This is a perfectly valid long update.", "author": "good_author", "tgChatIds": [111, 222]}`
		_, _, err = producer.SendMessage(&sarama.ProducerMessage{
			Topic: "link.raw-updates",
			Value: sarama.StringEncoder(validMsg),
		})
		if err != nil {
			t.Fatalf("send message: %v", err)
		}

		select {
		case msg := <-partConsumer.Messages():
			var processed domain.ProcessedUpdate
			if err = json.Unmarshal(msg.Value, &processed); err != nil {
				t.Fatalf("failed to unmarshal output message: %v", err)
			}
			if processed.ID != 12345 || processed.Priority != "HIGH" {
				t.Errorf("unexpected message content: %+v", processed)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("timeout waiting for processed message")
		}
	})

	t.Run("invalid format does not crash agent", func(t *testing.T) {
		invalidMsg := `{"id": "this-should-be-int", "broken_json": `
		_, _, err = producer.SendMessage(&sarama.ProducerMessage{
			Topic: "link.raw-updates",
			Value: sarama.StringEncoder(invalidMsg),
		})
		if err != nil {
			t.Fatalf("send message: %v", err)
		}

		time.Sleep(3 * time.Second)

		validMsg2 := `{"id": 999, "description": "This is another valid update.", "author": "good_author", "tgChatIds": [333]}`
		_, _, err = producer.SendMessage(&sarama.ProducerMessage{
			Topic: "link.raw-updates",
			Value: sarama.StringEncoder(validMsg2),
		})
		if err != nil {
			t.Fatalf("send message: %v", err)
		}

		select {
		case msg := <-partConsumer.Messages():
			var processed domain.ProcessedUpdate
			if err = json.Unmarshal(msg.Value, &processed); err != nil {
				t.Fatalf("failed to unmarshal output message: %v", err)
			}
			if processed.ID != 999 {
				t.Errorf("expected to recover and process message ID 999, got %d", processed.ID)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("timeout waiting for recovery message")
		}
	})
}
