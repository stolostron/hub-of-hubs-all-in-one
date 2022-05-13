package kafka

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/go-logr/logr"
	"github.com/stolostron/hub-of-hubs-kafka-transport/headers"
	kafkaproducer "github.com/stolostron/hub-of-hubs-kafka-transport/kafka-client/kafka-producer"
	"github.com/stolostron/hub-of-hubs-manager/pkg/compressor"
	"github.com/stolostron/hub-of-hubs-manager/pkg/specsyncer/db2transport/transport"
)

const (
	MaxMessageSizeLimit = 987 // to make sure that the message size is below 1 MB.
	partition           = 0
	kiloBytesToBytes    = 1000
)

type KafkaProducerConfig struct {
	ProducerID     string
	ProducerTopic  string
	MsgSizeLimitKB int
}

// NewProducer returns a new instance of Producer object.
func NewProducer(compressor compressor.Compressor, bootstrapServer string, producerConfig *KafkaProducerConfig, log logr.Logger) (*Producer, error) {
	kafkaConfigMap := &kafka.ConfigMap{
		"bootstrap.servers": bootstrapServer,
		"client.id":         producerConfig.ProducerID,
		"acks":              "1",
		"retries":           "0",
	}

	deliveryChan := make(chan kafka.Event)
	kafkaProducer, err := kafkaproducer.NewKafkaProducer(kafkaConfigMap, producerConfig.MsgSizeLimitKB*kiloBytesToBytes,
		deliveryChan)
	if err != nil {
		close(deliveryChan)
		return nil, fmt.Errorf("failed to create producer: %w", err)
	}

	return &Producer{
		log:           log,
		kafkaProducer: kafkaProducer,
		topic:         producerConfig.ProducerTopic,
		compressor:    compressor,
		deliveryChan:  deliveryChan,
		stopChan:      make(chan struct{}),
	}, nil
}

// Producer abstracts hub-of-hubs-kafka-transport kafka-producer's generic usage.
type Producer struct {
	log           logr.Logger
	kafkaProducer *kafkaproducer.KafkaProducer
	topic         string
	compressor    compressor.Compressor
	deliveryChan  chan kafka.Event
	stopChan      chan struct{}
	startOnce     sync.Once
	stopOnce      sync.Once
}

// Start starts kafka producer.
func (p *Producer) Start() {
	p.startOnce.Do(func() {
		go p.deliveryReportHandler()
	})
}

// Stop stops the producer.
func (p *Producer) Stop() {
	p.stopOnce.Do(func() {
		p.stopChan <- struct{}{}
		p.kafkaProducer.Close()
		close(p.deliveryChan)
		close(p.stopChan)
	})
}

// SendAsync sends a message to the sync service asynchronously.
func (p *Producer) SendAsync(destinationHubName string, id string, msgType string, version string, payload []byte) {
	msg := &transport.Message{
		Destination: destinationHubName,
		ID:          id,
		MsgType:     msgType,
		Version:     version,
		Payload:     payload,
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		p.log.Error(err, "Failed to send message", "MessageId", msg.ID, "MessageType", msg.MsgType,
			"Version", msg.Version)

		return
	}

	compressedBytes, err := p.compressor.Compress(msgBytes)
	if err != nil {
		p.log.Error(err, "Failed to compress bundle", "CompressorType", p.compressor.GetType(),
			"MessageId", msg.ID, "MessageType", msg.MsgType, "Version", msg.Version)

		return
	}

	messageHeaders := []kafka.Header{
		{Key: headers.CompressionType, Value: []byte(p.compressor.GetType())},
	}

	msgKey := msg.ID
	if destinationHubName != transport.Broadcast { // set destination if specified
		msgKey = fmt.Sprintf("%s.%s", destinationHubName, msg.ID)

		messageHeaders = append(messageHeaders, kafka.Header{
			Key:   headers.DestinationHub,
			Value: []byte(destinationHubName),
		})
	}

	if err = p.kafkaProducer.ProduceAsync(msgKey, p.topic, partition, messageHeaders, compressedBytes); err != nil {
		p.log.Error(err, "Failed to send message", "MessageId", msg.ID, "MessageType", msg.MsgType,
			"Version", msg.Version, "Destination", msg.Destination)
	}

	p.log.Info("Message sent successfully", "MessageId", msg.ID, "MessageType", msg.MsgType,
		"Version", msg.Version, "Destination", msg.Destination)
}

func (p *Producer) deliveryReportHandler() {
	for {
		select {
		case <-p.stopChan:
			return

		case event := <-p.deliveryChan:
			p.handleDeliveryReport(event)
		}
	}
}

// handleDeliveryReport handles results of sent messages. For now failed messages are only logged.
func (p *Producer) handleDeliveryReport(kafkaEvent kafka.Event) {
	switch event := kafkaEvent.(type) {
	case *kafka.Message:
		if event.TopicPartition.Error != nil {
			p.log.Error(event.TopicPartition.Error, "Failed to deliver message", "MessageId",
				string(event.Key), "TopicPartition", event.TopicPartition)
		}
	default:
		p.log.Info("Received unsupported kafka-event type", "EventType", event)
	}
}
