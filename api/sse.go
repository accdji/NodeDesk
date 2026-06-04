package api

import (
	"encoding/json"
	"sync"
	"time"
)

// LogBuffer 收集日志行，定时或达阈值时批量推送
type LogBuffer struct {
	mu      sync.Mutex
	lines   []logLine
	ticker  *time.Ticker
	onFlush func([]logLine)
	done    chan struct{}
}

type logLine struct {
	Type      string `json:"type"`
	RunID     string `json:"run_id"`
	StepName  string `json:"step_name"`
	Timestamp string `json:"timestamp"`
	Message   string `json:"message"`
}

// NewLogBuffer 创建日志缓冲区，每隔 interval 或累积 maxLines 条时刷新
func NewLogBuffer(interval time.Duration, maxLines int, onFlush func([]logLine)) *LogBuffer {
	b := &LogBuffer{
		lines:   make([]logLine, 0, maxLines),
		ticker:  time.NewTicker(interval),
		onFlush: onFlush,
		done:    make(chan struct{}),
	}
	go b.flushLoop()
	return b
}

func (b *LogBuffer) Add(runID, stepName, timestamp, message string) {
	b.mu.Lock()
	b.lines = append(b.lines, logLine{Type: "log", RunID: runID, StepName: stepName, Timestamp: timestamp, Message: message})
	shouldFlush := len(b.lines) >= 10
	b.mu.Unlock()
	if shouldFlush {
		b.flush()
	}
}

func (b *LogBuffer) flushLoop() {
	for {
		select {
		case <-b.ticker.C:
			b.flush()
		case <-b.done:
			b.ticker.Stop()
			return
		}
	}
}

func (b *LogBuffer) flush() {
	b.mu.Lock()
	if len(b.lines) == 0 {
		b.mu.Unlock()
		return
	}
	lines := b.lines
	b.lines = make([]logLine, 0, cap(b.lines))
	b.mu.Unlock()

	if b.onFlush != nil {
		b.onFlush(lines)
	}
}

// Stop 停止定时器并刷新剩余日志
func (b *LogBuffer) Stop() {
	close(b.done)
	b.flush()
}

// logBuffers 管理每个 run 的 LogBuffer
var (
	logBuffersMu sync.Mutex
	logBuffers   = make(map[string]*LogBuffer)
)

func getOrCreateLogBuffer(runID string, onFlush func([]logLine)) *LogBuffer {
	logBuffersMu.Lock()
	defer logBuffersMu.Unlock()
	if lb, ok := logBuffers[runID]; ok {
		return lb
	}
	lb := NewLogBuffer(500*time.Millisecond, 10, onFlush)
	logBuffers[runID] = lb
	return lb
}

func removeLogBuffer(runID string) {
	logBuffersMu.Lock()
	defer logBuffersMu.Unlock()
	if lb, ok := logBuffers[runID]; ok {
		lb.Stop()
		delete(logBuffers, runID)
	}
}

// batchBroadcastSSE 将缓冲的日志批量编码为一条 SSE 消息发送
func batchBroadcastSSE(lines []logLine) {
	if len(lines) == 0 {
		return
	}
	data, _ := json.Marshal(lines)
	msg := string(data)

	sseMu.Lock()
	defer sseMu.Unlock()
	// 使用第一条日志的 runID 查找客户端
	runID := lines[0].RunID
	for _, c := range sseClients[runID] {
		select {
		case c.ch <- msg:
		default:
		}
	}
}
