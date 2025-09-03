package kafka

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
	"faraday/internal/config"
)

// KafkaClient 提供Kafka消息发送功能
type KafkaClient struct {
	writer *kafka.Writer
	config *config.KafkaConfig
}

// QueryLog 表示要发送到Kafka的MySQL查询日志结构
type QueryLog struct {
	SourceIP      string    `json:"source_ip"`
	QueryContent  string    `json:"query_content"`
	ExecutionTime int64     `json:"execution_time_ms"`
	Timestamp     time.Time `json:"timestamp"`
}

// NewKafkaClient 创建一个新的Kafka客户端
func NewKafkaClient(config *config.KafkaConfig) *KafkaClient {
	// 如果没有配置Kafka，返回空客户端
	if config == nil || len(config.Brokers) == 0 || config.Topic == "" {
		return &KafkaClient{nil, config}
	}

	writer := kafka.NewWriter(kafka.WriterConfig{
		Brokers:  config.Brokers,
		Topic:    config.Topic,
		Balancer: &kafka.LeastBytes{},
	})

	return &KafkaClient{
		writer: writer,
		config: config,
	}
}

// IsEnabled 检查Kafka功能是否启用
func (c *KafkaClient) IsEnabled() bool {
	return c.writer != nil
}

// SendQueryLog 发送MySQL查询日志到Kafka
func (c *KafkaClient) SendQueryLog(logEntry *QueryLog) error {
	if !c.IsEnabled() {
		return nil // Kafka未启用，直接返回成功
	}
	
	// 初始化时间戳
	if logEntry.Timestamp.IsZero() {
		logEntry.Timestamp = time.Now()
	}

	// 将日志条目转换为JSON
	jsonData, err := json.Marshal(logEntry)
	if err != nil {
		return err
	}

	// 创建Kafka消息
	message := kafka.Message{
		Value: jsonData,
	}

	// 发送消息
	err = c.writer.WriteMessages(context.Background(), message)
	if err != nil {
		log.Printf("发送Kafka消息失败: %v", err)
		return err
	}

	return nil
}

// Close 关闭Kafka客户端
func (c *KafkaClient) Close() error {
	if !c.IsEnabled() {
		return nil
	}

	return c.writer.Close()
}