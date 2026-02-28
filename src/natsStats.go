// Package src 提供 NATS 通訊統計追蹤，用於定期 STATUS 日誌輸出。
//
// 此檔案定義 NatsStats 結構與相關方法，用於追蹤 NATS 請求/回應次數、
// 錯誤次數與最後活動時間，並提供並行安全的計數器更新與查詢。
package src

import (
	"sync"
	"sync/atomic"
	"time"
)

// NatsStats 保存 NATS 通訊的運行統計資訊。
//
// Requests、Replies、Errors 使用 atomic 操作以確保跨 goroutine 的並行安全。
// LastActivity 受 mu 保護。
type NatsStats struct {
	Requests int64
	Replies  int64
	Errors   int64

	mu           sync.Mutex
	LastActivity time.Time
}

// globalNatsStats 是全域的 NATS 統計追蹤器實例。
var globalNatsStats NatsStats

// GetNatsStats 回傳全域 NATS 統計追蹤器的指標。
func GetNatsStats() *NatsStats {
	return &globalNatsStats
}

// RecordRequest 記錄一次 NATS 請求已發送。
func (s *NatsStats) RecordRequest() {
	atomic.AddInt64(&s.Requests, 1)
	s.mu.Lock()
	s.LastActivity = time.Now()
	s.mu.Unlock()
}

// RecordReply 記錄一次 NATS 回應已接收。
func (s *NatsStats) RecordReply() {
	atomic.AddInt64(&s.Replies, 1)
	s.mu.Lock()
	s.LastActivity = time.Now()
	s.mu.Unlock()
}

// RecordError 記錄一次 NATS 請求失敗。
func (s *NatsStats) RecordError() {
	atomic.AddInt64(&s.Errors, 1)
	s.mu.Lock()
	s.LastActivity = time.Now()
	s.mu.Unlock()
}

// NatsStatsSnapshot 回傳目前統計值的快照。
type NatsStatsSnapshot struct {
	Requests     int64
	Replies      int64
	Errors       int64
	LastActivity time.Time
}

// Snapshot 回傳目前統計值的唯讀快照。
func (s *NatsStats) Snapshot() NatsStatsSnapshot {
	s.mu.Lock()
	la := s.LastActivity
	s.mu.Unlock()
	return NatsStatsSnapshot{
		Requests:     atomic.LoadInt64(&s.Requests),
		Replies:      atomic.LoadInt64(&s.Replies),
		Errors:       atomic.LoadInt64(&s.Errors),
		LastActivity: la,
	}
}
