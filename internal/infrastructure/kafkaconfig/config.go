package kafkaconfig

import "github.com/IBM/sarama"

func NewConfig(kafkaUser, kafkaPassword string) *sarama.Config {
	cfg := sarama.NewConfig()

	cfg.Version = sarama.V4_0_0_0

	cfg.Producer.Partitioner = sarama.NewRandomPartitioner
	cfg.Producer.Return.Successes = true
	cfg.Producer.RequiredAcks = sarama.WaitForAll
	cfg.Producer.Compression = sarama.CompressionGZIP

	cfg.Consumer.Offsets.AutoCommit.Enable = false

	cfg.Net.SASL.Enable = true
	cfg.Net.SASL.User = kafkaUser
	cfg.Net.SASL.Password = kafkaPassword
	cfg.Net.SASL.Mechanism = sarama.SASLTypePlaintext

	return cfg
}
