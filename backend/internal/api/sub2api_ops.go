package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/worryzyy/upstream-hub/internal/storage"
	"github.com/worryzyy/upstream-hub/internal/sub2apiops"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
)

const (
	maxOpsConfigBody          = 128 << 10
	maxWidgets                = 24
	maxSnapshotGroups         = 12
	maxAccountFilterGroups    = 12
	maxAccountUsageDetailIDs  = 50
	maxOpsAccountCandidates   = 10_000
	maxRecentAccountLimit     = 20
	maxRecentAccountPages     = 5
	opsAccountCandidateSize   = 100
	recentAccountPageSize     = 100
	accountUsageDetailTimeout = 60 * time.Second
)

var dashboardWidgetIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
var sub2APIOpsUsageDetailSlots = make(chan struct{}, 8)

var errSub2APIOpsNotConfigured = errors.New("请先配置 Sub2API 连接")

type sub2APIOpsInputError struct{ err error }

func (e *sub2APIOpsInputError) Error() string { return e.err.Error() }
func (e *sub2APIOpsInputError) Unwrap() error { return e.err }

type sub2APIOpsConfigInput struct {
	Name     string `json:"name"`
	SiteURL  string `json:"site_url"`
	AdminKey string `json:"admin_key"`
}

type sub2APIOpsConfigView struct {
	Configured         bool            `json:"configured"`
	Name               string          `json:"name"`
	SiteURL            string          `json:"site_url"`
	AdminKeyConfigured bool            `json:"admin_key_configured"`
	Layout             json.RawMessage `json:"layout,omitempty"`
	UpdatedAt          *time.Time      `json:"updated_at,omitempty"`
}

type dashboardLayoutInput struct {
	Version int                    `json:"version"`
	Widgets []dashboardWidgetInput `json:"widgets"`
}

type dashboardWidgetInput struct {
	ID     string          `json:"id"`
	Type   string          `json:"type"`
	Width  int             `json:"width"`
	Height string          `json:"height"`
	Config json.RawMessage `json:"config"`
}

type sub2APIOpsRateSummary struct {
	Current float64 `json:"current"`
	Peak    float64 `json:"peak"`
	Avg     float64 `json:"avg"`
}

type sub2APIOpsPercentiles struct {
	P50MS *float64 `json:"p50_ms"`
	P90MS *float64 `json:"p90_ms"`
	P95MS *float64 `json:"p95_ms"`
	P99MS *float64 `json:"p99_ms"`
	AvgMS *float64 `json:"avg_ms"`
	MaxMS *float64 `json:"max_ms"`
}

type sub2APISafeOpsOverview struct {
	StartTime            string                `json:"start_time"`
	EndTime              string                `json:"end_time"`
	Platform             string                `json:"platform"`
	GroupID              *int64                `json:"group_id"`
	HealthScore          int                   `json:"health_score"`
	SuccessCount         int64                 `json:"success_count"`
	ErrorCountTotal      int64                 `json:"error_count_total"`
	BusinessLimitedCount int64                 `json:"business_limited_count"`
	ErrorCountSLA        int64                 `json:"error_count_sla"`
	RequestCountTotal    int64                 `json:"request_count_total"`
	RequestCountSLA      int64                 `json:"request_count_sla"`
	TokenConsumed        int64                 `json:"token_consumed"`
	SLA                  float64               `json:"sla"`
	ErrorRate            float64               `json:"error_rate"`
	UpstreamErrorRate    float64               `json:"upstream_error_rate"`
	UpstreamErrorCount   int64                 `json:"upstream_error_count_excl_429_529"`
	Upstream429Count     int64                 `json:"upstream_429_count"`
	Upstream529Count     int64                 `json:"upstream_529_count"`
	QPS                  sub2APIOpsRateSummary `json:"qps"`
	TPS                  sub2APIOpsRateSummary `json:"tps"`
	Duration             sub2APIOpsPercentiles `json:"duration"`
	TTFT                 sub2APIOpsPercentiles `json:"ttft"`
}

type sub2APISafeThroughputPoint struct {
	BucketStart   string  `json:"bucket_start"`
	RequestCount  int64   `json:"request_count"`
	TokenConsumed int64   `json:"token_consumed"`
	SwitchCount   int64   `json:"switch_count"`
	QPS           float64 `json:"qps"`
	TPS           float64 `json:"tps"`
}

type sub2APISafeThroughputTrend struct {
	Bucket string                       `json:"bucket"`
	Points []sub2APISafeThroughputPoint `json:"points"`
}

type sub2APISafeErrorPoint struct {
	BucketStart          string `json:"bucket_start"`
	ErrorCountTotal      int64  `json:"error_count_total"`
	BusinessLimitedCount int64  `json:"business_limited_count"`
	ErrorCountSLA        int64  `json:"error_count_sla"`
}

type sub2APISafeErrorTrend struct {
	Bucket string                  `json:"bucket"`
	Points []sub2APISafeErrorPoint `json:"points"`
}

type sub2APISafeOpsSnapshot struct {
	GeneratedAt     string                      `json:"generated_at"`
	Overview        *sub2APISafeOpsOverview     `json:"overview"`
	ThroughputTrend *sub2APISafeThroughputTrend `json:"throughput_trend"`
	ErrorTrend      *sub2APISafeErrorTrend      `json:"error_trend"`
	Aggregation     *sub2APISnapshotAggregation `json:"aggregation,omitempty"`
}

type sub2APISnapshotAggregation struct {
	GroupIDs       []int64 `json:"group_ids"`
	PercentileMode string  `json:"percentile_mode"`
}

type sub2APISafeAccountStats struct {
	Requests     int64   `json:"requests"`
	Tokens       int64   `json:"tokens"`
	Cost         float64 `json:"cost"`
	StandardCost float64 `json:"standard_cost"`
	UserCost     float64 `json:"user_cost"`
}

type sub2APISafeTodayStatsResponse struct {
	Stats map[string]sub2APISafeAccountStats `json:"stats"`
}

type sub2APIUpstreamUsageStats struct {
	TotalRequests    int64    `json:"total_requests"`
	TotalTokens      int64    `json:"total_tokens"`
	TotalCost        float64  `json:"total_cost"`
	TotalActualCost  float64  `json:"total_actual_cost"`
	TotalAccountCost *float64 `json:"total_account_cost"`
}

type sub2APIUpstreamUsageProgress struct {
	WindowStats *sub2APISafeAccountStats `json:"window_stats"`
}

type sub2APIUpstreamAccountUsage struct {
	FiveHour              *sub2APIUpstreamUsageProgress `json:"five_hour"`
	SevenDay              *sub2APIUpstreamUsageProgress `json:"seven_day"`
	SevenDaySonnet        *sub2APIUpstreamUsageProgress `json:"seven_day_sonnet"`
	SevenDayFable         *sub2APIUpstreamUsageProgress `json:"seven_day_fable"`
	GeminiSharedDaily     *sub2APIUpstreamUsageProgress `json:"gemini_shared_daily"`
	GeminiProDaily        *sub2APIUpstreamUsageProgress `json:"gemini_pro_daily"`
	GeminiFlashDaily      *sub2APIUpstreamUsageProgress `json:"gemini_flash_daily"`
	GeminiSharedMinute    *sub2APIUpstreamUsageProgress `json:"gemini_shared_minute"`
	GeminiProMinute       *sub2APIUpstreamUsageProgress `json:"gemini_pro_minute"`
	GeminiFlashMinute     *sub2APIUpstreamUsageProgress `json:"gemini_flash_minute"`
	GrokLocalUsage24H     *sub2APISafeAccountStats      `json:"grok_local_usage_24h"`
	GrokLocalUsage7D      *sub2APISafeAccountStats      `json:"grok_local_usage_7d"`
	GrokLocalUsageMonthly *sub2APISafeAccountStats      `json:"grok_local_usage_monthly"`
}

type sub2APISafeAccountUsageDetail struct {
	Total       *sub2APISafeAccountStats           `json:"total,omitempty"`
	WindowStats map[string]sub2APISafeAccountStats `json:"window_stats,omitempty"`
}

type sub2APISafeAccountUsageDetailsResponse struct {
	Details         map[string]sub2APISafeAccountUsageDetail `json:"details"`
	FailedTotalIDs  []int64                                  `json:"failed_total_ids"`
	FailedWindowIDs []int64                                  `json:"failed_window_ids"`
}

type sub2APISafeGroup struct {
	ID                      int64  `json:"id"`
	Name                    string `json:"name"`
	Description             string `json:"description"`
	Platform                string `json:"platform"`
	Status                  string `json:"status"`
	SortOrder               int    `json:"sort_order"`
	AccountCount            int64  `json:"account_count,omitempty"`
	ActiveAccountCount      int64  `json:"active_account_count,omitempty"`
	RateLimitedAccountCount int64  `json:"rate_limited_account_count,omitempty"`
}

type sub2APISchedulerScore struct {
	BaseScore             float64 `json:"base_score"`
	StickyScore           float64 `json:"sticky_score"`
	StickyScoreInfinity   bool    `json:"sticky_score_infinity"`
	StickyWeightedEnabled bool    `json:"sticky_weighted_enabled"`
}

type sub2APISchedulerGroupScore struct {
	GroupID       *int64 `json:"group_id"`
	GroupName     string `json:"group_name,omitempty"`
	GroupPriority *int   `json:"group_priority,omitempty"`
	sub2APISchedulerScore
}

type sub2APISafeAccount struct {
	ID                     int64                           `json:"id"`
	Name                   string                          `json:"name"`
	Notes                  *string                         `json:"notes"`
	Platform               string                          `json:"platform"`
	Type                   string                          `json:"type"`
	Status                 string                          `json:"status"`
	Schedulable            bool                            `json:"schedulable"`
	Concurrency            int                             `json:"concurrency"`
	LoadFactor             *int                            `json:"load_factor,omitempty"`
	CurrentConcurrency     int                             `json:"current_concurrency"`
	Priority               int                             `json:"priority"`
	RateMultiplier         float64                         `json:"rate_multiplier"`
	LastUsedAt             *time.Time                      `json:"last_used_at"`
	ExpiresAt              *int64                          `json:"expires_at"`
	AutoPauseOnExpired     bool                            `json:"auto_pause_on_expired"`
	CreatedAt              time.Time                       `json:"created_at"`
	UpdatedAt              time.Time                       `json:"updated_at"`
	RateLimitedAt          *time.Time                      `json:"rate_limited_at"`
	RateLimitResetAt       *time.Time                      `json:"rate_limit_reset_at"`
	OverloadUntil          *time.Time                      `json:"overload_until"`
	TempUnschedulableUntil *time.Time                      `json:"temp_unschedulable_until"`
	SessionWindowStart     *time.Time                      `json:"session_window_start"`
	SessionWindowEnd       *time.Time                      `json:"session_window_end"`
	SessionWindowStatus    string                          `json:"session_window_status"`
	GroupIDs               []int64                         `json:"group_ids,omitempty"`
	Groups                 []sub2APISafeGroup              `json:"groups,omitempty"`
	SchedulerScore         *sub2APISchedulerScore          `json:"scheduler_score,omitempty"`
	SchedulerScores        []sub2APISchedulerGroupScore    `json:"scheduler_scores,omitempty"`
	CurrentWindowCost      *float64                        `json:"current_window_cost,omitempty"`
	ActiveSessions         *int                            `json:"active_sessions,omitempty"`
	CurrentRPM             *int                            `json:"current_rpm,omitempty"`
	QuotaLimit             *float64                        `json:"quota_limit,omitempty"`
	QuotaUsed              *float64                        `json:"quota_used,omitempty"`
	QuotaDailyLimit        *float64                        `json:"quota_daily_limit,omitempty"`
	QuotaDailyUsed         *float64                        `json:"quota_daily_used,omitempty"`
	QuotaWeeklyLimit       *float64                        `json:"quota_weekly_limit,omitempty"`
	QuotaWeeklyUsed        *float64                        `json:"quota_weekly_used,omitempty"`
	UsageWindows           *sub2APISafeAccountUsageWindows `json:"usage_windows,omitempty"`
}

// sub2APISafeAccountExtra is the small, non-sensitive subset needed to render
// provider usage windows. The upstream account response contains credentials
// and many unrelated extra fields, so it is decoded into this allowlist only.
type sub2APISafeAccountExtra struct {
	SessionWindowUtilization        *float64                `json:"session_window_utilization"`
	PassiveUsage7DUtilization       *float64                `json:"passive_usage_7d_utilization"`
	PassiveUsage7DReset             *float64                `json:"passive_usage_7d_reset"`
	PassiveUsage7DOIUtilization     *float64                `json:"passive_usage_7d_oi_utilization"`
	PassiveUsage7DOIReset           *float64                `json:"passive_usage_7d_oi_reset"`
	PassiveUsageSampledAt           string                  `json:"passive_usage_sampled_at"`
	CodexPrimaryUsedPercent         *float64                `json:"codex_primary_used_percent"`
	CodexPrimaryResetAfterSeconds   *float64                `json:"codex_primary_reset_after_seconds"`
	CodexPrimaryWindowMinutes       *float64                `json:"codex_primary_window_minutes"`
	CodexSecondaryUsedPercent       *float64                `json:"codex_secondary_used_percent"`
	CodexSecondaryResetAfterSeconds *float64                `json:"codex_secondary_reset_after_seconds"`
	CodexSecondaryWindowMinutes     *float64                `json:"codex_secondary_window_minutes"`
	Codex5HUsedPercent              *float64                `json:"codex_5h_used_percent"`
	Codex5HResetAt                  string                  `json:"codex_5h_reset_at"`
	Codex5HResetAfterSeconds        *float64                `json:"codex_5h_reset_after_seconds"`
	Codex5HWindowMinutes            *float64                `json:"codex_5h_window_minutes"`
	Codex7DUsedPercent              *float64                `json:"codex_7d_used_percent"`
	Codex7DResetAt                  string                  `json:"codex_7d_reset_at"`
	Codex7DResetAfterSeconds        *float64                `json:"codex_7d_reset_after_seconds"`
	Codex7DWindowMinutes            *float64                `json:"codex_7d_window_minutes"`
	CodexUsageUpdatedAt             string                  `json:"codex_usage_updated_at"`
	GrokBilling                     *sub2APISafeGrokBilling `json:"grok_billing_snapshot"`
}

type sub2APISafeGrokBilling struct {
	PeriodType         string   `json:"period_type"`
	UsagePercent       *float64 `json:"usage_percent"`
	PeriodStart        string   `json:"period_start"`
	PeriodEnd          string   `json:"period_end"`
	UsedPercent        *float64 `json:"used_percent"`
	BillingPeriodStart string   `json:"billing_period_start"`
	BillingPeriodEnd   string   `json:"billing_period_end"`
	FetchedAt          string   `json:"fetched_at"`
	UpdatedAt          string   `json:"updated_at"`
	WeeklyUpdatedAt    string   `json:"weekly_updated_at"`
	MonthlyUpdatedAt   string   `json:"monthly_updated_at"`
}

type sub2APISafeUsageWindow struct {
	Label         string     `json:"label"`
	Utilization   float64    `json:"utilization"`
	WindowMinutes *int       `json:"window_minutes,omitempty"`
	ResetsAt      *time.Time `json:"resets_at,omitempty"`
}

type sub2APISafeAccountUsageWindows struct {
	Items     []sub2APISafeUsageWindow `json:"items"`
	UpdatedAt *time.Time               `json:"updated_at,omitempty"`
}

// Decode account extra through an allowlist and immediately normalize it. The
// raw upstream extra map is intentionally never represented in the safe view.
func (account *sub2APISafeAccount) UnmarshalJSON(data []byte) error {
	type accountAlias sub2APISafeAccount
	var raw struct {
		*accountAlias
		Extra *sub2APISafeAccountExtra `json:"extra"`
	}
	raw.accountAlias = (*accountAlias)(account)
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	account.UsageWindows = buildSub2APISafeAccountUsageWindows(
		account.Platform,
		raw.Extra,
		account.SessionWindowEnd,
		account.SessionWindowStatus,
		time.Now(),
	)
	return nil
}

type sub2APISafeAccountPage struct {
	Items    []sub2APISafeAccount `json:"items"`
	Total    int64                `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
	Pages    int                  `json:"pages"`
}

type sub2APISafeRequestDetail struct {
	CreatedAt time.Time `json:"created_at"`
	Platform  string    `json:"platform"`
	AccountID *int64    `json:"account_id"`
	GroupID   *int64    `json:"group_id"`
}

type sub2APISafeRequestDetailPage struct {
	Items    []sub2APISafeRequestDetail `json:"items"`
	Total    int64                      `json:"total"`
	Page     int                        `json:"page"`
	PageSize int                        `json:"page_size"`
	Pages    int                        `json:"pages"`
}

type sub2APIOpsRecentAccount struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Platform   string    `json:"platform"`
	GroupIDs   []int64   `json:"group_ids,omitempty"`
	LastUsedAt time.Time `json:"last_used_at"`
}

type sub2APIOpsRecentAccountsResponse struct {
	Accounts    []sub2APIOpsRecentAccount `json:"accounts"`
	Limit       int                       `json:"limit"`
	StartTime   string                    `json:"start_time"`
	EndTime     string                    `json:"end_time"`
	GeneratedAt string                    `json:"generated_at"`
	Truncated   bool                      `json:"truncated"`
}

func registerSub2APIOps(g *gin.RouterGroup, d *Deps) {
	group := g.Group("/sub2api-ops")
	group.Use(func(c *gin.Context) {
		if d.Auth == nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "Sub2API 运维面板要求启用 upstream-hub 后台登录",
			})
			return
		}
		c.Next()
	})
	group.GET("/config", func(c *gin.Context) { getSub2APIOpsConfig(c, d) })
	group.PUT("/config", func(c *gin.Context) { updateSub2APIOpsConfig(c, d) })
	group.POST("/config/test", func(c *gin.Context) { testSub2APIOpsConfig(c, d) })
	group.PUT("/layout", func(c *gin.Context) { updateSub2APIOpsLayout(c, d) })
	group.GET("/groups", func(c *gin.Context) { getSub2APIOpsGroups(c, d) })
	group.GET("/snapshot", func(c *gin.Context) { getSub2APIOpsSnapshot(c, d) })
	group.GET("/recent-accounts", func(c *gin.Context) { getSub2APIOpsRecentAccounts(c, d) })
	group.GET("/accounts", func(c *gin.Context) { getSub2APIOpsAccounts(c, d) })
	group.POST("/accounts/today-stats", func(c *gin.Context) { getSub2APIOpsTodayStats(c, d) })
	group.GET("/accounts/usage-details", func(c *gin.Context) { getSub2APIOpsAccountUsageDetails(c, d) })
}

func getSub2APIOpsConfig(c *gin.Context, d *Deps) {
	config, err := d.Sub2APIOps.Get()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusOK, gin.H{"data": sub2APIOpsConfigView{}})
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": sub2APIOpsConfigResponse(config)})
}

func updateSub2APIOpsConfig(c *gin.Context, d *Deps) {
	var input sub2APIOpsConfigInput
	if err := bindLimitedJSON(c, &input); err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 128 {
		fail(c, http.StatusBadRequest, errors.New("实例名称不能为空且不能超过 128 个字符"))
		return
	}
	siteURL, err := sub2apiops.NormalizeSiteURL(input.SiteURL)
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}

	config, err := d.Sub2APIOps.Get()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		config = &storage.Sub2APIOpsConfig{DashboardLayout: ""}
	} else if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	adminKey := strings.TrimSpace(input.AdminKey)
	if err := validateAdminKeyBinding(config.SiteURL, siteURL, adminKey); err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	if adminKey != "" {
		if _, err := sub2apiops.NewClient(siteURL, adminKey); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		config.AdminKeyCipher, err = d.Cipher.Encrypt(adminKey)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
	} else if config.AdminKeyCipher == "" {
		fail(c, http.StatusBadRequest, errors.New("首次配置必须填写 Admin Key"))
		return
	}
	config.Name = input.Name
	config.SiteURL = siteURL
	if adminKey != "" {
		err = d.Sub2APIOps.UpsertConnection(config)
	} else {
		err = d.Sub2APIOps.UpdateConnectionMetadata(config)
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		fail(c, http.StatusConflict, errors.New("Sub2API 连接已被其他请求修改，请刷新后重试"))
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": sub2APIOpsConfigResponse(config)})
}

func testSub2APIOpsConfig(c *gin.Context, d *Deps) {
	var input sub2APIOpsConfigInput
	if err := bindOptionalLimitedJSON(c, &input); err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	client, err := sub2APIOpsClient(d, input.SiteURL, input.AdminKey)
	if err != nil {
		writeSub2APIOpsClientError(c, d, err)
		return
	}
	if err := client.Test(c.Request.Context()); err != nil {
		writeSub2APIOpsError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"ok": true}})
}

func updateSub2APIOpsLayout(c *gin.Context, d *Deps) {
	var input dashboardLayoutInput
	if err := bindLimitedJSON(c, &input); err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	if err := validateDashboardLayout(input); err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	payload, err := json.Marshal(input)
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	if err := d.Sub2APIOps.UpdateLayout(string(payload)); errors.Is(err, gorm.ErrRecordNotFound) {
		fail(c, http.StatusConflict, errors.New("请先配置 Sub2API 连接"))
		return
	} else if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": input})
}

func getSub2APIOpsSnapshot(c *gin.Context, d *Deps) {
	groupIDs, err := selectedSub2APIGroupIDs(c)
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	query := url.Values{}
	timeRange := c.DefaultQuery("time_range", "1h")
	if !stringAllowed(timeRange, "5m", "30m", "1h", "6h", "24h", "7d", "30d") {
		fail(c, http.StatusBadRequest, errors.New("不支持的时间范围"))
		return
	}
	query.Set("time_range", timeRange)
	if platform := c.Query("platform"); platform != "" {
		if !validAccountPlatform(platform) {
			fail(c, http.StatusBadRequest, errors.New("不支持的平台"))
			return
		}
		query.Set("platform", platform)
	}
	query.Set("mode", "auto")

	client, err := sub2APIOpsClient(d, "", "")
	if err != nil {
		writeSub2APIOpsClientError(c, d, err)
		return
	}
	snapshots, status, err := fetchSub2APIOpsSnapshots(c.Request.Context(), client, query, groupIDs)
	if err != nil {
		writeSub2APIOpsError(c, err)
		return
	}
	if len(snapshots) == 1 {
		c.JSON(status, gin.H{"data": snapshots[0]})
		return
	}
	c.JSON(status, gin.H{"data": aggregateSub2APIOpsSnapshots(groupIDs, snapshots)})
}

func getSub2APIOpsRecentAccounts(c *gin.Context, d *Deps) {
	groupIDs, err := selectedSub2APIGroupIDs(c)
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	timeRange := c.DefaultQuery("time_range", "1h")
	duration, ok := sub2APIOpsTimeRangeDuration(timeRange)
	if !ok {
		fail(c, http.StatusBadRequest, errors.New("不支持的时间范围"))
		return
	}
	limit, err := positiveIntQuery(c, "limit", 5, maxRecentAccountLimit)
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	platform := strings.TrimSpace(c.Query("platform"))
	if platform != "" && !validAccountPlatform(platform) {
		fail(c, http.StatusBadRequest, errors.New("不支持的平台"))
		return
	}

	client, err := sub2APIOpsClient(d, "", "")
	if err != nil {
		writeSub2APIOpsClientError(c, d, err)
		return
	}
	endTime := time.Now().UTC()
	startTime := endTime.Add(-duration)
	accounts, truncated, status, err := fetchSub2APIOpsRecentAccounts(
		c.Request.Context(),
		client,
		platform,
		groupIDs,
		startTime,
		endTime,
		limit,
	)
	if err != nil {
		writeSub2APIOpsError(c, err)
		return
	}
	result := sub2APIOpsRecentAccountsResponse{
		Accounts:    accounts,
		Limit:       limit,
		StartTime:   startTime.Format(time.RFC3339Nano),
		EndTime:     endTime.Format(time.RFC3339Nano),
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Truncated:   truncated,
	}
	c.JSON(status, gin.H{"data": result})
}

func fetchSub2APIOpsSnapshots(
	ctx context.Context,
	client *sub2apiops.Client,
	baseQuery url.Values,
	groupIDs []int64,
) ([]sub2APISafeOpsSnapshot, int, error) {
	if len(groupIDs) == 0 {
		snapshot, status, err := fetchSub2APIOpsSnapshot(ctx, client, baseQuery)
		return []sub2APISafeOpsSnapshot{snapshot}, status, err
	}

	snapshots := make([]sub2APISafeOpsSnapshot, len(groupIDs))
	statuses := make([]int, len(groupIDs))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(4)
	for index, groupID := range groupIDs {
		index, groupID := index, groupID
		group.Go(func() error {
			query := cloneURLValues(baseQuery)
			query.Set("group_id", strconv.FormatInt(groupID, 10))
			snapshot, status, err := fetchSub2APIOpsSnapshot(groupCtx, client, query)
			if err != nil {
				return fmt.Errorf("加载分组 %d 指标: %w", groupID, err)
			}
			snapshots[index] = snapshot
			statuses[index] = status
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, 0, err
	}
	return snapshots, statuses[0], nil
}

func fetchSub2APIOpsSnapshot(
	ctx context.Context,
	client *sub2apiops.Client,
	query url.Values,
) (sub2APISafeOpsSnapshot, int, error) {
	var snapshot sub2APISafeOpsSnapshot
	payload, status, err := client.Do(ctx, http.MethodGet, "/ops/dashboard/snapshot-v2", query, nil)
	if err != nil {
		return snapshot, status, err
	}
	if err := decodeSub2APIData(payload, &snapshot); err != nil {
		return snapshot, status, err
	}
	return snapshot, status, nil
}

func aggregateSub2APIOpsSnapshots(groupIDs []int64, snapshots []sub2APISafeOpsSnapshot) sub2APISafeOpsSnapshot {
	result := sub2APISafeOpsSnapshot{
		Aggregation: &sub2APISnapshotAggregation{
			GroupIDs:       append([]int64(nil), groupIDs...),
			PercentileMode: "worst_group",
		},
	}
	overview := &sub2APISafeOpsOverview{}
	hasOverview := false
	healthSet := false
	platformSet := false
	var upstreamRateWeighted float64
	var upstreamRateWeight int64

	for _, snapshot := range snapshots {
		result.GeneratedAt = laterRFC3339(result.GeneratedAt, snapshot.GeneratedAt)
		if snapshot.Overview == nil {
			continue
		}
		item := snapshot.Overview
		hasOverview = true
		if overview.StartTime == "" || item.StartTime < overview.StartTime {
			overview.StartTime = item.StartTime
		}
		if item.EndTime > overview.EndTime {
			overview.EndTime = item.EndTime
		}
		if !platformSet {
			overview.Platform = item.Platform
			platformSet = true
		} else if overview.Platform != item.Platform {
			overview.Platform = ""
		}
		if !healthSet || item.HealthScore < overview.HealthScore {
			overview.HealthScore = item.HealthScore
			healthSet = true
		}

		overview.SuccessCount += item.SuccessCount
		overview.ErrorCountTotal += item.ErrorCountTotal
		overview.BusinessLimitedCount += item.BusinessLimitedCount
		overview.ErrorCountSLA += item.ErrorCountSLA
		overview.RequestCountTotal += item.RequestCountTotal
		overview.RequestCountSLA += item.RequestCountSLA
		overview.TokenConsumed += item.TokenConsumed
		overview.UpstreamErrorCount += item.UpstreamErrorCount
		overview.Upstream429Count += item.Upstream429Count
		overview.Upstream529Count += item.Upstream529Count
		overview.QPS.Current += item.QPS.Current
		overview.QPS.Peak += item.QPS.Peak
		overview.QPS.Avg += item.QPS.Avg
		overview.TPS.Current += item.TPS.Current
		overview.TPS.Peak += item.TPS.Peak
		overview.TPS.Avg += item.TPS.Avg
		overview.Duration = maxSub2APIOpsPercentiles(overview.Duration, item.Duration)
		overview.TTFT = maxSub2APIOpsPercentiles(overview.TTFT, item.TTFT)
		if item.RequestCountSLA > 0 {
			upstreamRateWeighted += item.UpstreamErrorRate * float64(item.RequestCountSLA)
			upstreamRateWeight += item.RequestCountSLA
		}
	}

	if hasOverview {
		overview.SLA = roundedRatio(overview.SuccessCount, overview.RequestCountSLA)
		overview.ErrorRate = roundedRatio(overview.ErrorCountSLA, overview.RequestCountSLA)
		overview.UpstreamErrorRate = roundedRatio(overview.UpstreamErrorCount, overview.RequestCountSLA)
		if overview.UpstreamErrorCount == 0 && upstreamRateWeighted > 0 && upstreamRateWeight > 0 {
			overview.UpstreamErrorRate = math.Round((upstreamRateWeighted/float64(upstreamRateWeight))*10_000) / 10_000
		}
		result.Overview = overview
	}
	result.ThroughputTrend = aggregateSub2APIThroughputTrends(snapshots)
	result.ErrorTrend = aggregateSub2APIErrorTrends(snapshots)
	return result
}

func aggregateSub2APIThroughputTrends(snapshots []sub2APISafeOpsSnapshot) *sub2APISafeThroughputTrend {
	points := make(map[string]*sub2APISafeThroughputPoint)
	bucket := ""
	for _, snapshot := range snapshots {
		if snapshot.ThroughputTrend == nil {
			continue
		}
		if bucket == "" {
			bucket = snapshot.ThroughputTrend.Bucket
		}
		for _, item := range snapshot.ThroughputTrend.Points {
			point := points[item.BucketStart]
			if point == nil {
				point = &sub2APISafeThroughputPoint{BucketStart: item.BucketStart}
				points[item.BucketStart] = point
			}
			point.RequestCount += item.RequestCount
			point.TokenConsumed += item.TokenConsumed
			point.SwitchCount += item.SwitchCount
			point.QPS += item.QPS
			point.TPS += item.TPS
		}
	}
	if len(points) == 0 && bucket == "" {
		return nil
	}
	keys := make([]string, 0, len(points))
	for key := range points {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	items := make([]sub2APISafeThroughputPoint, 0, len(keys))
	for _, key := range keys {
		items = append(items, *points[key])
	}
	return &sub2APISafeThroughputTrend{Bucket: bucket, Points: items}
}

func aggregateSub2APIErrorTrends(snapshots []sub2APISafeOpsSnapshot) *sub2APISafeErrorTrend {
	points := make(map[string]*sub2APISafeErrorPoint)
	bucket := ""
	for _, snapshot := range snapshots {
		if snapshot.ErrorTrend == nil {
			continue
		}
		if bucket == "" {
			bucket = snapshot.ErrorTrend.Bucket
		}
		for _, item := range snapshot.ErrorTrend.Points {
			point := points[item.BucketStart]
			if point == nil {
				point = &sub2APISafeErrorPoint{BucketStart: item.BucketStart}
				points[item.BucketStart] = point
			}
			point.ErrorCountTotal += item.ErrorCountTotal
			point.BusinessLimitedCount += item.BusinessLimitedCount
			point.ErrorCountSLA += item.ErrorCountSLA
		}
	}
	if len(points) == 0 && bucket == "" {
		return nil
	}
	keys := make([]string, 0, len(points))
	for key := range points {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	items := make([]sub2APISafeErrorPoint, 0, len(keys))
	for _, key := range keys {
		items = append(items, *points[key])
	}
	return &sub2APISafeErrorTrend{Bucket: bucket, Points: items}
}

func fetchSub2APIOpsRecentAccounts(
	ctx context.Context,
	client *sub2apiops.Client,
	platform string,
	groupIDs []int64,
	startTime time.Time,
	endTime time.Time,
	limit int,
) ([]sub2APIOpsRecentAccount, bool, int, error) {
	if len(groupIDs) == 0 {
		accounts, truncated, status, err := fetchSub2APIOpsRecentAccountCandidates(
			ctx, client, platform, nil, startTime, endTime, limit,
		)
		if err != nil {
			return nil, false, status, err
		}
		hydrateSub2APIOpsRecentAccountNames(ctx, client, accounts)
		return accounts, truncated, status, nil
	}

	results := make([][]sub2APIOpsRecentAccount, len(groupIDs))
	truncatedResults := make([]bool, len(groupIDs))
	statuses := make([]int, len(groupIDs))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(4)
	for index, groupID := range groupIDs {
		index, groupID := index, groupID
		group.Go(func() error {
			accounts, truncated, status, err := fetchSub2APIOpsRecentAccountCandidates(
				groupCtx, client, platform, &groupID, startTime, endTime, limit,
			)
			if err != nil {
				return fmt.Errorf("加载分组 %d 调度记录: %w", groupID, err)
			}
			results[index] = accounts
			truncatedResults[index] = truncated
			statuses[index] = status
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, false, 0, err
	}

	merged := make(map[int64]*sub2APIOpsRecentAccount)
	truncated := false
	for index, accounts := range results {
		truncated = truncated || truncatedResults[index]
		for _, account := range accounts {
			mergeSub2APIOpsRecentAccount(merged, account)
		}
	}
	accounts := sortedSub2APIOpsRecentAccounts(merged, limit)
	hydrateSub2APIOpsRecentAccountNames(ctx, client, accounts)
	return accounts, truncated, statuses[0], nil
}

func fetchSub2APIOpsRecentAccountCandidates(
	ctx context.Context,
	client *sub2apiops.Client,
	platform string,
	groupID *int64,
	startTime time.Time,
	endTime time.Time,
	limit int,
) ([]sub2APIOpsRecentAccount, bool, int, error) {
	query := url.Values{
		"start_time": {startTime.Format(time.RFC3339Nano)},
		"end_time":   {endTime.Format(time.RFC3339Nano)},
		"page_size":  {strconv.Itoa(recentAccountPageSize)},
		"sort":       {"created_at_desc"},
	}
	if platform != "" {
		query.Set("platform", platform)
	}
	if groupID != nil {
		query.Set("group_id", strconv.FormatInt(*groupID, 10))
	}

	accounts := make(map[int64]*sub2APIOpsRecentAccount)
	status := http.StatusOK
	for pageNumber := 1; pageNumber <= maxRecentAccountPages; pageNumber++ {
		query.Set("page", strconv.Itoa(pageNumber))
		payload, currentStatus, err := client.Do(ctx, http.MethodGet, "/ops/requests", query, nil)
		if err != nil {
			return nil, false, currentStatus, err
		}
		status = currentStatus
		var page sub2APISafeRequestDetailPage
		if err := decodeSub2APIData(payload, &page); err != nil {
			return nil, false, status, err
		}
		for _, item := range page.Items {
			if item.AccountID == nil || *item.AccountID <= 0 {
				continue
			}
			account := sub2APIOpsRecentAccount{
				ID:         *item.AccountID,
				Name:       fmt.Sprintf("账号 #%d", *item.AccountID),
				Platform:   item.Platform,
				LastUsedAt: item.CreatedAt,
			}
			if item.GroupID != nil && *item.GroupID > 0 {
				account.GroupIDs = []int64{*item.GroupID}
			}
			mergeSub2APIOpsRecentAccount(accounts, account)
		}

		if len(accounts) >= limit {
			return sortedSub2APIOpsRecentAccounts(accounts, limit), false, status, nil
		}
		if pageNumber >= page.Pages || len(page.Items) < recentAccountPageSize {
			return sortedSub2APIOpsRecentAccounts(accounts, limit), false, status, nil
		}
		if pageNumber == maxRecentAccountPages {
			return sortedSub2APIOpsRecentAccounts(accounts, limit), true, status, nil
		}
	}
	return sortedSub2APIOpsRecentAccounts(accounts, limit), false, status, nil
}

func mergeSub2APIOpsRecentAccount(accounts map[int64]*sub2APIOpsRecentAccount, item sub2APIOpsRecentAccount) {
	if item.ID <= 0 {
		return
	}
	existing := accounts[item.ID]
	if existing == nil {
		copy := item
		copy.GroupIDs = append([]int64(nil), item.GroupIDs...)
		accounts[item.ID] = &copy
		return
	}
	if item.LastUsedAt.After(existing.LastUsedAt) {
		existing.LastUsedAt = item.LastUsedAt
		if item.Platform != "" {
			existing.Platform = item.Platform
		}
	}
	for _, groupID := range item.GroupIDs {
		if !int64SliceContains(existing.GroupIDs, groupID) {
			existing.GroupIDs = append(existing.GroupIDs, groupID)
		}
	}
}

func sortedSub2APIOpsRecentAccounts(accounts map[int64]*sub2APIOpsRecentAccount, limit int) []sub2APIOpsRecentAccount {
	result := make([]sub2APIOpsRecentAccount, 0, len(accounts))
	for _, account := range accounts {
		sort.Slice(account.GroupIDs, func(i, j int) bool { return account.GroupIDs[i] < account.GroupIDs[j] })
		result = append(result, *account)
	}
	sort.Slice(result, func(i, j int) bool {
		if !result[i].LastUsedAt.Equal(result[j].LastUsedAt) {
			return result[i].LastUsedAt.After(result[j].LastUsedAt)
		}
		return result[i].ID < result[j].ID
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

func hydrateSub2APIOpsRecentAccountNames(ctx context.Context, client *sub2apiops.Client, accounts []sub2APIOpsRecentAccount) {
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(4)
	for index := range accounts {
		index := index
		group.Go(func() error {
			payload, _, err := client.Do(
				groupCtx,
				http.MethodGet,
				fmt.Sprintf("/accounts/%d", accounts[index].ID),
				nil,
				nil,
			)
			if err != nil {
				return nil
			}
			var account sub2APISafeAccount
			if err := decodeSub2APIData(payload, &account); err != nil {
				return nil
			}
			if account.Name != "" {
				accounts[index].Name = account.Name
			}
			if account.Platform != "" {
				accounts[index].Platform = account.Platform
			}
			return nil
		})
	}
	_ = group.Wait()
}

func int64SliceContains(values []int64, candidate int64) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func maxSub2APIOpsPercentiles(left, right sub2APIOpsPercentiles) sub2APIOpsPercentiles {
	return sub2APIOpsPercentiles{
		P50MS: maxFloat64Pointer(left.P50MS, right.P50MS),
		P90MS: maxFloat64Pointer(left.P90MS, right.P90MS),
		P95MS: maxFloat64Pointer(left.P95MS, right.P95MS),
		P99MS: maxFloat64Pointer(left.P99MS, right.P99MS),
		AvgMS: maxFloat64Pointer(left.AvgMS, right.AvgMS),
		MaxMS: maxFloat64Pointer(left.MaxMS, right.MaxMS),
	}
}

func maxFloat64Pointer(left, right *float64) *float64 {
	if left == nil && right == nil {
		return nil
	}
	value := 0.0
	if left != nil {
		value = *left
	}
	if right != nil && (left == nil || *right > value) {
		value = *right
	}
	return &value
}

func roundedRatio(numerator, denominator int64) float64 {
	if denominator <= 0 {
		return 0
	}
	return math.Round((float64(numerator)/float64(denominator))*10_000) / 10_000
}

func laterRFC3339(left, right string) string {
	if right == "" {
		return left
	}
	if left == "" {
		return right
	}
	leftTime, leftErr := time.Parse(time.RFC3339Nano, left)
	rightTime, rightErr := time.Parse(time.RFC3339Nano, right)
	if leftErr == nil && rightErr == nil {
		if rightTime.After(leftTime) {
			return right
		}
		return left
	}
	if right > left {
		return right
	}
	return left
}

type sub2APIUsageWindowCandidate struct {
	utilization   *float64
	resetAtRaw    string
	resetAfter    *float64
	windowMinutes *float64
	updatedAtRaw  string
	fallbackLabel string
}

func buildSub2APISafeAccountUsageWindows(
	platform string,
	extra *sub2APISafeAccountExtra,
	sessionWindowEnd *time.Time,
	sessionWindowStatus string,
	now time.Time,
) *sub2APISafeAccountUsageWindows {
	var windows []sub2APISafeUsageWindow
	var updatedAt *time.Time

	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "openai":
		windows, updatedAt = buildSub2APIOpenAIUsageWindows(extra, now)
	case "grok":
		windows, updatedAt = buildSub2APIGrokUsageWindows(extra, now)
	default:
		windows, updatedAt = buildSub2APIAnthropicUsageWindows(
			extra,
			sessionWindowEnd,
			sessionWindowStatus,
			now,
		)
	}

	if len(windows) == 0 {
		return nil
	}
	return &sub2APISafeAccountUsageWindows{Items: windows, UpdatedAt: updatedAt}
}

func buildSub2APIOpenAIUsageWindows(extra *sub2APISafeAccountExtra, now time.Time) ([]sub2APISafeUsageWindow, *time.Time) {
	if extra == nil {
		return nil, nil
	}

	updatedAt := parseSub2APITime(extra.CodexUsageUpdatedAt)
	canonicalShortValue := sub2APICodexCandidate(
		extra.Codex5HUsedPercent,
		extra.Codex5HResetAt,
		extra.Codex5HResetAfterSeconds,
		extra.Codex5HWindowMinutes,
		extra.CodexUsageUpdatedAt,
		"5h",
	)
	canonicalLongValue := sub2APICodexCandidate(
		extra.Codex7DUsedPercent,
		extra.Codex7DResetAt,
		extra.Codex7DResetAfterSeconds,
		extra.Codex7DWindowMinutes,
		extra.CodexUsageUpdatedAt,
		"7d",
	)

	// Prefer the raw primary/secondary windows when present. Their minute value
	// is authoritative; the canonical 5h/7d names are historical aliases.
	raw := make([]sub2APIUsageWindowCandidate, 0, 2)
	for _, candidate := range []sub2APIUsageWindowCandidate{
		sub2APICodexCandidate(
			extra.CodexPrimaryUsedPercent,
			"",
			extra.CodexPrimaryResetAfterSeconds,
			extra.CodexPrimaryWindowMinutes,
			extra.CodexUsageUpdatedAt,
			"",
		),
		sub2APICodexCandidate(
			extra.CodexSecondaryUsedPercent,
			"",
			extra.CodexSecondaryResetAfterSeconds,
			extra.CodexSecondaryWindowMinutes,
			extra.CodexUsageUpdatedAt,
			"",
		),
	} {
		if candidate.utilization != nil {
			raw = append(raw, candidate)
		}
	}

	var short, long *sub2APIUsageWindowCandidate
	if len(raw) > 0 {
		short = chooseSub2APICodexShortWindow(raw)
		long = chooseSub2APICodexLongWindow(raw)
		if short == nil && long == nil && len(raw) == 1 {
			if sub2APIWindowMinutesValue(raw[0].windowMinutes) > 0 && sub2APIWindowMinutesValue(raw[0].windowMinutes) <= 360 {
				short = &raw[0]
			} else {
				long = &raw[0]
			}
		}
	} else {
		if canonicalShortValue.utilization != nil {
			short = &canonicalShortValue
		}
		if canonicalLongValue.utilization != nil {
			long = &canonicalLongValue
		}
	}

	result := make([]sub2APISafeUsageWindow, 0, 2)
	if window := buildSub2APISafeUsageWindow(short, "5h", now); window != nil {
		result = append(result, *window)
	}
	if window := buildSub2APISafeUsageWindow(long, "7d", now); window != nil {
		result = append(result, *window)
	}
	return result, updatedAt
}

func sub2APICodexCandidate(
	utilization *float64,
	resetAtRaw string,
	resetAfter *float64,
	windowMinutes *float64,
	updatedAtRaw string,
	fallbackLabel string,
) sub2APIUsageWindowCandidate {
	return sub2APIUsageWindowCandidate{
		utilization:   utilization,
		resetAtRaw:    resetAtRaw,
		resetAfter:    resetAfter,
		windowMinutes: windowMinutes,
		updatedAtRaw:  updatedAtRaw,
		fallbackLabel: fallbackLabel,
	}
}

func chooseSub2APICodexShortWindow(candidates []sub2APIUsageWindowCandidate) *sub2APIUsageWindowCandidate {
	var selected *sub2APIUsageWindowCandidate
	for index := range candidates {
		minutes := sub2APIWindowMinutesValue(candidates[index].windowMinutes)
		if minutes <= 0 || minutes > 360 {
			continue
		}
		if selected == nil || absInt(minutes-300) < absInt(sub2APIWindowMinutesValue(selected.windowMinutes)-300) {
			selected = &candidates[index]
		}
	}
	return selected
}

func chooseSub2APICodexLongWindow(candidates []sub2APIUsageWindowCandidate) *sub2APIUsageWindowCandidate {
	var selected *sub2APIUsageWindowCandidate
	for index := range candidates {
		minutes := sub2APIWindowMinutesValue(candidates[index].windowMinutes)
		if minutes <= 360 {
			continue
		}
		// Prefer the shortest long window, normally 7d, and fall back to 30d
		// or another provider-defined window when 7d is absent.
		if selected == nil || minutes < sub2APIWindowMinutesValue(selected.windowMinutes) {
			selected = &candidates[index]
		}
	}
	return selected
}

func buildSub2APIAnthropicUsageWindows(
	extra *sub2APISafeAccountExtra,
	sessionWindowEnd *time.Time,
	sessionWindowStatus string,
	now time.Time,
) ([]sub2APISafeUsageWindow, *time.Time) {
	if extra == nil {
		extra = &sub2APISafeAccountExtra{}
	}
	updatedAt := parseSub2APITime(extra.PassiveUsageSampledAt)
	var fiveHour *sub2APISafeUsageWindow
	if extra.SessionWindowUtilization != nil {
		utilization := *extra.SessionWindowUtilization * 100
		fiveHour = normalizedSub2APIUsageWindow("5h", utilization, sessionWindowEnd, intPtr(300), now)
	} else if sessionWindowEnd != nil {
		utilization := 0.0
		switch sessionWindowStatus {
		case "rejected":
			utilization = 100
		case "allowed_warning":
			utilization = 80
		}
		fiveHour = normalizedSub2APIUsageWindow("5h", utilization, sessionWindowEnd, intPtr(300), now)
	}

	var sevenDay *sub2APISafeUsageWindow
	if extra.PassiveUsage7DUtilization != nil || extra.PassiveUsage7DReset != nil {
		sevenDay = normalizedSub2APIUsageWindow(
			"7d",
			sub2APIPercentage(extra.PassiveUsage7DUtilization),
			sub2APIUnixTime(extra.PassiveUsage7DReset),
			intPtr(7*24*60),
			now,
		)
	} else if extra.PassiveUsage7DOIUtilization != nil || extra.PassiveUsage7DOIReset != nil {
		sevenDay = normalizedSub2APIUsageWindow(
			"7d",
			sub2APIPercentage(extra.PassiveUsage7DOIUtilization),
			sub2APIUnixTime(extra.PassiveUsage7DOIReset),
			intPtr(7*24*60),
			now,
		)
	}

	result := make([]sub2APISafeUsageWindow, 0, 2)
	if fiveHour != nil {
		result = append(result, *fiveHour)
	}
	if sevenDay != nil {
		result = append(result, *sevenDay)
	}
	return result, updatedAt
}

func buildSub2APIGrokUsageWindows(extra *sub2APISafeAccountExtra, now time.Time) ([]sub2APISafeUsageWindow, *time.Time) {
	if extra == nil || extra.GrokBilling == nil {
		return nil, nil
	}
	billing := extra.GrokBilling
	var window *sub2APISafeUsageWindow
	var updatedAt *time.Time
	if strings.EqualFold(strings.TrimSpace(billing.PeriodType), "weekly") && billing.UsagePercent != nil {
		window = normalizedSub2APIUsageWindow(
			"7d",
			*billing.UsagePercent,
			parseSub2APITime(billing.PeriodEnd),
			sub2APIBillingWindowMinutes(billing.PeriodStart, billing.PeriodEnd),
			now,
		)
		updatedAt = firstSub2APITime(billing.WeeklyUpdatedAt, billing.UpdatedAt, billing.FetchedAt)
	} else if billing.UsedPercent != nil {
		window = normalizedSub2APIUsageWindow(
			"monthly",
			*billing.UsedPercent,
			parseSub2APITime(billing.BillingPeriodEnd),
			sub2APIBillingWindowMinutes(billing.BillingPeriodStart, billing.BillingPeriodEnd),
			now,
		)
		updatedAt = firstSub2APITime(billing.MonthlyUpdatedAt, billing.UpdatedAt, billing.FetchedAt)
	}
	if window == nil {
		return nil, updatedAt
	}
	return []sub2APISafeUsageWindow{*window}, updatedAt
}

func buildSub2APISafeUsageWindow(candidate *sub2APIUsageWindowCandidate, fallbackLabel string, now time.Time) *sub2APISafeUsageWindow {
	if candidate == nil || candidate.utilization == nil {
		return nil
	}
	resetAt := parseSub2APITime(candidate.resetAtRaw)
	if resetAt == nil && candidate.resetAfter != nil && *candidate.resetAfter > 0 {
		base := now
		if updatedAt := parseSub2APITime(candidate.updatedAtRaw); updatedAt != nil {
			base = *updatedAt
		}
		value := base.Add(time.Duration(*candidate.resetAfter * float64(time.Second)))
		resetAt = &value
	}
	minutes := sub2APIWindowMinutes(candidate.windowMinutes)
	label := candidate.fallbackLabel
	if label == "" {
		label = fallbackLabel
	}
	return normalizedSub2APIUsageWindow(label, *candidate.utilization, resetAt, minutes, now)
}

func normalizedSub2APIUsageWindow(label string, utilization float64, resetAt *time.Time, windowMinutes *int, now time.Time) *sub2APISafeUsageWindow {
	if math.IsNaN(utilization) || math.IsInf(utilization, 0) || utilization < 0 {
		utilization = 0
	}
	if resetAt != nil && !now.Before(*resetAt) {
		utilization = 0
		resetAt = nil
	}
	return &sub2APISafeUsageWindow{
		Label:         usageWindowLabel(windowMinutes, label),
		Utilization:   utilization,
		WindowMinutes: windowMinutes,
		ResetsAt:      resetAt,
	}
}

func sub2APIPercentage(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value * 100
}

func sub2APIUnixTime(value *float64) *time.Time {
	if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) || *value <= 0 {
		return nil
	}
	result := time.Unix(int64(*value), 0)
	return &result
}

func sub2APIWindowMinutes(value *float64) *int {
	if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) || *value <= 0 {
		return nil
	}
	minutes := int(math.Round(*value))
	if minutes <= 0 {
		return nil
	}
	return &minutes
}

func sub2APIWindowMinutesValue(value *float64) int {
	minutes := sub2APIWindowMinutes(value)
	if minutes == nil {
		return 0
	}
	return *minutes
}

func usageWindowLabel(windowMinutes *int, fallback string) string {
	if strings.EqualFold(strings.TrimSpace(fallback), "monthly") {
		return "monthly"
	}
	if windowMinutes == nil || *windowMinutes <= 0 {
		return fallback
	}
	minutes := *windowMinutes
	if minutes%(24*60) == 0 {
		return fmt.Sprintf("%dd", minutes/(24*60))
	}
	if minutes%60 == 0 {
		return fmt.Sprintf("%dh", minutes/60)
	}
	return fmt.Sprintf("%dm", minutes)
}

func sub2APIBillingWindowMinutes(startRaw, endRaw string) *int {
	start := parseSub2APITime(startRaw)
	end := parseSub2APITime(endRaw)
	if start == nil || end == nil || !end.After(*start) {
		return nil
	}
	minutes := int(math.Round(end.Sub(*start).Minutes()))
	if minutes <= 0 {
		return nil
	}
	return &minutes
}

func firstSub2APITime(values ...string) *time.Time {
	for _, value := range values {
		if parsed := parseSub2APITime(value); parsed != nil {
			return parsed
		}
	}
	return nil
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func intPtr(value int) *int { return &value }

func parseSub2APITime(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if value, err := time.Parse(layout, raw); err == nil {
			return &value
		}
	}
	return nil
}

func cloneURLValues(values url.Values) url.Values {
	cloned := make(url.Values, len(values))
	for key, items := range values {
		cloned[key] = append([]string(nil), items...)
	}
	return cloned
}

func getSub2APIOpsGroups(c *gin.Context, d *Deps) {
	client, err := sub2APIOpsClient(d, "", "")
	if err != nil {
		writeSub2APIOpsClientError(c, d, err)
		return
	}
	payload, status, err := client.Do(
		c.Request.Context(),
		http.MethodGet,
		"/groups/all",
		url.Values{"include_inactive": {"true"}},
		nil,
	)
	if err != nil {
		writeSub2APIOpsError(c, err)
		return
	}
	var groups []sub2APISafeGroup
	if err := decodeSub2APIData(payload, &groups); err != nil {
		fail(c, http.StatusBadGateway, err)
		return
	}
	if groups == nil {
		groups = []sub2APISafeGroup{}
	}
	c.JSON(status, gin.H{"data": groups})
}

func getSub2APIOpsAccounts(c *gin.Context, d *Deps) {
	query := url.Values{}
	page, err := positiveIntQuery(c, "page", 1, 100000)
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	pageSize, err := positiveIntQuery(c, "page_size", 20, 100)
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	query.Set("page", strconv.Itoa(page))
	query.Set("page_size", strconv.Itoa(pageSize))
	query.Set("lite", "1")

	if platform := c.Query("platform"); platform != "" {
		if !validAccountPlatform(platform) {
			fail(c, http.StatusBadRequest, errors.New("不支持的平台"))
			return
		}
		query.Set("platform", platform)
	}
	if accountType := c.Query("type"); accountType != "" {
		if !stringAllowed(accountType, "oauth", "setup-token", "apikey", "upstream", "bedrock", "service_account") {
			fail(c, http.StatusBadRequest, errors.New("不支持的账号类型"))
			return
		}
		query.Set("type", accountType)
	}
	statuses, err := selectedSub2APIAccountStatuses(c, "statuses", "status")
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	excludedStatuses, err := selectedSub2APIAccountStatuses(c, "exclude_statuses", "")
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	if len(statuses) > 0 && len(excludedStatuses) > 0 {
		fail(c, http.StatusBadRequest, errors.New("指定运行状态时不能同时排除状态"))
		return
	}
	groups, err := selectedSub2APIAccountGroups(c)
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	if search := strings.TrimSpace(c.Query("search")); search != "" {
		if len(search) > 100 {
			fail(c, http.StatusBadRequest, errors.New("搜索内容不能超过 100 个字节"))
			return
		}
		query.Set("search", search)
	}
	sortBy := c.DefaultQuery("sort_by", "name")
	if !stringAllowed(
		sortBy,
		"id", "name", "status", "schedulable", "priority", "rate_multiplier",
		"concurrency", "current_concurrency", "last_used_at", "created_at", "expires_at",
	) {
		fail(c, http.StatusBadRequest, errors.New("不支持的账号排序字段"))
		return
	}
	sortOrder := c.DefaultQuery("sort_order", "asc")
	if !stringAllowed(sortOrder, "asc", "desc") {
		fail(c, http.StatusBadRequest, errors.New("排序方向必须是 asc 或 desc"))
		return
	}
	query.Set("sort_by", sortBy)
	query.Set("sort_order", sortOrder)
	if c.Query("include_scheduler_score") == "1" {
		query.Set("include_scheduler_score", "1")
	} else {
		query.Set("include_scheduler_score", "0")
	}

	localFilter := len(statuses) > 1 || len(excludedStatuses) > 0 || len(groups) > 1
	localSort := stringAllowed(sortBy, "concurrency", "current_concurrency", "last_used_at", "expires_at")
	if !localFilter && !localSort {
		if len(statuses) == 1 {
			query.Set("status", statuses[0])
		}
		if len(groups) == 1 {
			query.Set("group", groups[0])
		}
		proxySub2APIOpsAccounts(c, d, query)
		return
	}

	client, err := sub2APIOpsClient(d, "", "")
	if err != nil {
		writeSub2APIOpsClientError(c, d, err)
		return
	}
	candidates, status, err := fetchSub2APIOpsAccountCandidates(c.Request.Context(), client, query)
	if err != nil {
		writeSub2APIOpsError(c, err)
		return
	}
	candidates = filterSub2APIOpsAccounts(candidates, statuses, excludedStatuses, groups, time.Now())
	sortSub2APIOpsAccounts(candidates, sortBy, sortOrder)
	total := len(candidates)
	pages := 1
	if total > 0 {
		pages = (total + pageSize - 1) / pageSize
	}
	start := (page - 1) * pageSize
	items := []sub2APISafeAccount{}
	if start < total {
		end := min(start+pageSize, total)
		items = candidates[start:end]
	}
	c.JSON(status, gin.H{"data": sub2APISafeAccountPage{
		Items:    items,
		Total:    int64(total),
		Page:     page,
		PageSize: pageSize,
		Pages:    pages,
	}})
}

func selectedSub2APIAccountStatuses(c *gin.Context, key, legacyKey string) ([]string, error) {
	raw := strings.TrimSpace(c.Query(key))
	if legacyKey != "" {
		legacy := strings.TrimSpace(c.Query(legacyKey))
		if raw != "" && legacy != "" {
			return nil, fmt.Errorf("%s 和 %s 不能同时使用", key, legacyKey)
		}
		if raw == "" {
			raw = legacy
		}
	}
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	if len(parts) > 6 {
		return nil, errors.New("账号状态不能超过 6 个")
	}
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		status := strings.TrimSpace(part)
		if !stringAllowed(status, "active", "inactive", "error", "rate_limited", "temp_unschedulable", "unschedulable") {
			return nil, errors.New("不支持的账号状态")
		}
		if _, ok := seen[status]; ok {
			return nil, errors.New("账号状态不能重复")
		}
		seen[status] = struct{}{}
		result = append(result, status)
	}
	return result, nil
}

func selectedSub2APIAccountGroups(c *gin.Context) ([]string, error) {
	raw := strings.TrimSpace(c.Query("groups"))
	legacy := strings.TrimSpace(c.Query("group"))
	if raw != "" && legacy != "" {
		return nil, errors.New("groups 和 group 不能同时使用")
	}
	if raw == "" {
		raw = legacy
	}
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	if len(parts) > maxAccountFilterGroups {
		return nil, fmt.Errorf("最多选择 %d 个账号分组", maxAccountFilterGroups)
	}
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		group := strings.TrimSpace(part)
		if group != "ungrouped" {
			id, err := strconv.ParseInt(group, 10, 64)
			if err != nil || id <= 0 {
				return nil, errors.New("账号分组必须是正整数或 ungrouped")
			}
			group = strconv.FormatInt(id, 10)
		}
		if _, ok := seen[group]; ok {
			return nil, errors.New("账号分组不能重复")
		}
		seen[group] = struct{}{}
		result = append(result, group)
	}
	return result, nil
}

func fetchSub2APIOpsAccountCandidates(
	ctx context.Context,
	client *sub2apiops.Client,
	baseQuery url.Values,
) ([]sub2APISafeAccount, int, error) {
	query := cloneURLValues(baseQuery)
	query.Del("page")
	query.Del("page_size")
	query.Del("status")
	query.Del("group")
	query.Set("page_size", strconv.Itoa(opsAccountCandidateSize))
	query.Set("sort_by", "id")
	query.Set("sort_order", "asc")
	accounts := make([]sub2APISafeAccount, 0, opsAccountCandidateSize)
	status := http.StatusOK
	for pageNumber := 1; ; pageNumber++ {
		query.Set("page", strconv.Itoa(pageNumber))
		payload, currentStatus, err := client.Do(ctx, http.MethodGet, "/accounts", query, nil)
		if err != nil {
			return nil, currentStatus, err
		}
		status = currentStatus
		var page sub2APISafeAccountPage
		if err := decodeSub2APIData(payload, &page); err != nil {
			return nil, status, err
		}
		if len(accounts)+len(page.Items) > maxOpsAccountCandidates || page.Total > maxOpsAccountCandidates {
			return nil, status, fmt.Errorf("账号数量超过本地筛选上限 %d", maxOpsAccountCandidates)
		}
		accounts = append(accounts, page.Items...)
		if len(page.Items) == 0 || int64(len(accounts)) >= page.Total || pageNumber >= page.Pages {
			break
		}
	}
	return accounts, status, nil
}

func filterSub2APIOpsAccounts(
	accounts []sub2APISafeAccount,
	statuses []string,
	excludedStatuses []string,
	groups []string,
	now time.Time,
) []sub2APISafeAccount {
	included := make(map[string]struct{}, len(statuses))
	for _, status := range statuses {
		included[status] = struct{}{}
	}
	excluded := make(map[string]struct{}, len(excludedStatuses))
	for _, status := range excludedStatuses {
		excluded[status] = struct{}{}
	}
	selectedGroups := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		selectedGroups[group] = struct{}{}
	}

	result := make([]sub2APISafeAccount, 0, len(accounts))
	for _, account := range accounts {
		status := sub2APIOpsAccountFilterStatus(account, now)
		if len(included) > 0 {
			if _, ok := included[status]; !ok {
				continue
			}
		}
		if _, ok := excluded[status]; ok {
			continue
		}
		if len(selectedGroups) > 0 && !sub2APIOpsAccountMatchesGroups(account, selectedGroups) {
			continue
		}
		result = append(result, account)
	}
	return result
}

func sub2APIOpsAccountFilterStatus(account sub2APISafeAccount, now time.Time) string {
	if account.Status != "active" {
		return account.Status
	}
	if account.TempUnschedulableUntil != nil && account.TempUnschedulableUntil.After(now) {
		return "temp_unschedulable"
	}
	if account.RateLimitResetAt != nil && account.RateLimitResetAt.After(now) {
		return "rate_limited"
	}
	if !account.Schedulable {
		return "unschedulable"
	}
	return "active"
}

func sub2APIOpsAccountMatchesGroups(account sub2APISafeAccount, selected map[string]struct{}) bool {
	groupIDs := account.GroupIDs
	if len(groupIDs) == 0 && len(account.Groups) > 0 {
		groupIDs = make([]int64, 0, len(account.Groups))
		for _, group := range account.Groups {
			groupIDs = append(groupIDs, group.ID)
		}
	}
	if len(groupIDs) == 0 {
		_, ok := selected["ungrouped"]
		return ok
	}
	for _, groupID := range groupIDs {
		if _, ok := selected[strconv.FormatInt(groupID, 10)]; ok {
			return true
		}
	}
	return false
}

func sortSub2APIOpsAccounts(accounts []sub2APISafeAccount, sortBy, sortOrder string) {
	descending := sortOrder == "desc"
	now := time.Now()
	sort.SliceStable(accounts, func(i, j int) bool {
		left, right := accounts[i], accounts[j]
		if sortBy == "last_used_at" && (left.LastUsedAt == nil || right.LastUsedAt == nil) {
			if left.LastUsedAt == nil && right.LastUsedAt != nil {
				return false
			}
			if left.LastUsedAt != nil && right.LastUsedAt == nil {
				return true
			}
		}
		if sortBy == "expires_at" && (left.ExpiresAt == nil || right.ExpiresAt == nil) {
			if left.ExpiresAt == nil && right.ExpiresAt != nil {
				return false
			}
			if left.ExpiresAt != nil && right.ExpiresAt == nil {
				return true
			}
		}

		comparison := 0
		switch sortBy {
		case "id":
			comparison = compareInt64(left.ID, right.ID)
		case "status":
			comparison = strings.Compare(sub2APIOpsAccountFilterStatus(left, now), sub2APIOpsAccountFilterStatus(right, now))
		case "schedulable":
			comparison = compareBool(left.Schedulable, right.Schedulable)
		case "priority":
			comparison = compareInt(left.Priority, right.Priority)
		case "rate_multiplier":
			comparison = compareFloat64(left.RateMultiplier, right.RateMultiplier)
		case "concurrency":
			comparison = compareInt(left.Concurrency, right.Concurrency)
		case "current_concurrency":
			comparison = compareInt(left.CurrentConcurrency, right.CurrentConcurrency)
		case "last_used_at":
			if left.LastUsedAt != nil && right.LastUsedAt != nil {
				comparison = left.LastUsedAt.Compare(*right.LastUsedAt)
			}
		case "created_at":
			comparison = left.CreatedAt.Compare(right.CreatedAt)
		case "expires_at":
			if left.ExpiresAt != nil && right.ExpiresAt != nil {
				comparison = compareInt64(*left.ExpiresAt, *right.ExpiresAt)
			}
		default:
			comparison = strings.Compare(strings.ToLower(left.Name), strings.ToLower(right.Name))
		}
		if comparison == 0 {
			comparison = compareInt64(left.ID, right.ID)
		}
		if descending {
			return comparison > 0
		}
		return comparison < 0
	})
}

func compareInt(left, right int) int {
	return compareInt64(int64(left), int64(right))
}

func compareInt64(left, right int64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func compareFloat64(left, right float64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func compareBool(left, right bool) int {
	if left == right {
		return 0
	}
	if !left {
		return -1
	}
	return 1
}

func getSub2APIOpsTodayStats(c *gin.Context, d *Deps) {
	var input struct {
		AccountIDs []int64 `json:"account_ids"`
	}
	if err := bindLimitedJSON(c, &input); err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	if len(input.AccountIDs) == 0 || len(input.AccountIDs) > 100 {
		fail(c, http.StatusBadRequest, errors.New("account_ids 数量必须在 1 到 100 之间"))
		return
	}
	seen := make(map[int64]struct{}, len(input.AccountIDs))
	for _, id := range input.AccountIDs {
		if id <= 0 {
			fail(c, http.StatusBadRequest, errors.New("账号 ID 必须是正整数"))
			return
		}
		seen[id] = struct{}{}
	}
	if len(seen) != len(input.AccountIDs) {
		fail(c, http.StatusBadRequest, errors.New("account_ids 不能重复"))
		return
	}
	payload, _ := json.Marshal(input)
	stats := sub2APISafeTodayStatsResponse{Stats: map[string]sub2APISafeAccountStats{}}
	proxyProjectedSub2APIOps(c, d, http.MethodPost, "/accounts/today-stats/batch", nil, payload, &stats)
}

func getSub2APIOpsAccountUsageDetails(c *gin.Context, d *Deps) {
	accountIDs, err := parsePositiveInt64List(c.Query("account_ids"), maxAccountUsageDetailIDs)
	if err != nil || len(accountIDs) == 0 {
		if err == nil {
			err = errors.New("account_ids 不能为空")
		}
		fail(c, http.StatusBadRequest, err)
		return
	}
	windowAccountIDs, err := parsePositiveInt64List(c.Query("window_account_ids"), maxAccountUsageDetailIDs)
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	accountSet := make(map[int64]struct{}, len(accountIDs))
	for _, accountID := range accountIDs {
		accountSet[accountID] = struct{}{}
	}
	windowSet := make(map[int64]struct{}, len(windowAccountIDs))
	for _, accountID := range windowAccountIDs {
		if _, ok := accountSet[accountID]; !ok {
			fail(c, http.StatusBadRequest, errors.New("window_account_ids 必须是 account_ids 的子集"))
			return
		}
		windowSet[accountID] = struct{}{}
	}
	includeTotal := c.DefaultQuery("include_total", "1")
	if !stringAllowed(includeTotal, "0", "1") {
		fail(c, http.StatusBadRequest, errors.New("include_total 必须是 0 或 1"))
		return
	}

	client, err := sub2APIOpsClient(d, "", "")
	if err != nil {
		writeSub2APIOpsClientError(c, d, err)
		return
	}
	type detailResult struct {
		id        int64
		detail    sub2APISafeAccountUsageDetail
		totalErr  error
		windowErr error
	}
	requestCtx, cancel := context.WithTimeout(c.Request.Context(), accountUsageDetailTimeout)
	defer cancel()
	results := make([]detailResult, len(accountIDs))
	group, groupCtx := errgroup.WithContext(requestCtx)
	group.SetLimit(4)
	for index, accountID := range accountIDs {
		index, accountID := index, accountID
		group.Go(func() error {
			result := detailResult{id: accountID}
			select {
			case sub2APIOpsUsageDetailSlots <- struct{}{}:
				defer func() { <-sub2APIOpsUsageDetailSlots }()
			case <-groupCtx.Done():
				if includeTotal == "1" {
					result.totalErr = groupCtx.Err()
				}
				if _, ok := windowSet[accountID]; ok {
					result.windowErr = groupCtx.Err()
				}
				results[index] = result
				return nil
			}
			if includeTotal == "1" {
				result.detail.Total, result.totalErr = fetchSub2APIOpsAccountTotalStats(groupCtx, client, accountID)
			}
			if _, ok := windowSet[accountID]; ok {
				result.detail.WindowStats, result.windowErr = fetchSub2APIOpsAccountWindowStats(groupCtx, client, accountID)
			}
			results[index] = result
			return nil
		})
	}
	_ = group.Wait()

	response := sub2APISafeAccountUsageDetailsResponse{
		Details:         make(map[string]sub2APISafeAccountUsageDetail, len(results)),
		FailedTotalIDs:  []int64{},
		FailedWindowIDs: []int64{},
	}
	var firstTotalErr error
	totalSuccesses := 0
	for _, result := range results {
		response.Details[strconv.FormatInt(result.id, 10)] = result.detail
		if result.totalErr != nil {
			response.FailedTotalIDs = append(response.FailedTotalIDs, result.id)
			if firstTotalErr == nil {
				firstTotalErr = result.totalErr
			}
		} else if includeTotal == "1" {
			totalSuccesses++
		}
		if result.windowErr != nil {
			response.FailedWindowIDs = append(response.FailedWindowIDs, result.id)
		}
	}
	if includeTotal == "1" && totalSuccesses == 0 && firstTotalErr != nil {
		writeSub2APIOpsError(c, firstTotalErr)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": response})
}

func fetchSub2APIOpsAccountTotalStats(ctx context.Context, client *sub2apiops.Client, accountID int64) (*sub2APISafeAccountStats, error) {
	query := url.Values{
		"account_id": {strconv.FormatInt(accountID, 10)},
		"start_date": {"1970-01-01"},
		"end_date":   {time.Now().UTC().Format("2006-01-02")},
		"timezone":   {"UTC"},
	}
	payload, _, err := client.Do(ctx, http.MethodGet, "/usage/stats", query, nil)
	if err != nil {
		return nil, err
	}
	var upstream sub2APIUpstreamUsageStats
	if err := decodeSub2APIData(payload, &upstream); err != nil {
		return nil, err
	}
	accountCost := upstream.TotalCost
	if upstream.TotalAccountCost != nil {
		accountCost = *upstream.TotalAccountCost
	}
	return &sub2APISafeAccountStats{
		Requests:     upstream.TotalRequests,
		Tokens:       upstream.TotalTokens,
		Cost:         accountCost,
		StandardCost: upstream.TotalCost,
		UserCost:     upstream.TotalActualCost,
	}, nil
}

func fetchSub2APIOpsAccountWindowStats(ctx context.Context, client *sub2apiops.Client, accountID int64) (map[string]sub2APISafeAccountStats, error) {
	payload, _, err := client.Do(
		ctx,
		http.MethodGet,
		fmt.Sprintf("/accounts/%d/usage", accountID),
		nil,
		nil,
	)
	if err != nil {
		return nil, err
	}
	var usage sub2APIUpstreamAccountUsage
	if err := decodeSub2APIData(payload, &usage); err != nil {
		return nil, err
	}
	return projectSub2APIAccountWindowStats(usage), nil
}

func projectSub2APIAccountWindowStats(usage sub2APIUpstreamAccountUsage) map[string]sub2APISafeAccountStats {
	windows := make(map[string]sub2APISafeAccountStats, 12)
	addProgress := func(label string, progress *sub2APIUpstreamUsageProgress) {
		if progress != nil && progress.WindowStats != nil {
			windows[label] = *progress.WindowStats
		}
	}
	addProgress("5h", usage.FiveHour)
	addProgress("7d", usage.SevenDay)
	addProgress("7d_sonnet", usage.SevenDaySonnet)
	addProgress("7d_fable", usage.SevenDayFable)
	addProgress("gemini_shared_daily", usage.GeminiSharedDaily)
	addProgress("gemini_pro_daily", usage.GeminiProDaily)
	addProgress("gemini_flash_daily", usage.GeminiFlashDaily)
	addProgress("gemini_shared_minute", usage.GeminiSharedMinute)
	addProgress("gemini_pro_minute", usage.GeminiProMinute)
	addProgress("gemini_flash_minute", usage.GeminiFlashMinute)
	if usage.GrokLocalUsage24H != nil {
		windows["24h"] = *usage.GrokLocalUsage24H
	}
	if usage.GrokLocalUsage7D != nil {
		windows["7d"] = *usage.GrokLocalUsage7D
	}
	if usage.GrokLocalUsageMonthly != nil {
		windows["monthly"] = *usage.GrokLocalUsageMonthly
	}
	return windows
}

func proxySub2APIOpsAccounts(c *gin.Context, d *Deps, query url.Values) {
	client, err := sub2APIOpsClient(d, "", "")
	if err != nil {
		writeSub2APIOpsClientError(c, d, err)
		return
	}
	payload, status, err := client.Do(c.Request.Context(), http.MethodGet, "/accounts", query, nil)
	if err != nil {
		writeSub2APIOpsError(c, err)
		return
	}
	var page sub2APISafeAccountPage
	if err := decodeSub2APIData(payload, &page); err != nil {
		fail(c, http.StatusBadGateway, err)
		return
	}
	if page.Items == nil {
		page.Items = []sub2APISafeAccount{}
	}
	c.JSON(status, gin.H{"data": page})
}

func decodeSub2APIData(payload []byte, target any) error {
	var envelope struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil || envelope.Code != 0 || len(envelope.Data) == 0 {
		return errors.New("Sub2API 返回了无效的管理响应")
	}
	if err := json.Unmarshal(envelope.Data, target); err != nil {
		return errors.New("Sub2API 返回了不兼容的数据格式")
	}
	return nil
}

func proxyProjectedSub2APIOps(c *gin.Context, d *Deps, method, endpoint string, query url.Values, body []byte, target any) {
	client, err := sub2APIOpsClient(d, "", "")
	if err != nil {
		writeSub2APIOpsClientError(c, d, err)
		return
	}
	payload, status, err := client.Do(c.Request.Context(), method, endpoint, query, body)
	if err != nil {
		writeSub2APIOpsError(c, err)
		return
	}
	if err := decodeSub2APIData(payload, target); err != nil {
		fail(c, http.StatusBadGateway, err)
		return
	}
	c.JSON(status, gin.H{"data": target})
}

func sub2APIOpsClient(d *Deps, siteURL, adminKey string) (*sub2apiops.Client, error) {
	config, err := d.Sub2APIOps.Get()
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if siteURL == "" {
		if config == nil {
			return nil, errSub2APIOpsNotConfigured
		}
		siteURL = config.SiteURL
	} else {
		normalized, normalizeErr := sub2apiops.NormalizeSiteURL(siteURL)
		if normalizeErr != nil {
			return nil, &sub2APIOpsInputError{err: normalizeErr}
		}
		if config != nil {
			if bindingErr := validateAdminKeyBinding(config.SiteURL, normalized, adminKey); bindingErr != nil {
				return nil, &sub2APIOpsInputError{err: bindingErr}
			}
		}
		siteURL = normalized
	}
	if adminKey == "" {
		if config == nil || config.AdminKeyCipher == "" {
			return nil, errSub2APIOpsNotConfigured
		}
		adminKey, err = d.Cipher.Decrypt(config.AdminKeyCipher)
		if err != nil {
			return nil, fmt.Errorf("解密 Admin Key: %w", err)
		}
	}
	client, err := sub2apiops.NewClient(siteURL, adminKey)
	if err != nil {
		return nil, &sub2APIOpsInputError{err: err}
	}
	return client, nil
}

func writeSub2APIOpsClientError(c *gin.Context, d *Deps, err error) {
	var inputErr *sub2APIOpsInputError
	switch {
	case errors.As(err, &inputErr):
		fail(c, http.StatusBadRequest, inputErr)
	case errors.Is(err, errSub2APIOpsNotConfigured):
		fail(c, http.StatusConflict, errSub2APIOpsNotConfigured)
	default:
		if d.Log != nil {
			d.Log.Error("load Sub2API ops client failed", "err", err)
		}
		fail(c, http.StatusInternalServerError, errors.New("加载 Sub2API 运维连接失败"))
	}
}

func writeSub2APIOpsError(c *gin.Context, err error) {
	status := http.StatusBadGateway
	if errors.Is(err, context.DeadlineExceeded) {
		status = http.StatusGatewayTimeout
	}
	var remoteErr *sub2apiops.RemoteError
	if errors.As(err, &remoteErr) {
		c.JSON(status, gin.H{"error": remoteErr.Error(), "upstream_status": remoteErr.StatusCode})
		return
	}
	fail(c, status, err)
}

func sub2APIOpsConfigResponse(config *storage.Sub2APIOpsConfig) sub2APIOpsConfigView {
	view := sub2APIOpsConfigView{
		Configured:         config.AdminKeyCipher != "" && config.SiteURL != "",
		Name:               config.Name,
		SiteURL:            config.SiteURL,
		AdminKeyConfigured: config.AdminKeyCipher != "",
		UpdatedAt:          &config.UpdatedAt,
	}
	if json.Valid([]byte(config.DashboardLayout)) {
		view.Layout = json.RawMessage(config.DashboardLayout)
	}
	return view
}

func validateDashboardLayout(layout dashboardLayoutInput) error {
	if layout.Version != 1 {
		return errors.New("不支持的大盘布局版本")
	}
	if len(layout.Widgets) > maxWidgets {
		return fmt.Errorf("大盘组件不能超过 %d 个", maxWidgets)
	}
	seen := make(map[string]struct{}, len(layout.Widgets))
	for _, widget := range layout.Widgets {
		if !dashboardWidgetIDPattern.MatchString(widget.ID) {
			return errors.New("组件 ID 格式不合法")
		}
		if _, exists := seen[widget.ID]; exists {
			return errors.New("组件 ID 不能重复")
		}
		seen[widget.ID] = struct{}{}
		if !dashboardWidgetIDPattern.MatchString(widget.Type) {
			return errors.New("组件类型格式不合法")
		}
		if widget.Width != 4 && widget.Width != 6 && widget.Width != 8 && widget.Width != 12 {
			return errors.New("组件宽度必须是 4、6、8 或 12")
		}
		if !stringAllowed(widget.Height, "compact", "standard", "tall") {
			return errors.New("组件高度必须是 compact、standard 或 tall")
		}
		if len(widget.Config) > 16<<10 || !json.Valid(widget.Config) || len(widget.Config) == 0 || widget.Config[0] != '{' {
			return errors.New("组件配置必须是小于 16 KiB 的 JSON 对象")
		}
	}
	return nil
}

func bindLimitedJSON(c *gin.Context, target any) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxOpsConfigBody)
	return c.ShouldBindJSON(target)
}

func bindOptionalLimitedJSON(c *gin.Context, target any) error {
	err := bindLimitedJSON(c, target)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

func positiveIntQuery(c *gin.Context, key string, fallback, max int) (int, error) {
	raw := c.DefaultQuery(key, strconv.Itoa(fallback))
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 || value > max {
		return 0, fmt.Errorf("%s 必须是 1 到 %d 之间的整数", key, max)
	}
	return value, nil
}

func selectedSub2APIGroupIDs(c *gin.Context) ([]int64, error) {
	groupIDs := strings.TrimSpace(c.Query("group_ids"))
	groupID := strings.TrimSpace(c.Query("group_id"))
	if groupIDs != "" && groupID != "" {
		return nil, errors.New("group_id 和 group_ids 不能同时使用")
	}
	if groupIDs == "" {
		groupIDs = groupID
	}
	return parsePositiveInt64List(groupIDs, maxSnapshotGroups)
}

func parsePositiveInt64List(raw string, maxItems int) ([]int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	if len(parts) > maxItems {
		return nil, fmt.Errorf("最多选择 %d 个分组", maxItems)
	}
	result := make([]int64, 0, len(parts))
	seen := make(map[int64]struct{}, len(parts))
	for _, part := range parts {
		value, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || value <= 0 {
			return nil, errors.New("分组 ID 必须是正整数")
		}
		if _, exists := seen[value]; exists {
			return nil, errors.New("分组 ID 不能重复")
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func validAccountPlatform(value string) bool {
	return stringAllowed(value, "anthropic", "openai", "gemini", "antigravity", "grok")
}

func sub2APIOpsTimeRangeDuration(value string) (time.Duration, bool) {
	switch value {
	case "5m":
		return 5 * time.Minute, true
	case "30m":
		return 30 * time.Minute, true
	case "1h":
		return time.Hour, true
	case "6h":
		return 6 * time.Hour, true
	case "24h":
		return 24 * time.Hour, true
	case "7d":
		return 7 * 24 * time.Hour, true
	case "30d":
		return 30 * 24 * time.Hour, true
	default:
		return 0, false
	}
}

func stringAllowed(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func validateAdminKeyBinding(currentSiteURL, candidateSiteURL, adminKey string) error {
	if currentSiteURL != "" && currentSiteURL != candidateSiteURL && strings.TrimSpace(adminKey) == "" {
		return errors.New("修改或测试新地址时必须重新填写 Admin Key")
	}
	return nil
}
