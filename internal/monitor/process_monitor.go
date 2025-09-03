package monitor

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"faraday/internal/config"
	"faraday/internal/logger"
)

// ProcessMonitor 进程监控器
type ProcessMonitor struct {
	config       *config.Config
	configPath   string
	logger       *logger.Logger
	processes    map[string]*MonitoredProcess
	mutex        sync.Mutex
	stopChan     chan struct{}
	wg           sync.WaitGroup
}

// MonitoredProcess 被监控的进程
type MonitoredProcess struct {
	ProxyConfig  *config.ProxyConfig
	configPath   string
	cmd          *exec.Cmd
	logger       *logger.Logger
	running      bool
	restartCount int
	mutex        sync.Mutex
	restartChan  chan struct{}
}

// NewProcessMonitor 创建一个新的进程监控器
func NewProcessMonitor(config *config.Config, configPath string, logger *logger.Logger) *ProcessMonitor {
	return &ProcessMonitor{
		config:    config,
		configPath: configPath,
		logger:    logger,
		processes: make(map[string]*MonitoredProcess),
		stopChan:  make(chan struct{}),
	}
}

// Start 启动进程监控器
func (pm *ProcessMonitor) Start() error {
	// 检查并启动配置中定义的所有代理进程
	for _, proxyConfig := range pm.config.Proxies {
		proxyConfig := proxyConfig // 捕获循环变量
		if err := pm.StartProxy(&proxyConfig); err != nil {
			pm.logger.Error(logger.LogEntry{
				ProxyName:    proxyConfig.ProxyName,
				ErrorMessage: fmt.Sprintf("启动代理失败: %v", err),
			})
		}
	}

	// 启动监控循环
	pm.wg.Add(1)
	go pm.monitorLoop()

	return nil
}

// Stop 停止进程监控器
func (pm *ProcessMonitor) Stop() {
	close(pm.stopChan)
	
	// 停止所有被监控的进程
	pm.mutex.Lock()
	defer pm.mutex.Unlock()
	for _, process := range pm.processes {
		process.Stop()
	}
	
	pm.wg.Wait()
}

// StartProxy 启动一个代理进程
func (pm *ProcessMonitor) StartProxy(proxyConfig *config.ProxyConfig) error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	// 检查进程是否已经存在
	if process, exists := pm.processes[proxyConfig.ProxyName]; exists {
		// 如果进程已经存在且正在运行，则不需要启动
		if process.running {
			pm.logger.Info(logger.LogEntry{
				ProxyName:    proxyConfig.ProxyName,
				AdditionalInfo: []byte(fmt.Sprintf(`{"event":"process_already_running", "pid":%d}`, process.cmd.Process.Pid)),
			})
			return nil
		}
	}

	// 创建进程日志记录器
	processLogger := logger.NewLogger(os.Stdout, logger.INFO, proxyConfig.ProxyName)

	// 创建被监控的进程
	process := &MonitoredProcess{
		ProxyConfig: proxyConfig,
		logger:      processLogger,
		running:     false,
		restartChan: make(chan struct{}),
		configPath:  pm.configPath,
	}

	// 启动进程
	if err := process.Start(); err != nil {
		return err
	}

	// 添加到进程映射
	pm.processes[proxyConfig.ProxyName] = process

	// 启动进程监控
	pm.wg.Add(1)
	go pm.monitorProcess(process)

	return nil
}

// monitorLoop 监控循环，定期检查进程状态
func (pm *ProcessMonitor) monitorLoop() {
	defer pm.wg.Done()

	ticker := time.NewTicker(5 * time.Second) // 每5秒检查一次
	defer ticker.Stop()

	for {
		select {
		case <-pm.stopChan:
			return
		case <-ticker.C:
			// 检查所有进程状态
			pm.checkProcesses()
		}
	}
}

// checkProcesses 检查所有进程的状态
func (pm *ProcessMonitor) checkProcesses() {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	for _, process := range pm.processes {
		process.mutex.Lock()
		if !process.running && process.restartCount < 5 { // 限制重启次数，防止无限重启
			process.mutex.Unlock()
			pm.logger.Warn(logger.LogEntry{
				ProxyName:    process.ProxyConfig.ProxyName,
				AdditionalInfo: []byte(fmt.Sprintf(`{"event":"process_not_running", "restart_count":%d}`, process.restartCount)),
			})
			// 尝试重启进程
			if err := process.Start(); err != nil {
				pm.logger.Error(logger.LogEntry{
					ProxyName:    process.ProxyConfig.ProxyName,
					ErrorMessage: fmt.Sprintf("重启进程失败: %v", err),
				})
			} else {
				pm.wg.Add(1)
				go pm.monitorProcess(process)
			}
		} else {
			process.mutex.Unlock()
		}
	}
}

// monitorProcess 监控单个进程
func (pm *ProcessMonitor) monitorProcess(process *MonitoredProcess) {
	defer pm.wg.Done()

	// 等待进程退出或监控器停止
	select {
	case <-pm.stopChan:
		// 监控器停止，无需处理
	case <-process.restartChan:
		// 进程退出，尝试重启
		pm.logger.Info(logger.LogEntry{
			ProxyName:    process.ProxyConfig.ProxyName,
			AdditionalInfo: []byte(`{"event":"process_exited"}`),
		})
		
		// 延迟重启，避免频繁重启
		time.Sleep(1 * time.Second)
		
		// 尝试重启进程
		process.mutex.Lock()
		process.restartCount++
		process.mutex.Unlock()
		
		if err := process.Start(); err != nil {
			pm.logger.Error(logger.LogEntry{
				ProxyName:    process.ProxyConfig.ProxyName,
				ErrorMessage: fmt.Sprintf("重启进程失败: %v", err),
			})
		} else {
			pm.wg.Add(1)
			go pm.monitorProcess(process)
		}
	}
}

// Start 启动被监控的进程
func (p *MonitoredProcess) Start() error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// 获取当前可执行文件路径
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取可执行文件路径失败: %w", err)
	}
	
	// 构建faraday可执行文件路径（假设在同一目录下）
	faradayPath := filepath.Join(filepath.Dir(exePath), "faraday")

	// 创建命令
	cmdArgs := []string{
		"--proxy-name", p.ProxyConfig.ProxyName,
		"--listen-port", fmt.Sprintf("%d", p.ProxyConfig.ListenPort),
		"--target-host", p.ProxyConfig.TargetHost,
		"--target-port", fmt.Sprintf("%d", p.ProxyConfig.TargetPort),
		"--buffer-size", fmt.Sprintf("%d", p.ProxyConfig.BufferSize),
		"--log-level", p.ProxyConfig.LogLevel,
	}
	
	// 如果有配置文件路径，添加到命令参数中
	if p.configPath != "" {
		cmdArgs = append(cmdArgs, "--config", p.configPath)
	}
	
	cmd := exec.Command(faradayPath, cmdArgs...)
	

	// 设置标准输出和标准错误
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("获取标准输出管道失败: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("获取标准错误管道失败: %w", err)
	}

	// 启动命令
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动进程失败: %w", err)
	}

	// 更新进程状态
	p.cmd = cmd
	p.running = true

	// 记录进程启动信息
	p.logger.Info(logger.LogEntry{
		AdditionalInfo: []byte(fmt.Sprintf(`{"event":"process_started", "pid":%d}`, cmd.Process.Pid)),
	})

	// 启动协程读取标准输出
	go p.readOutput(stdout)
	go p.readOutput(stderr)

	// 启动协程等待进程退出
	go func() {
		cmd.Wait()
		p.mutex.Lock()
		p.running = false
		p.mutex.Unlock()
		// 发送重启信号
		p.restartChan <- struct{}{}
	}()

	return nil
}

// Stop 停止被监控的进程
func (p *MonitoredProcess) Stop() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.running && p.cmd != nil && p.cmd.Process != nil {
		// 尝试优雅停止进程
		if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
			// 如果优雅停止失败，强制终止进程
			log.Printf("优雅停止进程失败: %v, 尝试强制终止", err)
			p.cmd.Process.Kill()
		}
	}
}

// readOutput 读取进程的输出
func (p *MonitoredProcess) readOutput(output io.ReadCloser) {
	reader := bufio.NewReader(output)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			// 输出流关闭，退出循环
			break
		}
		// 处理进程输出，这里我们简单地打印到控制台
		// 在实际应用中，可能需要根据日志格式进行解析和处理
		fmt.Printf("[%s] %s", p.ProxyConfig.ProxyName, line)
	}
}

// CheckAndRestartDaemon 检查守护进程状态并在需要时重启
func CheckAndRestartDaemon(configPath string, logLevel string) {
	// 这个函数会被faraday进程调用，用于检查守护进程是否运行
	// 实现思路：检查pid文件或使用进程间通信来确认守护进程状态
	// 如果守护进程未运行，则尝试启动一个新的守护进程实例
	
	// 保存配置参数，用于重启时使用
	if configPath == "" {
		configPath = "configs/faraday.yml"
	}
	if logLevel == "" {
		logLevel = "INFO"
	}
	
	go func() {
		for {
			time.Sleep(30 * time.Second) // 每30秒检查一次
			
			// 检查守护进程是否运行
			// 这里简化处理，实际应该通过pid文件或其他方式检查
			// 守护进程名称为faraday-daemon
			cmd := exec.Command("pgrep", "faraday-daemon")
			if err := cmd.Run(); err != nil {
				// 守护进程未运行，尝试启动
				log.Println("检测到守护进程未运行，尝试重新启动...")
				
				// 获取当前可执行文件路径
				exePath, err := os.Executable()
				if err != nil {
					log.Printf("获取可执行文件路径失败: %v", err)
					continue
				}
				
				// 构建守护进程可执行文件路径（假设在同一目录下）
			daemonPath := filepath.Join(filepath.Dir(exePath), "faraday-daemon")
				
				// 启动守护进程，传递配置文件路径和日志级别参数
				cmd := exec.Command(daemonPath, "--config", configPath, "--log-level", logLevel)
				if err := cmd.Start(); err != nil {
					log.Printf("启动守护进程失败: %v", err)
				} else {
					log.Printf("守护进程已启动，PID: %d", cmd.Process.Pid)
				}
			}
		}
	}()
}