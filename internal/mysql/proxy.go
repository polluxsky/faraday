package mysql

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"faraday/internal/config"
	"faraday/internal/kafka"
	"faraday/internal/logger"
	"faraday/pkg/utils"
)

// PacketType 定义MySQL包类型
const (
	PacketTypeOK uint8 = 0
	PacketTypeError uint8 = 1
	PacketTypeEOF uint8 = 0xfe
	PacketTypeResult uint8 = 0xfb
	// 其他包类型
)

// Proxy MySQL代理结构体
type Proxy struct {
	config       *config.ProxyConfig
	log          *logger.Logger
	kafkaEnabled bool
	kafkaConfig  *config.KafkaConfig
	kafkaClient  *kafka.KafkaClient
	serverAddr   string
	clientAddr   string
	bufferPool   *sync.Pool
	wg           sync.WaitGroup
	statsChan    chan *QueryStats
	stopChan     chan struct{}
}

// QueryStats 查询统计信息
type QueryStats struct {
	SourceIP      string
	QueryContent  string
	StartTime     time.Time
	EndTime       time.Time
	RowCount      int64
}

// NewProxy 创建一个新的MySQL代理实例
func NewProxy(proxyConfig *config.ProxyConfig, fullConfig *config.Config, log *logger.Logger) *Proxy {
	// 创建一个缓冲区池，用于处理数据包
	bufferPool := &sync.Pool{
		New: func() interface{} {
			return make([]byte, 4096)
		},
	}

	// 创建查询统计通道
	statsChan := make(chan *QueryStats, proxyConfig.BufferSize)

	// 检查Kafka配置是否启用
	kafkaEnabled := false
	var kafkaConfig *config.KafkaConfig
	var kafkaClient *kafka.KafkaClient
	if fullConfig != nil && len(fullConfig.Kafka.Brokers) > 0 && fullConfig.Kafka.Topic != "" {
		kafkaEnabled = true
		kafkaConfig = &fullConfig.Kafka
		kafkaClient = kafka.NewKafkaClient(kafkaConfig)
		log.Info(logger.LogEntry{
			AdditionalInfo: []byte(fmt.Sprintf(`{"event":"kafka_enabled", "brokers":"%v", "topic":"%s"}`, kafkaConfig.Brokers, kafkaConfig.Topic)),
		})
	}

	return &Proxy{
		config:     proxyConfig,
		log:        log,
		kafkaEnabled: kafkaEnabled,
		kafkaConfig:  kafkaConfig,
		kafkaClient:  kafkaClient,
		serverAddr: fmt.Sprintf("%s:%d", proxyConfig.TargetHost, proxyConfig.TargetPort),
		clientAddr: fmt.Sprintf(":%d", proxyConfig.ListenPort),
		bufferPool: bufferPool,
		statsChan:  statsChan,
		stopChan:   make(chan struct{}),
	}
}

// Start 启动MySQL代理
func (p *Proxy) Start() error {
	// 启动统计处理协程
	p.wg.Add(1)
	go p.processStats()

	// 监听客户端连接
	listener, err := net.Listen("tcp", p.clientAddr)
	if err != nil {
		p.wg.Done()
		return fmt.Errorf("监听端口 %d 失败: %w", p.config.ListenPort, err)
	}

	// 记录代理启动信息
	p.log.Info(logger.LogEntry{
		AdditionalInfo: []byte(fmt.Sprintf(`{"event":"proxy_start", "listen_port":%d, "target_server":"%s"}`, p.config.ListenPort, p.serverAddr)),
	})

	// 启动连接接受循环
	go func() {
		defer func() {
			listener.Close()
			p.wg.Wait()
			p.log.Info(logger.LogEntry{
				AdditionalInfo: []byte(fmt.Sprintf(`{"event":"proxy_stop", "listen_port":%d}`, p.config.ListenPort)),
			})
		}()

		for {
			select {
			case <-p.stopChan:
				return
			default:
				// 接受客户端连接
				conn, err := listener.Accept()
				if err != nil {
					// 检查是否是因为关闭监听器导致的错误
					select {
					case <-p.stopChan:
						return
					default:
						p.log.Error(logger.LogEntry{
					ErrorMessage: fmt.Sprintf("接受连接失败: %v", err),
				})
					}
					continue
				}

				// 处理连接
				p.wg.Add(1)
				go p.handleConnection(conn)
			}
		}
	}()

	return nil
}

// Stop 停止MySQL代理
func (p *Proxy) Stop() {
	close(p.stopChan)
	close(p.statsChan)
	p.wg.Wait()
}

// handleConnection 处理客户端连接
func (p *Proxy) handleConnection(clientConn net.Conn) {
	defer p.wg.Done()
	defer clientConn.Close()

	// 获取客户端地址
	clientAddr := clientConn.RemoteAddr().String()

	// 连接到MySQL服务器
	serverConn, err := net.Dial("tcp", p.serverAddr)
	if err != nil {
		p.log.Error(logger.LogEntry{
			SourceIP:    clientAddr,
			ErrorMessage: fmt.Sprintf("连接MySQL服务器失败: %v", err),
		})
		return
	}
	defer serverConn.Close()

	// 记录连接信息
	p.log.Info(logger.LogEntry{
		SourceIP:    clientAddr,
		AdditionalInfo: []byte(fmt.Sprintf(`{"event":"client_connect"}`)),
	})

	// 创建双向数据流
	done := make(chan struct{}) 

	// 从客户端到服务器的数据流
	p.wg.Add(1)
	go p.pipe(clientConn, serverConn, clientAddr, done)

	// 从服务器到客户端的数据流
	p.wg.Add(1)
	go p.pipe(serverConn, clientConn, clientAddr, done)

	// 等待任一方向的数据传输结束
	<-done
	close(done)

	// 记录断开连接信息
	p.log.Info(logger.LogEntry{
		SourceIP:    clientAddr,
		AdditionalInfo: []byte(fmt.Sprintf(`{"event":"client_disconnect"}`)),
	})
}

// pipe 在两个连接之间传输数据
func (p *Proxy) pipe(src, dst net.Conn, clientAddr string, done chan struct{}) {
	defer p.wg.Done()

	// 从src读取数据并写入dst
	buffer := p.bufferPool.Get().([]byte)
	defer p.bufferPool.Put(buffer)

	for {
		select {
		case <-p.stopChan:
			return
		default:
			// 读取数据
			n, err := src.Read(buffer)
			if err != nil {
				// 如果是EOF错误，表示连接已关闭
				if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
					p.log.Error(logger.LogEntry{
					SourceIP:    clientAddr,
					ErrorMessage: fmt.Sprintf("读取数据失败: %v", err),
				})
				}
				done <- struct{}{}
				return
			}

			// 分析和记录MySQL数据包
			if src.RemoteAddr().String() == clientAddr {
				// 从客户端到服务器的数据，可能包含查询
				p.analyzePacket(buffer[:n], clientAddr)
			}

			// 转发数据到目标连接
			written, err := dst.Write(buffer[:n])
			if err != nil {
				p.log.Error(logger.LogEntry{
			SourceIP:    clientAddr,
			ErrorMessage: fmt.Sprintf("写入数据失败: %v", err),
		})
				done <- struct{}{}
				return
			}

			if written != n {
				p.log.Error(logger.LogEntry{
			SourceIP:    clientAddr,
			ErrorMessage: "写入的数据长度不匹配",
		})
				done <- struct{}{}
				return
			}
		}
	}
}

// analyzePacket 分析MySQL数据包
func (p *Proxy) analyzePacket(packet []byte, clientAddr string) {
	// 如果数据包太小，无法解析，直接返回
	if len(packet) < 4 {
		return
	}

	// 解析数据包长度
	packetLen := int(binary.LittleEndian.Uint32(packet[0:4]))
	if packetLen > len(packet) {
		// 数据包不完整
		return
	}

	// 跳过数据包长度和序号
	data := packet[4:]

	// 解析MySQL命令类型
	if len(data) > 0 {
		commandType := data[0]
		// 对于查询命令，尝试提取查询内容
		if commandType == 3 /* COM_QUERY */ && len(data) > 1 {
			queryContent := string(data[1:])
			// 对查询字符串进行转码，处理特殊字符
			transcodedQuery := utils.TranscodeString(queryContent, utils.DefaultSQLTranscoder())
			// 发送查询统计信息到通道
			p.statsChan <- &QueryStats{
				SourceIP:     clientAddr,
				QueryContent: transcodedQuery,
				StartTime:    time.Now(),
			}
		}
	}
}

// processStats 处理查询统计信息
func (p *Proxy) processStats() {
	defer p.wg.Done()

	for stats := range p.statsChan {
		// 计算查询执行时间（毫秒）
		executionTime := time.Since(stats.StartTime).Milliseconds()

		// 记录查询日志 - 正确设置所有必要字段
		logEntry := logger.LogEntry{
			SourceIP:     stats.SourceIP,
			QueryContent: stats.QueryContent,
			QueryTime:    executionTime,
			AdditionalInfo: []byte(fmt.Sprintf(`{
				"execution_time_ms": %d
			}`, executionTime)),
		}

		// 根据执行时间选择日志级别
		if executionTime > 1000 {
			p.log.Warn(logEntry)
		} else if executionTime > 500 {
			p.log.Info(logEntry)
		} else {
			p.log.Debug(logEntry)
		}

		// 如果Kafka配置启用，发送查询日志到Kafka
		if p.kafkaEnabled && p.kafkaClient != nil {
			queryLog := &kafka.QueryLog{
				SourceIP:      stats.SourceIP,
				QueryContent:  stats.QueryContent,
				ExecutionTime: executionTime,
			}
			
			// 异步发送日志到Kafka
			go func() {
				err := p.kafkaClient.SendQueryLog(queryLog)
				if err != nil {
					p.log.Error(logger.LogEntry{
						SourceIP:     stats.SourceIP,
						ErrorMessage: fmt.Sprintf("发送到Kafka失败: %v", err),
					})
				}
			}()
		}
	}
}