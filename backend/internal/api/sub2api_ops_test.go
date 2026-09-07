package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestSub2APIOpsRequiresUpstreamHubAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api")
	registerSub2APIOps(group, &Deps{})

	request := httptest.NewRequest(http.MethodGet, "/api/sub2api-ops/config", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestSafeAccountProjectionDropsManagementFields(t *testing.T) {
	payload := []byte(`{
		"code": 0,
		"data": {
			"items": [{
				"id": 7,
				"name": "account-a",
				"platform": "openai",
				"type": "apikey",
				"status": "active",
				"schedulable": true,
				"error_message": "canary-error-body",
				"temp_unschedulable_reason": "canary-temp-reason",
				"credentials": {"header_overrides": {"x-relay-key": "canary-secret"}},
				"extra": {"private_probe": "canary-extra"},
				"proxy": {"host": "10.0.0.8", "username": "canary-user"},
				"groups": [{"id": 2, "name": "primary", "platform": "openai", "secret": "group-secret"}]
			}],
			"total": 1,
			"page": 1,
			"page_size": 20,
			"pages": 1
		}
	}`)

	var page sub2APISafeAccountPage
	if err := decodeSub2APIData(payload, &page); err != nil {
		t.Fatal(err)
	}
	projected, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	text := string(projected)
	for _, forbidden := range []string{"credentials", "canary-secret", "canary-extra", "canary-error-body", "canary-temp-reason", "10.0.0.8", "canary-user", "group-secret"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("projected response contains %q: %s", forbidden, text)
		}
	}
	if !strings.Contains(text, `"name":"account-a"`) || !strings.Contains(text, `"name":"primary"`) {
		t.Fatalf("projection removed required fields: %s", text)
	}
}

func TestSafeSnapshotProjectionDropsOperationalDiagnostics(t *testing.T) {
	payload := []byte(`{
		"code": 0,
		"data": {
			"generated_at": "2026-07-22T10:00:00Z",
			"overview": {
				"request_count_total": 42,
				"sla": 0.99,
				"qps": {"current": 1.5, "peak": 2, "avg": 1},
				"tps": {"current": 10, "peak": 20, "avg": 8},
				"duration": {"p95_ms": 800},
				"ttft": {"p95_ms": 300},
				"system_metrics": {"private": "canary-system"},
				"job_heartbeats": [{"last_error": "canary-job-error"}]
			},
			"throughput_trend": {"bucket": "5m", "points": [{"bucket_start": "2026-07-22T10:00:00Z", "request_count": 42}]},
			"error_trend": {"bucket": "5m", "points": []},
			"future_private_field": "canary-future"
		}
	}`)

	var snapshot sub2APISafeOpsSnapshot
	if err := decodeSub2APIData(payload, &snapshot); err != nil {
		t.Fatal(err)
	}
	projected, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	text := string(projected)
	for _, forbidden := range []string{"canary-system", "canary-job-error", "canary-future", "system_metrics", "job_heartbeats"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("projected response contains %q: %s", forbidden, text)
		}
	}
	if !strings.Contains(text, `"request_count_total":42`) || !strings.Contains(text, `"p95_ms":300`) {
		t.Fatalf("projection removed required fields: %s", text)
	}
}

func TestValidateAdminKeyBinding(t *testing.T) {
	if err := validateAdminKeyBinding("https://old.example", "https://new.example", ""); err == nil {
		t.Fatal("changing site without a new key unexpectedly succeeded")
	}
	if err := validateAdminKeyBinding("https://old.example", "https://new.example", "admin-new"); err != nil {
		t.Fatalf("changing site with a new key failed: %v", err)
	}
	if err := validateAdminKeyBinding("https://same.example", "https://same.example", ""); err != nil {
		t.Fatalf("keeping the same site failed: %v", err)
	}
}

func TestValidateDashboardLayout(t *testing.T) {
	valid := dashboardLayoutInput{
		Version: 1,
		Widgets: []dashboardWidgetInput{{
			ID:     "group_one",
			Type:   "group_metrics",
			Width:  6,
			Height: "standard",
			Config: json.RawMessage(`{"timeRange":"1h"}`),
		}},
	}
	if err := validateDashboardLayout(valid); err != nil {
		t.Fatalf("valid layout rejected: %v", err)
	}
	invalid := valid
	invalid.Widgets = append(invalid.Widgets, invalid.Widgets[0])
	if err := validateDashboardLayout(invalid); err == nil {
		t.Fatal("duplicate widget ID unexpectedly accepted")
	}
}

func TestSub2APIOpsRejectsNonPositiveIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	deps := &Deps{}
	router.GET("/snapshot", func(c *gin.Context) { getSub2APIOpsSnapshot(c, deps) })
	router.GET("/accounts", func(c *gin.Context) { getSub2APIOpsAccounts(c, deps) })
	router.POST("/accounts/today-stats", func(c *gin.Context) { getSub2APIOpsTodayStats(c, deps) })
	router.GET("/accounts/usage-details", func(c *gin.Context) { getSub2APIOpsAccountUsageDetails(c, deps) })

	tests := []struct {
		name   string
		method string
		target string
		body   string
	}{
		{name: "snapshot group", method: http.MethodGet, target: "/snapshot?group_id=-1"},
		{name: "snapshot groups", method: http.MethodGet, target: "/snapshot?group_ids=1,-2"},
		{name: "snapshot duplicate groups", method: http.MethodGet, target: "/snapshot?group_ids=1,1"},
		{name: "snapshot mixed group params", method: http.MethodGet, target: "/snapshot?group_id=1&group_ids=2"},
		{name: "account group", method: http.MethodGet, target: "/accounts?group=-1"},
		{name: "account duplicate statuses", method: http.MethodGet, target: "/accounts?statuses=active,active"},
		{name: "account mixed status modes", method: http.MethodGet, target: "/accounts?statuses=active,error&exclude_statuses=inactive"},
		{name: "account duplicate canonical groups", method: http.MethodGet, target: "/accounts?groups=1,01"},
		{name: "today account", method: http.MethodPost, target: "/accounts/today-stats", body: `{"account_ids":[-1]}`},
		{name: "usage account", method: http.MethodGet, target: "/accounts/usage-details?account_ids=0"},
		{name: "usage duplicate accounts", method: http.MethodGet, target: "/accounts/usage-details?account_ids=1,1"},
		{name: "usage window outside accounts", method: http.MethodGet, target: "/accounts/usage-details?account_ids=1&window_account_ids=2"},
		{name: "usage invalid total", method: http.MethodGet, target: "/accounts/usage-details?account_ids=1&include_total=2"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.target, strings.NewReader(test.body))
			if test.body != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestParsePositiveInt64List(t *testing.T) {
	ids, err := parsePositiveInt64List("7, 9,11", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 3 || ids[0] != 7 || ids[1] != 9 || ids[2] != 11 {
		t.Fatalf("ids = %#v", ids)
	}
	for _, input := range []string{"0", "1,-2", "1,1", "1,2,3,4", "1,"} {
		if _, err := parsePositiveInt64List(input, 3); err == nil {
			t.Errorf("input %q unexpectedly succeeded", input)
		}
	}
}

func TestSelectSub2APIAccountFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(
		http.MethodGet,
		"/accounts?statuses=active,error&exclude_statuses=inactive&groups=2,ungrouped",
		nil,
	)

	statuses, err := selectedSub2APIAccountStatuses(context, "statuses", "status")
	if err != nil || len(statuses) != 2 || statuses[0] != "active" || statuses[1] != "error" {
		t.Fatalf("statuses = %#v, err = %v", statuses, err)
	}
	excluded, err := selectedSub2APIAccountStatuses(context, "exclude_statuses", "")
	if err != nil || len(excluded) != 1 || excluded[0] != "inactive" {
		t.Fatalf("excluded statuses = %#v, err = %v", excluded, err)
	}
	groups, err := selectedSub2APIAccountGroups(context)
	if err != nil || len(groups) != 2 || groups[0] != "2" || groups[1] != "ungrouped" {
		t.Fatalf("groups = %#v, err = %v", groups, err)
	}
}

func TestSelectSub2APIAccountGroupsCanonicalizesIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodGet, "/accounts?groups=01,%2B2,ungrouped", nil)

	groups, err := selectedSub2APIAccountGroups(context)
	if err != nil || len(groups) != 3 || groups[0] != "1" || groups[1] != "2" || groups[2] != "ungrouped" {
		t.Fatalf("groups = %#v, err = %v", groups, err)
	}

	context, _ = gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodGet, "/accounts?groups=1,01", nil)
	if _, err := selectedSub2APIAccountGroups(context); err == nil {
		t.Fatal("canonical duplicate group IDs unexpectedly succeeded")
	}
}

func TestFilterSub2APIOpsAccountsSupportsStatusAndGroupOR(t *testing.T) {
	now := time.Date(2026, 7, 24, 10, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	accounts := []sub2APISafeAccount{
		{ID: 1, Status: "active", Schedulable: true, GroupIDs: []int64{10}},
		{ID: 2, Status: "active", Schedulable: false, GroupIDs: []int64{20}},
		{ID: 3, Status: "error", Schedulable: true},
		{ID: 4, Status: "active", Schedulable: true, RateLimitResetAt: &future, GroupIDs: []int64{10}},
		{ID: 5, Status: "active", Schedulable: true, TempUnschedulableUntil: &future},
	}

	filtered := filterSub2APIOpsAccounts(
		accounts,
		[]string{"active", "error"},
		nil,
		[]string{"10", "ungrouped"},
		now,
	)
	if ids := sub2APIOpsAccountIDs(filtered); len(ids) != 2 || ids[0] != 1 || ids[1] != 3 {
		t.Fatalf("included account IDs = %#v", ids)
	}

	filtered = filterSub2APIOpsAccounts(
		accounts,
		nil,
		[]string{"rate_limited", "temp_unschedulable"},
		[]string{"20", "ungrouped"},
		now,
	)
	if ids := sub2APIOpsAccountIDs(filtered); len(ids) != 2 || ids[0] != 2 || ids[1] != 3 {
		t.Fatalf("excluded account IDs = %#v", ids)
	}
}

func TestSortSub2APIOpsAccountsKeepsMissingDatesLast(t *testing.T) {
	earlier := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	later := earlier.Add(time.Hour)
	for _, order := range []string{"asc", "desc"} {
		accounts := []sub2APISafeAccount{
			{ID: 1, LastUsedAt: nil},
			{ID: 2, LastUsedAt: &later},
			{ID: 3, LastUsedAt: &earlier},
		}
		sortSub2APIOpsAccounts(accounts, "last_used_at", order)
		ids := sub2APIOpsAccountIDs(accounts)
		expected := []int64{3, 2, 1}
		if order == "desc" {
			expected = []int64{2, 3, 1}
		}
		for index := range expected {
			if ids[index] != expected[index] {
				t.Fatalf("order %s: IDs = %#v, want %#v", order, ids, expected)
			}
		}
	}
}

func TestSortSub2APIOpsAccountsKeepsMissingExpiryLast(t *testing.T) {
	earlier, later := int64(1_800_000_000), int64(1_900_000_000)
	for _, order := range []string{"asc", "desc"} {
		accounts := []sub2APISafeAccount{
			{ID: 1, ExpiresAt: nil},
			{ID: 2, ExpiresAt: &later},
			{ID: 3, ExpiresAt: &earlier},
		}
		sortSub2APIOpsAccounts(accounts, "expires_at", order)
		ids := sub2APIOpsAccountIDs(accounts)
		expected := []int64{3, 2, 1}
		if order == "desc" {
			expected = []int64{2, 3, 1}
		}
		for index := range expected {
			if ids[index] != expected[index] {
				t.Fatalf("order %s: IDs = %#v, want %#v", order, ids, expected)
			}
		}
	}
}

func TestSortSub2APIOpsAccountsByPriority(t *testing.T) {
	accounts := []sub2APISafeAccount{
		{ID: 1, Priority: 10},
		{ID: 2, Priority: 100},
		{ID: 3, Priority: 50},
	}
	sortSub2APIOpsAccounts(accounts, "priority", "desc")
	if ids := sub2APIOpsAccountIDs(accounts); ids[0] != 2 || ids[1] != 3 || ids[2] != 1 {
		t.Fatalf("priority sort IDs = %#v", ids)
	}
}

func TestSafeAccountStatsProjectionDropsUnknownFields(t *testing.T) {
	payload := []byte(`{
		"code": 0,
		"data": {
			"stats": {
				"7": {
					"requests": 12,
					"tokens": 345,
					"cost": 1.25,
					"standard_cost": 1.5,
					"user_cost": 2.75,
					"credentials": "canary-secret",
					"debug": {"admin_key": "canary-admin-key"}
				}
			}
		}
	}`)

	var stats sub2APISafeTodayStatsResponse
	if err := decodeSub2APIData(payload, &stats); err != nil {
		t.Fatal(err)
	}
	projected, err := json.Marshal(stats)
	if err != nil {
		t.Fatal(err)
	}
	text := string(projected)
	for _, forbidden := range []string{"credentials", "canary-secret", "debug", "canary-admin-key"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("projected stats contain %q: %s", forbidden, text)
		}
	}
	if !strings.Contains(text, `"requests":12`) || !strings.Contains(text, `"user_cost":2.75`) {
		t.Fatalf("projection removed required fields: %s", text)
	}
}

func TestProjectSub2APIAccountWindowStatsIncludesGeminiAndGrok(t *testing.T) {
	payload := []byte(`{
		"code": 0,
		"data": {
			"gemini_shared_daily": {
				"window_stats": {
					"requests": 8,
					"tokens": 900,
					"cost": 1.2,
					"standard_cost": 1.5,
					"user_cost": 2.4,
					"credentials": "canary-secret"
				}
			},
			"grok_local_usage_24h": {
				"requests": 3,
				"tokens": 400,
				"cost": 0.5,
				"standard_cost": 0.6,
				"user_cost": 0.8,
				"admin_key": "canary-admin-key"
			}
		}
	}`)

	var usage sub2APIUpstreamAccountUsage
	if err := decodeSub2APIData(payload, &usage); err != nil {
		t.Fatal(err)
	}
	projected := projectSub2APIAccountWindowStats(usage)
	if projected["gemini_shared_daily"].UserCost != 2.4 || projected["24h"].Requests != 3 {
		t.Fatalf("unexpected projected windows: %#v", projected)
	}
	encoded, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, forbidden := range []string{"credentials", "canary-secret", "admin_key", "canary-admin-key"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("projected window stats contain %q: %s", forbidden, text)
		}
	}
}

func sub2APIOpsAccountIDs(accounts []sub2APISafeAccount) []int64 {
	ids := make([]int64, len(accounts))
	for index := range accounts {
		ids[index] = accounts[index].ID
	}
	return ids
}

func TestAggregateSub2APIOpsSnapshots(t *testing.T) {
	ttftA, ttftB := 320.0, 780.0
	snapshots := []sub2APISafeOpsSnapshot{
		{
			GeneratedAt: "2026-07-23T01:00:00Z",
			Overview: &sub2APISafeOpsOverview{
				StartTime:          "2026-07-23T00:00:00Z",
				EndTime:            "2026-07-23T01:00:00Z",
				Platform:           "openai",
				HealthScore:        98,
				SuccessCount:       9,
				ErrorCountTotal:    2,
				ErrorCountSLA:      1,
				RequestCountTotal:  11,
				RequestCountSLA:    10,
				TokenConsumed:      100,
				UpstreamErrorCount: 1,
				QPS:                sub2APIOpsRateSummary{Current: 1, Peak: 2, Avg: 0.5},
				TTFT:               sub2APIOpsPercentiles{P95MS: &ttftA},
			},
			ThroughputTrend: &sub2APISafeThroughputTrend{
				Bucket: "5m",
				Points: []sub2APISafeThroughputPoint{{BucketStart: "2026-07-23T00:55:00Z", RequestCount: 4, TokenConsumed: 40}},
			},
		},
		{
			GeneratedAt: "2026-07-23T01:00:01Z",
			Overview: &sub2APISafeOpsOverview{
				StartTime:          "2026-07-23T00:00:00Z",
				EndTime:            "2026-07-23T01:00:00Z",
				Platform:           "openai",
				HealthScore:        91,
				SuccessCount:       18,
				ErrorCountTotal:    3,
				ErrorCountSLA:      2,
				RequestCountTotal:  21,
				RequestCountSLA:    20,
				TokenConsumed:      250,
				UpstreamErrorCount: 2,
				QPS:                sub2APIOpsRateSummary{Current: 3, Peak: 5, Avg: 2},
				TTFT:               sub2APIOpsPercentiles{P95MS: &ttftB},
			},
			ThroughputTrend: &sub2APISafeThroughputTrend{
				Bucket: "5m",
				Points: []sub2APISafeThroughputPoint{{BucketStart: "2026-07-23T00:55:00Z", RequestCount: 8, TokenConsumed: 90}},
			},
		},
	}

	result := aggregateSub2APIOpsSnapshots([]int64{41, 42}, snapshots)
	if result.Overview == nil {
		t.Fatal("overview is nil")
	}
	if result.Overview.RequestCountTotal != 32 || result.Overview.SuccessCount != 27 || result.Overview.TokenConsumed != 350 {
		t.Fatalf("unexpected totals: %#v", result.Overview)
	}
	if result.Overview.SLA != 0.9 || result.Overview.ErrorRate != 0.1 || result.Overview.UpstreamErrorRate != 0.1 {
		t.Fatalf("unexpected rates: %#v", result.Overview)
	}
	if result.Overview.HealthScore != 91 || result.Overview.TTFT.P95MS == nil || *result.Overview.TTFT.P95MS != 780 {
		t.Fatalf("unexpected conservative metrics: %#v", result.Overview)
	}
	if result.Aggregation == nil || result.Aggregation.PercentileMode != "worst_group" || len(result.Aggregation.GroupIDs) != 2 {
		t.Fatalf("unexpected aggregation metadata: %#v", result.Aggregation)
	}
	if result.ThroughputTrend == nil || len(result.ThroughputTrend.Points) != 1 || result.ThroughputTrend.Points[0].RequestCount != 12 {
		t.Fatalf("unexpected throughput trend: %#v", result.ThroughputTrend)
	}
}

func TestMergeAndSortSub2APIOpsRecentAccounts(t *testing.T) {
	first := time.Date(2026, 7, 23, 1, 0, 0, 0, time.UTC)
	second := first.Add(2 * time.Minute)
	accounts := map[int64]*sub2APIOpsRecentAccount{}
	mergeSub2APIOpsRecentAccount(accounts, sub2APIOpsRecentAccount{
		ID: 101, Platform: "openai", LastUsedAt: first, GroupIDs: []int64{41},
	})
	mergeSub2APIOpsRecentAccount(accounts, sub2APIOpsRecentAccount{
		ID: 101, Platform: "openai", LastUsedAt: second, GroupIDs: []int64{42},
	})
	mergeSub2APIOpsRecentAccount(accounts, sub2APIOpsRecentAccount{
		ID: 102, Platform: "openai", LastUsedAt: first,
	})

	result := sortedSub2APIOpsRecentAccounts(accounts, 20)
	if len(result) != 2 || result[0].ID != 101 || !result[0].LastUsedAt.Equal(second) {
		t.Fatalf("unexpected recent account order: %#v", result)
	}
	if len(result[0].GroupIDs) != 2 || result[0].GroupIDs[0] != 41 || result[0].GroupIDs[1] != 42 {
		t.Fatalf("expected deduplicated group IDs: %#v", result[0].GroupIDs)
	}

	topOne := sortedSub2APIOpsRecentAccounts(accounts, 1)
	if len(topOne) != 1 || topOne[0].ID != 101 {
		t.Fatalf("top limit was not applied: %#v", topOne)
	}
}

func TestSafeAccountUsageWindowsProjection(t *testing.T) {
	future := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	payload := []byte(`{
		"id": 7,
		"name": "account-a",
		"platform": "openai",
		"type": "oauth",
		"session_window_end": "2026-07-23T03:00:00Z",
		"extra": {
			"codex_5h_used_percent": 42.5,
			"codex_5h_reset_at": "` + future + `",
			"codex_5h_window_minutes": 300,
			"codex_7d_used_percent": 12,
			"codex_7d_window_minutes": 43200,
			"credentials": "must-drop",
			"private_key": "must-drop"
		}
	}`)
	var account sub2APISafeAccount
	if err := json.Unmarshal(payload, &account); err != nil {
		t.Fatal(err)
	}
	if account.UsageWindows == nil || len(account.UsageWindows.Items) != 2 {
		t.Fatalf("usage windows missing: %#v", account.UsageWindows)
	}
	if account.UsageWindows.Items[0].Label != "5h" || account.UsageWindows.Items[0].Utilization != 42.5 {
		t.Fatalf("unexpected short usage window: %#v", account.UsageWindows.Items[0])
	}
	if account.UsageWindows.Items[1].Label != "30d" || account.UsageWindows.Items[1].Utilization != 12 {
		t.Fatalf("unexpected usage windows: %#v", account.UsageWindows)
	}
	projected, err := json.Marshal(account)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(projected), "must-drop") || strings.Contains(string(projected), "credentials") {
		t.Fatalf("safe account projection leaked raw extra: %s", projected)
	}
}

func TestSafeAccountUsageWindowsSessionFallback(t *testing.T) {
	end := time.Now().Add(time.Hour)
	account := sub2APISafeAccount{}
	account.SessionWindowEnd = &end
	account.UsageWindows = buildSub2APISafeAccountUsageWindows(
		"anthropic",
		&sub2APISafeAccountExtra{SessionWindowUtilization: float64Ptr(0.25)},
		account.SessionWindowEnd,
		"",
		time.Now(),
	)
	if account.UsageWindows == nil || len(account.UsageWindows.Items) != 1 {
		t.Fatal("session fallback did not produce a 5h window")
	}
	if account.UsageWindows.Items[0].Label != "5h" || account.UsageWindows.Items[0].Utilization != 25 {
		t.Fatalf("unexpected session utilization: %#v", account.UsageWindows.Items[0])
	}
}

func TestSafeAccountUsageWindowsGrokMonthlyFallback(t *testing.T) {
	start := time.Now().Add(-29 * 24 * time.Hour).UTC().Format(time.RFC3339)
	future := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	payload := []byte(`{
		"id": 8,
		"platform": "grok",
		"extra": {
			"grok_billing_snapshot": {
				"period_type": "monthly",
				"used_percent": 67.5,
				"billing_period_start": "` + start + `",
				"billing_period_end": "` + future + `",
				"private_token": "must-drop"
			}
		}
	}`)

	var account sub2APISafeAccount
	if err := json.Unmarshal(payload, &account); err != nil {
		t.Fatal(err)
	}
	if account.UsageWindows == nil || len(account.UsageWindows.Items) != 1 {
		t.Fatalf("monthly fallback missing: %#v", account.UsageWindows)
	}
	window := account.UsageWindows.Items[0]
	if window.Label != "monthly" || window.Utilization != 67.5 {
		t.Fatalf("unexpected monthly fallback: %#v", window)
	}
	projected, err := json.Marshal(account)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(projected), "private_token") || strings.Contains(string(projected), "must-drop") {
		t.Fatalf("safe Grok projection leaked private billing field: %s", projected)
	}
}

func float64Ptr(value float64) *float64 { return &value }
