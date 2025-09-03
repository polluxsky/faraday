package utils

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"strings"
)

// GetSourceIP 从net.Addr中提取IP地址
func GetSourceIP(addr net.Addr) string {
	if addr == nil {
		return "unknown"
	}

	// 提取IP地址部分
	ipStr := addr.String()
	if idx := strings.LastIndex(ipStr, ":"); idx != -1 {
		ipStr = ipStr[:idx]
	}

	// 移除可能的方括号（IPv6地址）
	ipStr = strings.TrimPrefix(ipStr, "[")
	ipStr = strings.TrimSuffix(ipStr, "]")

	return ipStr
}

// GenerateRandomID 生成一个随机的ID字符串
func GenerateRandomID(length int) (string, error) {
	bytes := make([]byte, length/2)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// IsValidIP 检查字符串是否是有效的IP地址
func IsValidIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	return ip != nil
}

// GetLocalIP 获取本机的IP地址
func GetLocalIP() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	for _, iface := range interfaces {
		// 跳过_down状态的接口和loopback接口
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addresses, err := iface.Addrs()
		if err != nil {
			return "", err
		}

		for _, addr := range addresses {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			// 跳过loopback地址和IPv6链路本地地址
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}

			// 返回IPv4地址或IPv6全局地址
			ip = ip.To4()
			if ip != nil {
				return ip.String(), nil
			}
		}
	}

	return "", net.UnknownNetworkError("无法找到有效的本机IP地址")
}

// ParseCommandLineArgs 解析命令行参数
func ParseCommandLineArgs(args []string) map[string]string {
	result := make(map[string]string)

	for i := 0; i < len(args); i++ {
		// 跳过非选项参数
		if !strings.HasPrefix(args[i], "--") {
			continue
		}

		// 移除前缀
		key := strings.TrimPrefix(args[i], "--")

		// 检查是否有值
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
			result[key] = args[i+1]
			i++ // 跳过值
		} else {
			// 没有值，设置为空字符串或布尔值
			result[key] = "true"
		}
	}

	return result
}

// LimitStringLength 限制字符串长度
func LimitStringLength(s string, maxLength int) string {
	if len(s) <= maxLength {
		return s
	}
	return s[:maxLength] + "..."
}

// FormatDuration 格式化持续时间为可读字符串
func FormatDuration(duration int64) string {
	if duration < 1000 {
		return fmt.Sprintf("%dms", duration)
	}
	seconds := duration / 1000
	milliseconds := duration % 1000
	if seconds < 60 {
		return fmt.Sprintf("%d.%03ds", seconds, milliseconds)
	}
	minutes := seconds / 60
	seconds = seconds % 60
	return fmt.Sprintf("%dm%02d.%03ds", minutes, seconds, milliseconds)
}

// FormatBytes 格式化字节数为可读字符串
func FormatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
		TB = 1024 * GB
	)

	if bytes < KB {
		return fmt.Sprintf("%dB", bytes)
	} else if bytes < MB {
		return fmt.Sprintf("%.2fKB", float64(bytes)/KB)
	} else if bytes < GB {
		return fmt.Sprintf("%.2fMB", float64(bytes)/MB)
	} else if bytes < TB {
		return fmt.Sprintf("%.2fGB", float64(bytes)/GB)
	} else {
		return fmt.Sprintf("%.2fTB", float64(bytes)/TB)
	}
}

// EscapeSpecialChars 转义特殊字符，用于日志输出
func EscapeSpecialChars(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}

// TranscodeString 对字符串进行转码处理，替换特殊字符
// transcodeItems 是一个映射，键是要替换的字符，值是替换后的字符
func TranscodeString(s string, transcodeItems map[rune]string) string {
	if transcodeItems == nil {
		return s
	}

	var result strings.Builder
	for _, char := range s {
		// 检查是否需要转码
		if replacement, exists := transcodeItems[char]; exists {
			result.WriteString(replacement)
		} else {
			result.WriteRune(char)
		}
	}

	return result.String()
}

// DefaultSQLTranscoder 获取默认的SQL字符串转码映射
func DefaultSQLTranscoder() map[rune]string {
	return map[rune]string{
		'\n': "\\n", // 换行符
		'\r': "\\r", // 回车符
		'\t': "\\t", // 制表符
		'"': "\\\"", // 双引号
		'\'': "\\'", // 单引号
		'\\': "\\\\", // 反斜杠
	}
}