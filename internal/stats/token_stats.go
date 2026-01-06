package stats

import (
	"sync"
	"time"
)

// TokenUsageRecord 单次请求的 token 使用记录
type TokenUsageRecord struct {
	Timestamp    time.Time `json:"timestamp"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	Model        string    `json:"model"`
}

// HourlyStats 每小时的统计数据
type HourlyStats struct {
	Hour         string `json:"hour"`          // 格式: "2024-12-28 10:00"
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	RequestCount int    `json:"request_count"`
}

// TokenStatsCollector token 使用统计收集器
type TokenStatsCollector struct {
	mutex       sync.RWMutex
	hourlyStats map[string]*HourlyStats // key: "2024-12-28 10:00"
	maxHours    int                     // 保留最近多少小时的数据
}

var (
	globalCollector *TokenStatsCollector
	once            sync.Once
)

// GetCollector 获取全局统计收集器
func GetCollector() *TokenStatsCollector {
	once.Do(func() {
		globalCollector = &TokenStatsCollector{
			hourlyStats: make(map[string]*HourlyStats),
			maxHours:    24, // 保留最近 24 小时
		}
	})
	return globalCollector
}

// Record 记录一次 token 使用
func (c *TokenStatsCollector) Record(inputTokens, outputTokens int, model string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	hourKey := time.Now().Format("2006-01-02 15:00")

	stats, exists := c.hourlyStats[hourKey]
	if !exists {
		stats = &HourlyStats{Hour: hourKey}
		c.hourlyStats[hourKey] = stats
		c.cleanup() // 清理旧数据
	}

	stats.InputTokens += int64(inputTokens)
	stats.OutputTokens += int64(outputTokens)
	stats.RequestCount++
}

// GetHourlyStats 获取最近 N 小时的统计数据
func (c *TokenStatsCollector) GetHourlyStats(hours int) []HourlyStats {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if hours > c.maxHours {
		hours = c.maxHours
	}

	result := make([]HourlyStats, 0, hours)
	now := time.Now()

	for i := hours - 1; i >= 0; i-- {
		t := now.Add(-time.Duration(i) * time.Hour)
		hourKey := t.Format("2006-01-02 15:00")

		if stats, exists := c.hourlyStats[hourKey]; exists {
			result = append(result, *stats)
		} else {
			// 没有数据的小时填充 0
			result = append(result, HourlyStats{
				Hour:         hourKey,
				InputTokens:  0,
				OutputTokens: 0,
				RequestCount: 0,
			})
		}
	}

	return result
}

// GetTodayTotal 获取今日总计
func (c *TokenStatsCollector) GetTodayTotal() (inputTokens, outputTokens int64, requestCount int) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	today := time.Now().Format("2006-01-02")

	for hourKey, stats := range c.hourlyStats {
		if len(hourKey) >= 10 && hourKey[:10] == today {
			inputTokens += stats.InputTokens
			outputTokens += stats.OutputTokens
			requestCount += stats.RequestCount
		}
	}

	return
}

// cleanup 清理超过 maxHours 的旧数据
func (c *TokenStatsCollector) cleanup() {
	cutoff := time.Now().Add(-time.Duration(c.maxHours) * time.Hour)
	cutoffKey := cutoff.Format("2006-01-02 15:00")

	for hourKey := range c.hourlyStats {
		if hourKey < cutoffKey {
			delete(c.hourlyStats, hourKey)
		}
	}
}
