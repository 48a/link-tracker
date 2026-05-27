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
	"github.com/testcontainers/testcontainers-go/modules/kafka"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/application/agent"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/agentkafka"
)

type tgSendMessageReq struct {
	ChatID int64  `json:"chat_id"`
	Text   string `json:"text"`
}

type containerResult struct {
	c   testcontainers.Container
	err error
}

//nolint:tparallel // avoid data race
func TestE2EFlow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tgUpdates := make(chan string, 10)
	tgReplies := make(chan tgSendMessageReq, 10)
	var githubTriggered atomic.Bool

	mockServer, internalMockURL := startMockServer(t, tgUpdates, tgReplies, &githubTriggered)
	defer mockServer.Close()

	netw, err := network.New(ctx)
	if err != nil {
		t.Fatalf("failed to create network: %v", err)
	}
	defer func() { _ = netw.Remove(ctx) }()

	dbScrapCh := asyncStart(func() (testcontainers.Container, error) {
		return startPostgres(ctx, netw.Name, "postgres", "scrapper_db")
	})
	dbBotCh := asyncStart(func() (testcontainers.Container, error) {
		return startPostgres(ctx, netw.Name, "postgres-1", "bot_db")
	})

	resScrap := <-dbScrapCh
	if resScrap.err != nil {
		t.Fatalf("failed to start scrapper db: %v", resScrap.err)
	}
	defer func() { _ = resScrap.c.Terminate(ctx) }()

	resBot := <-dbBotCh
	if resBot.err != nil {
		t.Fatalf("failed to start bot db: %v", resBot.err)
	}
	defer func() { _ = resBot.c.Terminate(ctx) }()

	scrapperCh := asyncStart(func() (testcontainers.Container, error) {
		return startScrapper(ctx, netw.Name, "postgres", internalMockURL, "HTTP", "", "")
	})
	botCh := asyncStart(func() (testcontainers.Container, error) {
		return startBot(ctx, netw.Name, "postgres-1", internalMockURL, "HTTP", "", "")
	})

	resScrapper := <-scrapperCh
	if resScrapper.err != nil {
		t.Fatalf("failed to start scrapper: %v", resScrapper.err)
	}
	defer func() { _ = resScrapper.c.Terminate(ctx) }()

	resBotApp := <-botCh
	if resBotApp.err != nil {
		t.Fatalf("failed to start bot: %v", resBotApp.err)
	}
	defer func() { _ = resBotApp.c.Terminate(ctx) }()

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

//nolint:tparallel // avoid data race
func TestScrapperKafkaBotFlow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tgUpdates := make(chan string, 10)
	tgReplies := make(chan tgSendMessageReq, 10)
	var githubTriggered atomic.Bool

	mockServer, internalMockURL := startMockServer(t, tgUpdates, tgReplies, &githubTriggered)
	defer mockServer.Close()

	netw, err := network.New(ctx)
	if err != nil {
		t.Fatalf("failed to create network: %v", err)
	}
	defer func() { _ = netw.Remove(ctx) }()

	dbScrapCh := asyncStart(func() (testcontainers.Container, error) {
		return startPostgres(ctx, netw.Name, "postgres", "scrapper_db")
	})
	dbBotCh := asyncStart(func() (testcontainers.Container, error) {
		return startPostgres(ctx, netw.Name, "postgres-1", "bot_db")
	})
	kafkaCh := asyncStart(func() (testcontainers.Container, error) { return startKafka(ctx, netw.Name) })

	resScrap := <-dbScrapCh
	if resScrap.err != nil {
		t.Fatalf("failed to start scrapper db: %v", resScrap.err)
	}
	defer func() { _ = resScrap.c.Terminate(ctx) }()

	resBot := <-dbBotCh
	if resBot.err != nil {
		t.Fatalf("failed to start bot db: %v", resBot.err)
	}
	defer func() { _ = resBot.c.Terminate(ctx) }()

	resKafka := <-kafkaCh
	if resKafka.err != nil {
		t.Fatalf("failed to start kafka: %v", resKafka.err)
	}
	defer func() { _ = resKafka.c.Terminate(ctx) }()

	srCh := asyncStart(func() (testcontainers.Container, error) { return startSchemaRegistry(ctx, netw.Name) })

	resSR := <-srCh
	if resSR.err != nil {
		t.Fatalf("failed to start schema registry: %v", resSR.err)
	}
	defer func() { _ = resSR.c.Terminate(ctx) }()

	scrapperCh := asyncStart(func() (testcontainers.Container, error) {
		return startScrapper(ctx, netw.Name, "postgres", internalMockURL, "KAFKA", "kafka:9092", "http://schema-registry:8081")
	})
	botCh := asyncStart(func() (testcontainers.Container, error) {
		return startBot(ctx, netw.Name, "postgres-1", internalMockURL, "KAFKA", "kafka:9092", "http://schema-registry:8081")
	})

	resScrapper := <-scrapperCh
	if resScrapper.err != nil {
		t.Fatalf("failed to start scrapper: %v", resScrapper.err)
	}
	defer func() { _ = resScrapper.c.Terminate(ctx) }()

	resBotApp := <-botCh
	if resBotApp.err != nil {
		t.Fatalf("failed to start bot: %v", resBotApp.err)
	}
	defer func() { _ = resBotApp.c.Terminate(ctx) }()

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

		githubTriggered.Store(true)

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

//nolint:gocognit,tparallel // avoid data race
func TestAgentKafkaIntegration(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	kafkaContainer, err := kafka.Run(ctx,
		"confluentinc/confluent-local:7.5.0",
		kafka.WithClusterID("test-cluster"),
	)
	if err != nil {
		t.Fatalf("failed to start kafka: %v", err)
	}
	defer func() { _ = kafkaContainer.Terminate(ctx) }()

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
	defer func() { _ = producer.Close() }()

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
	defer func() { _ = consumerGroup.Close() }()

	go func() {
		for {
			if consumeErr := consumerGroup.Consume(agentCtx, []string{"link.raw-updates"}, worker); consumeErr != nil {
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
	defer func() { _ = testConsumer.Close() }()

	time.Sleep(3 * time.Second)

	var partConsumer sarama.PartitionConsumer
	for range 10 {
		partConsumer, err = testConsumer.ConsumePartition("link.processed-updates", 0, sarama.OffsetOldest)
		if err == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}
	if err != nil {
		t.Fatalf("consume partition: %v", err)
	}
	defer func() { _ = partConsumer.Close() }()

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

func asyncStart(startFn func() (testcontainers.Container, error)) <-chan containerResult {
	ch := make(chan containerResult, 1)
	go func() {
		c, err := startFn()
		ch <- containerResult{c: c, err: err}
		close(ch)
	}()
	return ch
}

func startMockServer(t *testing.T, tgUpdates <-chan string, tgReplies chan<- tgSendMessageReq, githubTriggered *atomic.Bool) (*httptest.Server, string) {
	t.Helper()
	mockServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path == "/botTEST_TOKEN/getMe" {
			_, _ = w.Write([]byte(`{"ok": true, "result": {"id": 123456789, "is_bot": true, "first_name": "TestBot", "username": "TestBot"}}`))
			return
		}

		if r.URL.Path == "/botTEST_TOKEN/getUpdates" {
			select {
			case update := <-tgUpdates:
				_, _ = w.Write([]byte(update))
			default:
				_, _ = w.Write([]byte(`{"ok": true, "result": []}`))
			}
			return
		}

		if r.URL.Path == "/botTEST_TOKEN/sendMessage" {
			var req tgSendMessageReq
			if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
				body, err := io.ReadAll(r.Body)
				if err == nil {
					_ = json.Unmarshal(body, &req)
				}
				_ = r.Body.Close()
			} else {
				_ = r.ParseForm()
				req.ChatID, _ = strconv.ParseInt(r.FormValue("chat_id"), 10, 64)
				req.Text = r.FormValue("text")
			}
			tgReplies <- req
			_, _ = w.Write([]byte(`{"ok": true}`))
			return
		}

		if strings.HasPrefix(r.URL.Path, "/repos/testuser/testrepo/issues") {
			if githubTriggered != nil && githubTriggered.Load() {
				futureTime := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
				resp := fmt.Sprintf(`[{"title": "Test Kafka Update", "user": {"login": "testuser"}, "body": "Kafka flow works!", "created_at": "%s", "updated_at": "%s"}]`, futureTime, futureTime)
				_, _ = w.Write([]byte(resp))
			} else {
				_, _ = w.Write([]byte(`[]`))
			}
			return
		}

		t.Logf("Mock server received unexpected request: %s %s", r.Method, r.URL.Path)
	}))

	l, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("failed to listen on 0.0.0.0: %v", err)
	}
	_ = mockServer.Listener.Close()
	mockServer.Listener = l
	mockServer.Start()

	port := mockServer.Listener.Addr().(*net.TCPAddr).Port
	return mockServer, fmt.Sprintf("http://host.docker.internal:%d", port)
}

func startPostgres(ctx context.Context, netName, alias, dbName string) (testcontainers.Container, error) {
	req := testcontainers.ContainerRequest{
		Image:        "postgres:17",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "user",
			"POSTGRES_PASSWORD": "password",
			"POSTGRES_DB":       dbName,
		},
		Networks:       []string{netName},
		NetworkAliases: map[string][]string{netName: {alias}},
		Tmpfs: map[string]string{
			"/var/lib/postgresql/data": "rw,noexec,nosuid,size=256m",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(2 * time.Minute),
	}
	return testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
}

func startKafka(ctx context.Context, netName string) (testcontainers.Container, error) {
	req := testcontainers.ContainerRequest{
		Image:          "bitnamilegacy/kafka:4.0.0",
		Networks:       []string{netName},
		NetworkAliases: map[string][]string{netName: {"kafka"}},
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
		Tmpfs: map[string]string{
			"/bitnami/kafka": "rw,noexec,nosuid,size=512m",
		},
		WaitingFor: wait.ForListeningPort("9092/tcp").WithStartupTimeout(3 * time.Minute),
	}
	return testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
}

func startSchemaRegistry(ctx context.Context, netName string) (testcontainers.Container, error) {
	req := testcontainers.ContainerRequest{
		Image:          "confluentinc/cp-schema-registry:7.5.0",
		Networks:       []string{netName},
		NetworkAliases: map[string][]string{netName: {"schema-registry"}},
		ExposedPorts:   []string{"8081/tcp"},
		Env: map[string]string{
			"SCHEMA_REGISTRY_HOST_NAME":                    "schema-registry",
			"SCHEMA_REGISTRY_KAFKASTORE_BOOTSTRAP_SERVERS": "PLAINTEXT://kafka:9094",
			"SCHEMA_REGISTRY_KAFKASTORE_SECURITY_PROTOCOL": "PLAINTEXT",
		},
		WaitingFor: wait.ForHTTP("/subjects").WithPort("8081/tcp").WithStatusCodeMatcher(func(status int) bool {
			return status == http.StatusOK
		}).WithStartupTimeout(2 * time.Minute),
	}
	return testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
}

func startScrapper(ctx context.Context, netName, dbHost, internalMockURL, botCommType, kafkaBroker, srURL string) (testcontainers.Container, error) {
	env := map[string]string{
		"DB_SCRAPPER_HOST":       dbHost,
		"DB_SCRAPPER_PORT":       "5432",
		"DB_SCRAPPER_USERNAME":   "user",
		"DB_SCRAPPER_PASSWORD":   "password",
		"DB_SCRAPPER_NAME":       "scrapper_db",
		"BOT_COMMUNICATION_TYPE": botCommType,
		"CACHE":                  "FALSE",
		"GITHUB_API_URL":         internalMockURL,
		"STACKEXCHANGE_API_URL":  internalMockURL,
	}

	if botCommType == "KAFKA" {
		env["KAFKA_USER"] = "user1"
		env["KAFKA_PASSWORD"] = "pass123"
		env["KAFKA_BROKER"] = kafkaBroker
		env["KAFKA_TOPIC"] = "updates"
		env["JOB_DURATION"] = "2"
		env["SCHEMA_REGISTRY_URL"] = srURL
	}

	req := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    ".",
			Dockerfile: "Dockerfile.scrapper",
		},
		Networks:       []string{netName},
		NetworkAliases: map[string][]string{netName: {"scrapper"}},
		ExtraHosts:     []string{"host.docker.internal:host-gateway"},
		Env:            env,
		WaitingFor: wait.ForHTTP("/links").WithPort("8001/tcp").WithStatusCodeMatcher(func(status int) bool {
			return status == http.StatusBadRequest || status == http.StatusOK
		}).WithStartupTimeout(2 * time.Minute),
	}
	return testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
}

func startBot(ctx context.Context, netName, dbHost, internalMockURL, botCommType, kafkaBroker, srURL string) (testcontainers.Container, error) {
	env := map[string]string{
		"DB_BOT_HOST":            dbHost,
		"DB_BOT_PORT":            "5432",
		"DB_BOT_USERNAME":        "user",
		"DB_BOT_PASSWORD":        "password",
		"DB_BOT_NAME":            "bot_db",
		"BOT_COMMUNICATION_TYPE": botCommType,
		"APP_TELEGRAM_TOKEN":     "TEST_TOKEN",
		"SKIP_COMMANDS":          "TRUE",
		"TG_API_BASE_URL":        internalMockURL,
	}

	if botCommType == "KAFKA" {
		env["KAFKA_USER"] = "user1"
		env["KAFKA_PASSWORD"] = "pass123"
		env["KAFKA_BROKER"] = kafkaBroker
		env["KAFKA_TOPIC"] = "updates"
		env["KAFKA_CONSUMER_GROUP"] = "e2e-test-group"
		env["SCHEMA_REGISTRY_URL"] = srURL
	}

	req := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    ".",
			Dockerfile: "Dockerfile.bot",
		},
		Networks:       []string{netName},
		NetworkAliases: map[string][]string{netName: {"bot"}},
		ExtraHosts:     []string{"host.docker.internal:host-gateway"},
		Env:            env,
		WaitingFor:     wait.ForLog("start polling").WithStartupTimeout(2 * time.Minute),
	}
	return testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
}
