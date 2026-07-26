package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSmartPostDraftCallsDeepSeekJSONMode(t *testing.T) {
	var captured deepSeekChatRequest
	deepSeek := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected deepseek path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected authorization header: %s", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode deepseek request failed: %v", err)
		}

		draft := smartPostDraftResp{
			Fields: smartPostDraftFields{
				Title:         "明晚羽毛球局",
				Description:   "明天晚上在北邮体育馆打羽毛球，人数上限 4 人。欢迎一起参加，建议提前到场热身。",
				Category:      "运动",
				SubCategory:   "羽毛球",
				TimeMode:      "fixed",
				TimeRange:     7,
				SelectedDate:  "2099-05-23",
				SelectedClock: "19:30",
				LocationMode:  "manual",
				LocationText:  "北邮体育馆",
				MaxCount:      4,
			},
			Summary: []string{"识别为：运动 / 羽毛球", "时间：2099-05-23 19:30"},
		}
		content, err := json.Marshal(draft)
		if err != nil {
			t.Fatalf("marshal draft failed: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"role": "assistant", "content": string(content)}},
			},
		})
	}))
	defer deepSeek.Close()

	t.Setenv("DEEPSEEK_API_KEY", "test-key")
	t.Setenv("DEEPSEEK_BASE_URL", deepSeek.URL)
	t.Setenv("DEEPSEEK_MODEL", "deepseek-test")

	db := openRouterTestDB(t)
	router := NewRouter(db)
	body := []byte(`{"input":"明晚北邮体育馆羽毛球 4 人","history":{"initiatedPosts":[],"joinedPosts":[]}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/posts/smart-draft", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearerFor(t, db, "user_smart_draft"))
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("smart draft failed, code=%d body=%s", resp.Code, resp.Body.String())
	}
	if captured.Model != "deepseek-test" {
		t.Fatalf("unexpected model: %s", captured.Model)
	}
	if captured.ResponseFormat["type"] != "json_object" {
		t.Fatalf("expected json_object response_format, got=%+v", captured.ResponseFormat)
	}
	if len(captured.Messages) < 2 || !strings.Contains(captured.Messages[0].Content, smartPostDraftJSONContract) {
		t.Fatalf("system prompt should include JSON contract, got=%+v", captured.Messages)
	}

	var payload smartPostDraftResp
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode smart draft response failed: %v", err)
	}
	if !payload.OK {
		t.Fatalf("expected ok response, got=%+v", payload)
	}
	if payload.Fields.FixedTime == "" || payload.Fields.FixedTimeDisplay != "2099-05-23 19:30" {
		t.Fatalf("expected normalized fixed time, got=%+v", payload.Fields)
	}
	if payload.Fields.Category != "运动" || payload.Fields.SubCategory != "羽毛球" {
		t.Fatalf("unexpected activity fields: %+v", payload.Fields)
	}
}

func TestSmartPostDraftRequiresDeepSeekKey(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "")

	db := openRouterTestDB(t)
	router := NewRouter(db)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/posts/smart-draft", strings.NewReader(`{"input":"明天羽毛球"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearerFor(t, db, "user_smart_draft"))
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when key missing, got=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestSmartPostDraftFillsMissingFieldsFromHistory(t *testing.T) {
	deepSeek := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		draft := smartPostAIOutput{
			Intent: smartPostIntent{
				ActivityText: "羽毛球",
				Category:     "运动",
				SubCategory:  "羽毛球",
			},
			Form: smartPostDraftFields{
				Title:       "羽毛球局",
				Description: "一起打羽毛球，时间待定，地点待定，人数 2-6 人。",
				Category:    "运动",
				SubCategory: "羽毛球",
				TimeMode:    "range",
				TimeRange:   7,
				MaxCount:    6,
			},
			Summary: []string{"识别到活动：羽毛球", "时间未指定，默认使用7天范围", "地点未指定，留空"},
		}
		content, err := json.Marshal(draft)
		if err != nil {
			t.Fatalf("marshal draft failed: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"role": "assistant", "content": string(content)}},
			},
		})
	}))
	defer deepSeek.Close()

	t.Setenv("DEEPSEEK_API_KEY", "test-key")
	t.Setenv("DEEPSEEK_BASE_URL", deepSeek.URL)

	db := openRouterTestDB(t)
	router := NewRouter(db)
	body := []byte(`{
		"input":"羽毛球",
		"history":{
			"initiatedPosts":[{
				"title":"上周羽毛球",
				"description":"明晚，晚8-10本部体育馆羽毛球dd，4-5级男双，微对抗。",
				"category":"运动",
				"subCategory":"羽毛球",
				"timeInfo":{"mode":"fixed","fixedTime":"2026-05-23T20:00:00+08:00"},
				"address":"北邮体育馆",
				"coords":{"latitude":40.1574,"longitude":116.2878},
				"maxCount":4,
				"createdAt":1760000000000
			}],
			"joinedPosts":[]
		}
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/posts/smart-draft", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearerFor(t, db, "user_smart_draft"))
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("smart draft failed, code=%d body=%s", resp.Code, resp.Body.String())
	}
	var payload smartPostDraftResp
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode smart draft response failed: %v", err)
	}
	if payload.Fields.LocationText != "北邮体育馆" {
		t.Fatalf("expected history location, got=%+v", payload.Fields)
	}
	if payload.Fields.TimeMode != "fixed" || payload.Fields.FixedTime == "" || payload.Fields.SelectedClock != "20:00" {
		t.Fatalf("expected history fixed time, got=%+v", payload.Fields)
	}
	if payload.Fields.MaxCount != 4 {
		t.Fatalf("expected history maxCount, got=%+v", payload.Fields)
	}
	if !strings.Contains(payload.Fields.Title, "北邮羽毛球局") {
		t.Fatalf("expected compact title, got=%s", payload.Fields.Title)
	}
	if !strings.Contains(payload.Fields.Description, "男双") || !strings.Contains(payload.Fields.Description, "微对抗") {
		t.Fatalf("expected rich history description, got=%s", payload.Fields.Description)
	}
	joinedSummary := strings.Join(payload.Summary, " ")
	if !strings.Contains(joinedSummary, "历史") {
		t.Fatalf("summary should mention history fallback, got=%+v", payload.Summary)
	}
}

func TestSmartPostDraftDoesNotBorrowDetailsAcrossSubCategory(t *testing.T) {
	now := time.Date(2026, 5, 23, 0, 7, 0, 0, smartPostTimeLocation())
	req := smartPostDraftReq{
		Input: "北邮游泳",
		History: smartPostHistoryReq{
			InitiatedPosts: []smartPostHistoryPostReq{
				{
					Title:       "北部男双微对抗",
					Description: "明晚，晚8-10本部体育馆羽毛球dd，4-5级男双，微对抗。",
					Category:    "运动",
					SubCategory: "羽毛球",
					TimeInfo:    timeInfoReq{Mode: "fixed", FixedTime: "2026-05-23T20:00:00+08:00"},
					Address:     "北京邮电大学(海淀校区)",
					MaxCount:    6,
					CreatedAt:   1760000000000,
				},
			},
		},
	}
	filteredHistory := filterSmartPostHistory(req.History, "运动", "游泳")
	if len(filteredHistory.InitiatedPosts) != 0 || len(filteredHistory.JoinedPosts) != 0 {
		t.Fatalf("badminton history should not be sent as swimming history, got=%+v", filteredHistory)
	}
	draft := smartPostAIOutput{
		Intent: smartPostIntent{
			ActivityText: "游泳",
			Category:     "运动",
			SubCategory:  "游泳",
		},
		Form: smartPostDraftFields{
			Title:        "今晚北邮游泳局",
			Description:  "2026-05-23 20:00 在北京邮电大学(海淀校区)一起游泳，人数上限 6 人。参考历史安排：北部男双微对抗，4-5级男双。",
			Category:     "运动",
			SubCategory:  "游泳",
			TimeMode:     "range",
			TimeRange:    7,
			LocationMode: "manual",
			LocationText: "北京邮电大学(海淀校区)",
			MaxCount:     6,
		},
	}

	payload, err := normalizeSmartPostDraft(draft, req, now)
	if err != nil {
		t.Fatalf("normalize smart draft failed: %v", err)
	}
	if strings.Contains(payload.Fields.Description, "男双") || strings.Contains(payload.Fields.Description, "微对抗") || strings.Contains(payload.Fields.Description, "羽毛球") {
		t.Fatalf("swimming description should not borrow badminton details, got=%s", payload.Fields.Description)
	}
	if payload.Fields.TimeMode != "range" {
		t.Fatalf("badminton history should not drive swimming time, got=%+v", payload.Fields)
	}
}
