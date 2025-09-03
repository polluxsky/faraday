package config

import (
	"io/ioutil"
	"log"
	"os"

	"gopkg.in/yaml.v2"
)

// ProxyConfig 定义单个MySQL代理的配置
type ProxyConfig struct {
	ProxyName   string `yaml:"proxy_name"`
	ListenPort  int    `yaml:"listen_port"`
	TargetHost  string `yaml:"target_host"`
	TargetPort  int    `yaml:"target_port"`
	BufferSize  int    `yaml:"buffer_size"`
	LogLevel    string `yaml:"log_level"`
}

// DaemonConfig 定义守护进程的配置
type DaemonConfig struct {
	LogFile string `yaml:"log_file"`
	PidFile string `yaml:"pid_file"`
}

// KafkaConfig 定义Kafka相关配置
type KafkaConfig struct {
	Brokers []string `yaml:"brokers"`
	Topic   string   `yaml:"topic"`
}

// Config 定义完整的配置结构
type Config struct {
	Proxies []ProxyConfig `yaml:"proxies"`
	Daemon  DaemonConfig  `yaml:"daemon"`
	Kafka   KafkaConfig   `yaml:"kafka,omitempty"`
}

// LoadConfig 从指定路径加载配置文件
func LoadConfig(configPath string) (*Config, error) {
	// 如果未指定配置路径，使用默认路径
	if configPath == "" {
		configPath = "configs/faraday.yml"
	}

	// 检查配置文件是否存在
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Printf("配置文件 %s 不存在，使用默认配置\n", configPath)
		// 返回默认配置
		return &Config{
			Proxies: []ProxyConfig{
				{
					ProxyName:  "default_proxy",
					ListenPort: 3307,
					TargetHost: "localhost",
					TargetPort: 3306,
					BufferSize: 1000,
					LogLevel:   "INFO",
				},
			},
			Daemon: DaemonConfig{
				LogFile: "/var/log/faraday-deamon.log",
				PidFile: "/var/run/faraday-deamon.pid",
			},
			Kafka: KafkaConfig{
				// 默认不启用Kafka，为空数组和空字符串
				Brokers: []string{},
				Topic:   "",
			},
		},
		nil
	}

	// 读取配置文件内容
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	// 解析YAML配置
	config := &Config{}
	err = yaml.Unmarshal(data, config)
	if err != nil {
		return nil, err
	}

	return config, nil
}