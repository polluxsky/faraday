package logger

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

// LogLevel 定义日志级别
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

// Logger 自定义日志记录器
type Logger struct {
	level     LogLevel
	writer    io.Writer
	proxyName string
	buffer    *bufio.Writer
	mutex     sync.Mutex
	isConsole bool // 标记是否为控制台输出
}

// LogEntry 定义日志条目结构
type LogEntry struct {
	Timestamp     string          `json:"timestamp"`
	Level         string          `json:"level"`
	ProxyName     string          `json:"proxy_name"`
	SourceIP      string          `json:"source_ip,omitempty"`
	QueryTime     int64           `json:"query_time,omitempty"` // 毫秒
	QueryContent  string          `json:"query_content,omitempty"`
	RowCount      int64           `json:"row_count,omitempty"`
	ErrorMessage  string          `json:"error_message,omitempty"`
	AdditionalInfo json.RawMessage `json:"additional_info,omitempty"`
}

// NewLogger 创建一个新的日志记录器
func NewLogger(writer io.Writer, level LogLevel, proxyName string) *Logger {
	buffer := bufio.NewWriter(writer)
	isConsole := false
	// 检查是否为标准输出（控制台）
	if writer == os.Stdout || writer == os.Stderr {
		isConsole = true
	}
	return &Logger{
		level:     level,
		writer:    writer,
		proxyName: proxyName,
		buffer:    buffer,
		isConsole: isConsole,
	}
}

// NewConsoleLogger 创建一个控制台日志记录器
func NewConsoleLogger(level LogLevel, proxyName string) *Logger {
	logger := NewLogger(os.Stdout, level, proxyName)
	logger.isConsole = true
	return logger
}

// NewFileLogger 创建一个文件日志记录器
func NewFileLogger(filePath string, level LogLevel, proxyName string) (*Logger, error) {
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	logger := NewLogger(file, level, proxyName)
	logger.isConsole = false
	return logger, nil
}

// logLevelToString 将日志级别转换为字符串
func logLevelToString(level LogLevel) string {
	switch level {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// log 记录一条日志
func (l *Logger) log(level LogLevel, entry LogEntry) {
	// 检查日志级别
	if level < l.level {
		return
	}

	// 设置日志级别和时间戳
	entry.Level = logLevelToString(level)
	entry.Timestamp = time.Now().Format("2006-01-02 15:04:05.000")
	entry.ProxyName = l.proxyName

	// 预处理QueryContent字段，将特殊字符转换为可读形式
	if entry.QueryContent != "" {
		// 创建一个新的字符串构建器
		var sb strings.Builder
		sb.Grow(len(entry.QueryContent) * 2) // 预分配空间以提高性能
		
		// 遍历原始字符串的每个字符
		for _, char := range entry.QueryContent {
			// 处理特殊字符
			switch {
			case char < 32 && char != '\n' && char != '\r' && char != '\t':
				// 只处理控制字符，保留换行符、回车符和制表符的原始形式
				sb.WriteString(fmt.Sprintf("\\u%04X", uint16(char)))
			case char == 127:
				// 处理DEL字符
				sb.WriteString(fmt.Sprintf("\\u%04X", uint16(char)))
			default:
				// 其他字符保持不变（包括换行符、回车符和制表符）
				sb.WriteRune(char)
			}
		}
		
		// 创建临时日志条目，避免修改原始entry
		tempEntry := entry
		tempEntry.QueryContent = sb.String()
		entry = tempEntry
	}

	// 加锁确保并发安全
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if l.isConsole {
		// 控制台输出：使用更友好的格式，直接显示控制字符效果
		var logLine string
		if entry.QueryContent != "" {
			// 对于查询日志，使用更清晰的格式
			logLine = fmt.Sprintf("%s [%s] [%s] Source: %s, Query Time: %dms, Rows: %d\nQuery:\n%s\n",
				entry.Timestamp, entry.Level, entry.ProxyName,
				entry.SourceIP, entry.QueryTime, entry.RowCount,
				entry.QueryContent)
		} else if entry.ErrorMessage != "" {
			// 错误日志
			logLine = fmt.Sprintf("%s [%s] [%s] Error: %s\nAdditional Info: %s\n",
				entry.Timestamp, entry.Level, entry.ProxyName,
				entry.ErrorMessage, string(entry.AdditionalInfo))
		} else {
			// 通用日志格式
			// 处理AdditionalInfo字段，确保显示为可读的JSON字符串
			additionalInfoStr := ""
			if len(entry.AdditionalInfo) > 0 {
				// 将json.RawMessage转换为可读字符串
				var m map[string]interface{}
				if err := json.Unmarshal(entry.AdditionalInfo, &m); err == nil {
					// 如果解析成功，使用格式化的JSON
					if prettyJSON, err := json.MarshalIndent(m, "", "  "); err == nil {
						additionalInfoStr = string(prettyJSON)
					} else {
						additionalInfoStr = string(entry.AdditionalInfo)
					}
				} else {
					// 如果解析失败，直接使用原始字符串
					additionalInfoStr = string(entry.AdditionalInfo)
				}
			}
			
			// 构建自定义的日志行，避免使用%+v导致AdditionalInfo显示为十六进制
			logLine := fmt.Sprintf("%s [%s] [%s] Source: %s, Query Time: %dms, Rows: %d\n", entry.Timestamp, entry.Level, entry.ProxyName, entry.SourceIP, entry.QueryTime, entry.RowCount)
			// 如果有AdditionalInfo，添加到日志中
			if additionalInfoStr != "" {
				logLine += fmt.Sprintf("Additional Info:\n%s\n", additionalInfoStr)
			}
		}
		
		// 直接写入标准输出，不进行JSON序列化
		_, err := l.writer.Write([]byte(logLine))
		if err != nil {
			log.Printf("写入控制台日志失败: %v\n", err)
		}
	} else {
		// 文件输出：保持原有的JSON格式
		// 创建缓冲区存储JSON数据
		buffer := &strings.Builder{}

		// 创建JSON编码器并配置
		encoder := json.NewEncoder(buffer)
		encoder.SetEscapeHTML(false) // 不转义HTML字符
		encoder.SetIndent("", "")  // 不缩进

		// 序列化日志条目
		err := encoder.Encode(entry)
		if err != nil {
			// 如果JSON序列化失败，使用简单格式记录
			log.Printf("%s [%s] [%s] 日志序列化失败: %v\n", 
				entry.Timestamp, entry.Level, entry.ProxyName, err)
			return
		}

		// 获取JSON字符串并移除末尾换行符
		jsonStr := buffer.String()
		if len(jsonStr) > 0 && jsonStr[len(jsonStr)-1] == '\n' {
			jsonStr = jsonStr[:len(jsonStr)-1]
		}

		// 写入日志，保持统一格式
		logLine := fmt.Sprintf("%s [%s] [%s] %s\n", 
			entry.Timestamp, entry.Level, entry.ProxyName, jsonStr)

		// 写入缓冲
		written, err := l.buffer.WriteString(logLine)
		if err != nil || written != len(logLine) {
			// 如果缓冲写入失败，尝试直接写入
			_, err := l.writer.Write([]byte(logLine))
			if err != nil {
				log.Printf("写入日志失败: %v\n", err)
			}
			return
		}

		// 定期刷新缓冲
		if l.buffer.Buffered() > 4096 {
			l.buffer.Flush()
		}
	}
}

// Flush 刷新日志缓冲区
func (l *Logger) Flush() {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.buffer.Flush()
}

// Debug 记录调试日志
func (l *Logger) Debug(entry LogEntry) {
	l.log(DEBUG, entry)
}

// Info 记录信息日志
func (l *Logger) Info(entry LogEntry) {
	l.log(INFO, entry)
}

// Warn 记录警告日志
func (l *Logger) Warn(entry LogEntry) {
	l.log(WARN, entry)
}

// Error 记录错误日志
func (l *Logger) Error(entry LogEntry) {
	l.log(ERROR, entry)
}

// LogMySQLQuery 记录MySQL查询日志
func (l *Logger) LogMySQLQuery(sourceIP, queryContent string, queryTime, rowCount int64) {
	entry := LogEntry{
		SourceIP:     sourceIP,
		QueryContent: queryContent,
		QueryTime:    queryTime,
		RowCount:     rowCount,
	}
	l.Info(entry)
}

// LogError 记录错误日志
func (l *Logger) LogError(errMsg string, additionalInfo json.RawMessage) {
	entry := LogEntry{
		ErrorMessage:  errMsg,
		AdditionalInfo: additionalInfo,
	}
	l.Error(entry)
}