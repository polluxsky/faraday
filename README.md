# Faraday

Faraday 是一个高性能的 MySQL 代理工具，包含两个核心组件：faraday（MySQL代理）和 faraday-deamon（进程管理守护进程）。支持将 MySQL 查询日志发送到 Kafka 进行实时分析。

## 功能特点

### faraday（MySQL代理）
- 作为标准的 MySQL 代理，将客户端请求转发到目标 MySQL 服务器
- 记录详细的请求日志，包括：请求来源 IP、请求时间、请求内容、请求返回行数、请求执行耗时等
- 日志以 JSON 格式输出，采用仿 log4j 输出格式
- 完整解析 MySQL 各类通讯包
- 异步请求统计，避免影响 MySQL 请求性能
- 可配置的缓冲区大小
- 当 faraday-deamon 异常退出时，能自动拉起守护进程
- 支持将查询日志异步发送到 Kafka，用于实时分析和监控

### faraday-deamon（进程管理守护进程）
- 根据配置文件拉起指定数量的 faraday 代理进程
- 监控 faraday 进程运行状态，一旦有进程异常退出，自动重新拉起
- 统一管理和区分不同 faraday 进程的日志输出
- 支持作为系统守护进程运行

## 项目结构

```
├── cmd/                  # 可执行文件入口
│   ├── faraday/          # MySQL代理入口
│   └── faraday-daemon/   # 守护进程入口
├── internal/             # 内部包
│   ├── config/           # 配置处理
│   ├── logger/           # 日志处理
│   ├── mysql/            # MySQL协议和代理实现
│   └── monitor/          # 进程监控
├── pkg/                  # 可公开复用的包
│   └── utils/            # 通用工具函数
├── configs/              # 配置文件目录
├── go.mod                # Go模块定义
└── go.sum                # 依赖版本锁定
```

## 配置文件

配置文件位于 `configs/faraday.yml`，示例配置：

```yaml
# faraday-deamon configuration
# This file configures the MySQL proxies that faraday-deamon should manage

# Kafka 配置（可选）
# kafka:
#   brokers: ["localhost:9092"]  # Kafka 服务器地址列表
#   topic: "mysql_query_logs"   # 发送查询日志的主题

proxies:
  - proxy_name: "mysql_proxy_1"
    listen_port: 3307
    target_host: "localhost"
    target_port: 3306
    buffer_size: 1000
    log_level: "INFO"
  
  - proxy_name: "mysql_proxy_2"
    listen_port: 3308
    target_host: "localhost"
    target_port: 3306
    buffer_size: 1000
    log_level: "INFO"

# Deamon configuration
daemon:
  log_file: "/var/log/faraday-deamon.log"
  pid_file: "/var/run/faraday-deamon.pid"
```

## 构建和运行

### 构建项目

```bash
# 在项目根目录执行
cd /Users/pollux.qu/go/faraday

# 构建 faraday
go build -o faraday ./cmd/faraday

# 构建 faraday-deamon
go build -o faraday-deamon ./cmd/faraday-daemon
```

### 运行 faraday-deamon

```bash
# 直接运行（前台模式）
./faraday-deamon

# 作为守护进程运行
./faraday-deamon --detach

# 指定配置文件路径
./faraday-deamon --config /path/to/faraday.yml
```

### 单独运行 faraday

通常情况下，faraday 由 faraday-deamon 自动管理和启动，但您也可以手动运行：

```bash
# 手动运行 faraday（指定参数）
./faraday --proxy-name test_proxy --listen-port 3307 --target-host localhost --target-port 3306
```

## 日志格式

日志采用 JSON 格式，示例：

```
2023-10-20 15:30:45.123 [INFO] [mysql_proxy_1] {"timestamp":"2023-10-20 15:30:45.123","level":"INFO","proxy_name":"mysql_proxy_1","source_ip":"192.168.1.100","query_time":125,"query_content":"SELECT * FROM users LIMIT 10","row_count":10}
```

## Kafka 日志格式

当启用 Kafka 支持时，发送到 Kafka 的查询日志格式如下：

```json
{
  "source_ip": "192.168.1.100",
  "query_content": "SELECT * FROM users LIMIT 10",
  "execution_time_ms": 125,
  "timestamp": "2023-10-20T15:30:45.123Z"
}
```

## 注意事项

1. 确保目标 MySQL 服务器正在运行且可访问
2. 首次运行时可能需要创建日志文件目录并设置适当的权限
3. 配置文件中的端口号确保未被其他程序占用
4. 在生产环境中建议使用 `--detach` 参数将 faraday-deamon 作为守护进程运行

## 许可证

[MIT License](LICENSE)