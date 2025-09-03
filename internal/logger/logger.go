package logger

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
	"os"
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
	return &Logger{
		level:     level,
		writer:    writer,
		proxyName: proxyName,
		buffer:    buffer,
	}
}

// NewConsoleLogger 创建一个控制台日志记录器
func NewConsoleLogger(level LogLevel, proxyName string) *Logger {
	return NewLogger(os.Stdout, level, proxyName)
}

// NewFileLogger 创建一个文件日志记录器
func NewFileLogger(filePath string, level LogLevel, proxyName string) (*Logger, error) {
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	return NewLogger(file, level, proxyName), nil
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

	// 转换为JSON
	logData, err := json.Marshal(entry)
	if err != nil {
		// 如果JSON序列化失败，使用简单格式记录
		l.mutex.Lock()
		defer l.mutex.Unlock()
		log.Printf("%s [%s] [%s] 日志序列化失败: %v\n", 
			entry.Timestamp, entry.Level, entry.ProxyName, err)
		return
	}

	// 写入日志，仿log4j格式
	logLine := entry.Timestamp + " [" + entry.Level + "] [" + entry.ProxyName + "] " + string(logData) + "\n"

	// 加锁确保并发安全
	l.mutex.Lock()
	defer l.mutex.Unlock()

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