package service

import (
	"context"
	"log"
	"sync"
	"time"
)

// DeferredService 提供延迟批量更新能力。
type DeferredService struct {
	accountRepo AccountRepository
	timingWheel *TimingWheelService
	interval    time.Duration

	lastUsedUpdates sync.Map

	telemetryPrivacyMu      sync.Mutex
	telemetryPrivacyUpdates map[int64]int64
}

type accountExtraCounterRepository interface {
	IncrementExtraCounter(ctx context.Context, id int64, key string, delta int64) error
}

// NewDeferredService 创建延迟批量更新服务。
func NewDeferredService(accountRepo AccountRepository, timingWheel *TimingWheelService, interval time.Duration) *DeferredService {
	return &DeferredService{
		accountRepo:             accountRepo,
		timingWheel:             timingWheel,
		interval:                interval,
		telemetryPrivacyUpdates: make(map[int64]int64),
	}
}

// Start 启动延迟批量更新服务。
func (s *DeferredService) Start() {
	s.timingWheel.ScheduleRecurring("deferred:last_used", s.interval, s.flushLastUsed)
	s.timingWheel.ScheduleRecurring("deferred:telemetry_privacy_protected_count", s.interval, s.flushTelemetryPrivacyProtectionCounts)
	log.Printf("[DeferredService] 已启动（间隔：%v）", s.interval)
}

// Stop 停止延迟批量更新服务，并尽力刷出内存中的待写数据。
func (s *DeferredService) Stop() {
	s.timingWheel.Cancel("deferred:last_used")
	s.timingWheel.Cancel("deferred:telemetry_privacy_protected_count")
	s.flushLastUsed()
	s.flushTelemetryPrivacyProtectionCounts()
	log.Printf("[DeferredService] 服务已停止")
}

func (s *DeferredService) ScheduleLastUsedUpdate(accountID int64) {
	s.lastUsedUpdates.Store(accountID, time.Now())
}

// ScheduleTelemetryPrivacyProtection 累加账号级遥测隐私保护次数。
// 该方法只记录账号 ID 和次数，不记录请求头、metadata.user_id、设备 ID、会话 ID 或正文内容。
// 计数先缓存在内存里，再由 flushTelemetryPrivacyProtectionCounts 批量写入数据库，
// 避免每次请求都在网关热路径执行数据库写入。
func (s *DeferredService) ScheduleTelemetryPrivacyProtection(accountID int64) {
	if s == nil || accountID <= 0 {
		return
	}
	s.telemetryPrivacyMu.Lock()
	defer s.telemetryPrivacyMu.Unlock()
	if s.telemetryPrivacyUpdates == nil {
		s.telemetryPrivacyUpdates = make(map[int64]int64)
	}
	s.telemetryPrivacyUpdates[accountID]++
}

func (s *DeferredService) flushLastUsed() {
	updates := make(map[int64]time.Time)
	s.lastUsedUpdates.Range(func(key, value any) bool {
		id, ok := key.(int64)
		if !ok {
			return true
		}
		ts, ok := value.(time.Time)
		if !ok {
			return true
		}
		updates[id] = ts
		s.lastUsedUpdates.Delete(key)
		return true
	})

	if len(updates) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.accountRepo.BatchUpdateLastUsed(ctx, updates); err != nil {
		log.Printf("[DeferredService] last_used 批量写入失败（账号数=%d）：%v", len(updates), err)
		for id, ts := range updates {
			s.lastUsedUpdates.Store(id, ts)
		}
	} else {
		log.Printf("[DeferredService] 已写入 %d 个账号的 last_used", len(updates))
	}
}

func (s *DeferredService) flushTelemetryPrivacyProtectionCounts() {
	if s == nil {
		return
	}
	repo, ok := s.accountRepo.(accountExtraCounterRepository)
	if !ok || repo == nil {
		return
	}

	s.telemetryPrivacyMu.Lock()
	updates := s.telemetryPrivacyUpdates
	s.telemetryPrivacyUpdates = make(map[int64]int64)
	s.telemetryPrivacyMu.Unlock()

	if len(updates) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	failed := make(map[int64]int64)
	for id, delta := range updates {
		if delta <= 0 {
			continue
		}
		if err := repo.IncrementExtraCounter(ctx, id, AccountExtraTelemetryPrivacyProtectedCount, delta); err != nil {
			log.Printf("[DeferredService] 遥测隐私保护计数写入失败（账号=%d 增量=%d）：%v", id, delta, err)
			failed[id] += delta
		}
	}
	if len(failed) == 0 {
		log.Printf("[DeferredService] 已写入 %d 个账号的遥测隐私保护计数", len(updates))
		return
	}

	s.telemetryPrivacyMu.Lock()
	if s.telemetryPrivacyUpdates == nil {
		s.telemetryPrivacyUpdates = make(map[int64]int64)
	}
	for id, delta := range failed {
		s.telemetryPrivacyUpdates[id] += delta
	}
	s.telemetryPrivacyMu.Unlock()
}
