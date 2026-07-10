package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	defaultDeepSeekBaseURL = "https://api.deepseek.com"
	defaultDeepSeekModel   = "deepseek-chat"
	smartPostPromptMaxJSON = 6000
)

var smartPostSubCategories = map[string]map[string]struct{}{
	"运动": {
		"羽毛球": {}, "足球": {}, "篮球": {}, "跑步": {}, "骑行": {}, "游泳": {}, "其他运动": {},
	},
	"娱乐": {
		"桌游": {}, "电影": {}, "KTV": {}, "游戏": {}, "其他娱乐": {},
	},
	"学习": {
		"英语": {}, "考研": {}, "编程": {}, "读书": {}, "其他学习": {},
	},
}

var smartPostActivityRules = []struct {
	Category    string
	SubCategory string
	Keywords    []string
}{
	{Category: "运动", SubCategory: "羽毛球", Keywords: []string{"羽毛球", "羽球"}},
	{Category: "运动", SubCategory: "篮球", Keywords: []string{"篮球"}},
	{Category: "运动", SubCategory: "足球", Keywords: []string{"足球"}},
	{Category: "运动", SubCategory: "跑步", Keywords: []string{"跑步", "夜跑", "晨跑"}},
	{Category: "运动", SubCategory: "骑行", Keywords: []string{"骑行", "单车"}},
	{Category: "运动", SubCategory: "游泳", Keywords: []string{"游泳"}},
	{Category: "娱乐", SubCategory: "桌游", Keywords: []string{"桌游", "剧本杀", "狼人杀", "棋牌"}},
	{Category: "娱乐", SubCategory: "电影", Keywords: []string{"电影", "观影", "看电影"}},
	{Category: "娱乐", SubCategory: "KTV", Keywords: []string{"KTV", "唱歌"}},
	{Category: "娱乐", SubCategory: "游戏", Keywords: []string{"游戏", "开黑"}},
	{Category: "学习", SubCategory: "英语", Keywords: []string{"英语", "口语"}},
	{Category: "学习", SubCategory: "考研", Keywords: []string{"考研"}},
	{Category: "学习", SubCategory: "编程", Keywords: []string{"编程", "代码", "刷题"}},
	{Category: "学习", SubCategory: "读书", Keywords: []string{"读书", "自习", "学习"}},
}

type smartPostDraftReq struct {
	Input           string                 `json:"input"`
	CurrentLocation *smartPostLocationReq  `json:"currentLocation"`
	History         smartPostHistoryReq    `json:"history"`
	Context         map[string]interface{} `json:"context"`
}

type smartPostLocationReq struct {
	Address   string  `json:"address"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type smartPostHistoryReq struct {
	InitiatedPosts []smartPostHistoryPostReq `json:"initiatedPosts"`
	JoinedPosts    []smartPostHistoryPostReq `json:"joinedPosts"`
}

type smartPostHistoryPostReq struct {
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Category    string      `json:"category"`
	SubCategory string      `json:"subCategory"`
	TimeInfo    timeInfoReq `json:"timeInfo"`
	Address     string      `json:"address"`
	Coords      *coordsReq  `json:"coords"`
	MaxCount    int         `json:"maxCount"`
	CreatedAt   int64       `json:"createdAt"`
}

type smartPostDraftResp struct {
	OK      bool                 `json:"ok"`
	Fields  smartPostDraftFields `json:"fields,omitempty"`
	Summary []string             `json:"summary,omitempty"`
}

type smartPostAIOutput struct {
	Intent         smartPostIntent                   `json:"intent"`
	Form           smartPostDraftFields              `json:"form"`
	Fields         smartPostDraftFields              `json:"fields"`
	FieldDecisions map[string]smartPostFieldDecision `json:"fieldDecisions"`
	MissingFields  []string                          `json:"missingFields"`
	Summary        []string                          `json:"summary"`
}

type smartPostIntent struct {
	ActivityText     string   `json:"activityText"`
	Category         string   `json:"category"`
	SubCategory      string   `json:"subCategory"`
	ExplicitTime     bool     `json:"explicitTime"`
	ExplicitLocation bool     `json:"explicitLocation"`
	ExplicitMaxCount bool     `json:"explicitMaxCount"`
	NeedsUserConfirm bool     `json:"needsUserConfirm"`
	Ambiguities      []string `json:"ambiguities"`
}

type smartPostFieldDecision struct {
	Value       interface{} `json:"value"`
	Source      string      `json:"source"`
	Confidence  float64     `json:"confidence"`
	Reason      string      `json:"reason"`
	Alternative []string    `json:"alternative"`
}

type smartPostDraftFields struct {
	Title            string     `json:"title"`
	Description      string     `json:"description"`
	Category         string     `json:"category"`
	SubCategory      string     `json:"subCategory"`
	TimeMode         string     `json:"timeMode"`
	TimeRange        int        `json:"timeRange"`
	FixedTime        string     `json:"fixedTime"`
	FixedTimeDisplay string     `json:"fixedTimeDisplay"`
	SelectedDate     string     `json:"selectedDate"`
	SelectedClock    string     `json:"selectedClock"`
	LocationMode     string     `json:"locationMode"`
	LocationText     string     `json:"locationText"`
	LocationCoords   *coordsReq `json:"locationCoords"`
	MaxCount         int        `json:"maxCount"`
}

type deepSeekChatRequest struct {
	Model          string            `json:"model"`
	Messages       []deepSeekMessage `json:"messages"`
	ResponseFormat map[string]string `json:"response_format"`
	Temperature    float64           `json:"temperature"`
	MaxTokens      int               `json:"max_tokens"`
}

type deepSeekMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type deepSeekChatResponse struct {
	Choices []struct {
		Message deepSeekMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

const smartPostDraftJSONContract = `{
  "intent": {
    "activityText": "string, 从用户输入中抽取的活动关键词",
    "category": "运动 | 娱乐 | 学习 | 其他",
    "subCategory": "按分类枚举",
    "explicitTime": "boolean, 用户是否明确说了日期、星期、最近几天或几点",
    "explicitLocation": "boolean, 用户是否明确说了地点",
    "explicitMaxCount": "boolean, 用户是否明确说了人数",
    "needsUserConfirm": "boolean",
    "ambiguities": ["string"]
  },
  "form": {
    "title": "string, 4-32 个中文字符，适合活动标题",
    "description": "string, 40-180 个中文字符，必须包含时间/地点/人数信息；要参考历史同类活动描述里的玩法、水平、对抗强度、装备提醒等具体信息；不要包含联系方式、费用承诺或夸张营销",
    "category": "运动 | 娱乐 | 学习 | 其他",
    "subCategory": "运动只能是 羽毛球/足球/篮球/跑步/骑行/游泳/其他运动；娱乐只能是 桌游/电影/KTV/游戏/其他娱乐；学习只能是 英语/考研/编程/读书/其他学习；其他分类时给 2-8 字具体类型",
    "timeMode": "fixed | range",
    "timeRange": "integer, timeMode=range 时 1-30；timeMode=fixed 时填 7",
    "selectedDate": "string, timeMode=fixed 时 YYYY-MM-DD，否则空字符串",
    "selectedClock": "string, timeMode=fixed 时 HH:mm 24小时制，否则默认 09:00",
    "fixedTime": "string, 可以留空，服务端会按 selectedDate/selectedClock 转成 ISO 时间",
    "fixedTimeDisplay": "string, 可以留空，服务端会按 selectedDate/selectedClock 转成 YYYY-MM-DD HH:mm",
    "locationMode": "manual | current",
    "locationText": "string, 用户明确提到地点优先；否则必须优先从历史同类活动中选择最高频地点；再否则用当前位置；都没有才留空",
    "locationCoords": "object|null, 仅在来自当前位置或历史坐标时填写 {\"latitude\": number, \"longitude\": number}",
    "maxCount": "integer, 2-99；用户没说人数时优先使用历史同类活动最常见人数"
  },
  "fieldDecisions": {
    "category": {"value": "string", "source": "input|history|currentLocation|default|modelInference", "confidence": 0.0, "reason": "string"},
    "time": {"value": "string", "source": "input|history|currentLocation|default|modelInference", "confidence": 0.0, "reason": "string"},
    "location": {"value": "string", "source": "input|history|currentLocation|default|modelInference", "confidence": 0.0, "reason": "string"},
    "maxCount": {"value": "number", "source": "input|history|currentLocation|default|modelInference", "confidence": 0.0, "reason": "string"},
    "description": {"value": "string", "source": "input|history|currentLocation|default|modelInference", "confidence": 0.0, "reason": "string"}
  },
  "missingFields": ["string"],
  "summary": ["2-5 条中文短句，说明识别到的活动、时间、地点、人数或使用了哪些默认值"]
}`

func (s *Server) SmartPostDraft(c *gin.Context) {
	var req smartPostDraftReq
	if !bindJSONOrBadRequest(c, &req) {
		return
	}
	req.Input = strings.TrimSpace(req.Input)
	if req.Input == "" {
		fail(c, http.StatusBadRequest, "SMART_INPUT_REQUIRED", "input required")
		return
	}
	if strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY")) == "" {
		fail(c, http.StatusServiceUnavailable, "DEEPSEEK_CONFIG_MISSING", "DEEPSEEK_API_KEY not configured")
		return
	}

	now := time.Now().In(smartPostTimeLocation())
	draft, err := s.generateDeepSeekSmartPostDraft(c.Request.Context(), req, now)
	if err != nil {
		log.Printf("deepseek smart draft failed: %v", err)
		fail(c, http.StatusBadGateway, "DEEPSEEK_DRAFT_FAILED", "smart draft failed")
		return
	}
	c.JSON(http.StatusOK, draft)
}

func (s *Server) generateDeepSeekSmartPostDraft(ctx context.Context, req smartPostDraftReq, now time.Time) (smartPostDraftResp, error) {
	apiKey := strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))
	baseURL := envOrDefault("DEEPSEEK_BASE_URL", defaultDeepSeekBaseURL)
	model := envOrDefault("DEEPSEEK_MODEL", defaultDeepSeekModel)
	endpoint, err := deepSeekChatCompletionsURL(baseURL)
	if err != nil {
		return smartPostDraftResp{}, err
	}
	promptReq := req
	if category, subCategory := inferSmartActivity(req.Input); category != "" {
		promptReq.History = filterSmartPostHistory(req.History, category, subCategory)
	}

	payload := deepSeekChatRequest{
		Model: model,
		Messages: []deepSeekMessage{
			{
				Role:    "system",
				Content: buildSmartPostSystemPrompt(now),
			},
			{
				Role:    "user",
				Content: buildSmartPostUserPrompt(promptReq),
			},
		},
		ResponseFormat: map[string]string{"type": "json_object"},
		Temperature:    0.2,
		MaxTokens:      1200,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return smartPostDraftResp{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return smartPostDraftResp{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 25 * time.Second}
	if s.HTTPClient != nil {
		copied := *s.HTTPClient
		if copied.Timeout == 0 || copied.Timeout < 20*time.Second {
			copied.Timeout = 25 * time.Second
		}
		client = &copied
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return smartPostDraftResp{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return smartPostDraftResp{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return smartPostDraftResp{}, fmt.Errorf("deepseek status=%d body=%s", resp.StatusCode, trimRunes(string(respBody), 300))
	}

	var dsResp deepSeekChatResponse
	if err := json.Unmarshal(respBody, &dsResp); err != nil {
		return smartPostDraftResp{}, err
	}
	if dsResp.Error != nil {
		return smartPostDraftResp{}, errors.New(dsResp.Error.Message)
	}
	if len(dsResp.Choices) == 0 {
		return smartPostDraftResp{}, errors.New("deepseek response has no choices")
	}
	content := strings.TrimSpace(dsResp.Choices[0].Message.Content)
	if content == "" {
		return smartPostDraftResp{}, errors.New("deepseek response content empty")
	}

	var draft smartPostAIOutput
	if err := json.Unmarshal([]byte(content), &draft); err != nil {
		return smartPostDraftResp{}, err
	}
	return normalizeSmartPostDraft(draft, req, now)
}

func buildSmartPostSystemPrompt(now time.Time) string {
	return strings.Join([]string{
		"你是“找个伴儿”小程序的活动发布助手。",
		"你必须只输出一个合法 JSON object，不要 Markdown，不要代码块，不要解释。",
		"当前时间：" + now.Format("2006-01-02 15:04:05 -07:00"),
		"所有相对日期都必须按当前时间推算，fixed 时间必须晚于当前时间；不确定具体日期时用历史同类活动的常见星期/时间，历史也没有再用 range。",
		"用户只输入活动类型时，不要偷懒留空：时间、地点、人数必须优先参考历史同类活动；历史没有才参考当前位置或默认值。",
		"历史同类活动必须是 category 和 subCategory 都相同的活动；发起过的历史比参加过的历史权重更高，最近的历史权重更高。",
		"活动描述不能只写模板话。必须阅读历史同类活动的 title/description，提取可复用的玩法细节，例如水平、单打/双打、微对抗、新手友好、自带球拍、场馆习惯称呼等；只复用与当前 subCategory 匹配的信息，不要把羽毛球的男双/球拍/微对抗写进游泳、跑步等其他活动里。",
		"输出 JSON 必须严格满足以下契约：",
		smartPostDraftJSONContract,
	}, "\n")
}

func buildSmartPostUserPrompt(req smartPostDraftReq) string {
	input := trimRunes(strings.TrimSpace(req.Input), 300)
	return strings.Join([]string{
		"用户的一句话发布需求：" + input,
		"当前位置 JSON：" + compactPromptJSON(req.CurrentLocation, 1000),
		"历史活动 JSON：" + compactPromptJSON(req.History, smartPostPromptMaxJSON),
		"生成一个可直接回填发布表单的 JSON。",
	}, "\n")
}

func normalizeSmartPostDraft(draft smartPostAIOutput, req smartPostDraftReq, now time.Time) (smartPostDraftResp, error) {
	fields := draft.Form
	if smartPostFieldsEmpty(fields) {
		fields = draft.Fields
	}
	if strings.TrimSpace(fields.Category) == "" {
		fields.Category = draft.Intent.Category
	}
	if strings.TrimSpace(fields.SubCategory) == "" {
		fields.SubCategory = draft.Intent.SubCategory
	}
	if inferredCategory, inferredSubCategory := inferSmartActivity(req.Input); inferredCategory != "" {
		if strings.TrimSpace(fields.Category) == "" || strings.TrimSpace(fields.Category) == "其他" {
			fields.Category = inferredCategory
		}
		if strings.TrimSpace(fields.SubCategory) == "" || strings.HasPrefix(strings.TrimSpace(fields.SubCategory), "其他") {
			fields.SubCategory = inferredSubCategory
		}
	}

	fields.Title = trimRunes(strings.TrimSpace(fields.Title), 32)
	fields.Description = trimRunes(strings.TrimSpace(fields.Description), 220)
	fields.Category = normalizeSmartPostCategory(fields.Category)
	fields.SubCategory = normalizeSmartPostSubCategory(fields.Category, fields.SubCategory)
	fields.MaxCount = clampInt(fields.MaxCount, 2, 99)
	fields.LocationMode = normalizeSmartLocationMode(fields.LocationMode)
	fields.LocationText = trimRunes(strings.TrimSpace(fields.LocationText), 80)
	if !validSmartCoords(fields.LocationCoords) {
		fields.LocationCoords = nil
	}
	if fields.LocationCoords == nil && fields.LocationMode == "current" {
		fields.LocationMode = "manual"
	}

	normalizeSmartPostTime(&fields, now)
	var notes []string
	var historyChanged bool
	var descriptionHint string
	fields, notes, historyChanged, descriptionHint = applySmartHistoryDefaults(req, fields, now)
	if fields.Title == "" || historyChanged || smartDescriptionConflictsWithActivity(fields.Title, fields) {
		fields.Title = buildSmartPostFallbackTitle(fields, now)
	} else {
		fields.Title = normalizeSmartPostTitle(fields.Title, fields, now)
	}
	if smartDescriptionConflictsWithActivity(fields.Description, fields) {
		fields.Description = ""
		descriptionHint = ""
	}
	if fields.Description == "" || historyChanged || (descriptionHint != "" && smartDescriptionLooksGeneric(fields.Description)) {
		fields.Description = buildSmartPostFallbackDescription(fields, descriptionHint)
	}
	if fields.Category == "" || fields.SubCategory == "" {
		return smartPostDraftResp{}, errors.New("smart draft missing activity category")
	}

	summarySource := draft.Summary
	if historyChanged {
		summarySource = nil
	}
	summary := normalizeSmartPostSummary(summarySource, fields)
	for _, note := range notes {
		if len(summary) >= 5 {
			break
		}
		summary = append(summary, note)
	}
	return smartPostDraftResp{
		OK:      true,
		Fields:  fields,
		Summary: summary,
	}, nil
}

func smartPostFieldsEmpty(fields smartPostDraftFields) bool {
	return strings.TrimSpace(fields.Title) == "" &&
		strings.TrimSpace(fields.Description) == "" &&
		strings.TrimSpace(fields.Category) == "" &&
		strings.TrimSpace(fields.SubCategory) == "" &&
		strings.TrimSpace(fields.TimeMode) == "" &&
		strings.TrimSpace(fields.LocationText) == "" &&
		fields.MaxCount == 0
}

func inferSmartActivity(input string) (string, string) {
	source := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(input), " ", ""))
	if source == "" {
		return "", ""
	}
	for _, rule := range smartPostActivityRules {
		for _, keyword := range rule.Keywords {
			if strings.Contains(source, strings.ToLower(keyword)) {
				return rule.Category, rule.SubCategory
			}
		}
	}
	return "", ""
}

type smartHistoryItem struct {
	Post   smartPostHistoryPostReq
	Role   string
	Weight float64
}

type smartHistoryLocation struct {
	Address string
	Coords  *coordsReq
}

type smartHistoryClock struct {
	Weekday int
	Hour    int
	Minute  int
}

func applySmartHistoryDefaults(req smartPostDraftReq, fields smartPostDraftFields, now time.Time) (smartPostDraftFields, []string, bool, string) {
	items := collectSmartHistoryItems(req.History, fields.Category, fields.SubCategory, now)
	notes := []string{}
	changed := false
	input := strings.TrimSpace(req.Input)
	descriptionHint := selectSmartHistoryDescriptionHint(items, fields)

	if fields.LocationText == "" {
		if location, ok := selectSmartHistoryLocation(items); ok {
			fields.LocationText = location.Address
			fields.LocationCoords = location.Coords
			fields.LocationMode = "manual"
			notes = append(notes, "地点参考了历史同类活动的常用地点。")
			changed = true
		} else if req.CurrentLocation != nil && strings.TrimSpace(req.CurrentLocation.Address) != "" {
			fields.LocationText = strings.TrimSpace(req.CurrentLocation.Address)
			fields.LocationCoords = &coordsReq{Latitude: req.CurrentLocation.Latitude, Longitude: req.CurrentLocation.Longitude}
			if !validSmartCoords(fields.LocationCoords) {
				fields.LocationCoords = nil
			}
			fields.LocationMode = "current"
			notes = append(notes, "地点使用了当前位置。")
			changed = true
		}
	}

	if !smartInputHasTimeHint(input) {
		if clock, ok := selectSmartHistoryClock(items, now.Location()); ok {
			fixed := nextSmartHistoryTime(now, clock)
			fields.TimeMode = "fixed"
			fields.TimeRange = 7
			fields.FixedTime = fixed.Format(time.RFC3339)
			fields.FixedTimeDisplay = fixed.Format("2006-01-02 15:04")
			fields.SelectedDate = fixed.Format("2006-01-02")
			fields.SelectedClock = fixed.Format("15:04")
			notes = append(notes, "时间参考了历史同类活动的常见星期和时间。")
			changed = true
		} else if days, ok := selectSmartHistoryRange(items); ok {
			fields.TimeMode = "range"
			fields.TimeRange = days
			notes = append(notes, "活动周期参考了历史同类活动的常用范围。")
			changed = true
		}
	}

	if !smartInputHasCountHint(input) {
		if maxCount, ok := selectSmartHistoryMaxCount(items); ok && maxCount != fields.MaxCount {
			fields.MaxCount = maxCount
			notes = append(notes, fmt.Sprintf("人数参考了历史同类活动的常见上限 %d 人。", maxCount))
			changed = true
		}
	}

	if descriptionHint != "" {
		notes = append(notes, "描述参考了历史同类活动的玩法细节。")
		changed = true
	}

	return fields, notes, changed, descriptionHint
}

func collectSmartHistoryItems(history smartPostHistoryReq, category, subCategory string, now time.Time) []smartHistoryItem {
	items := make([]smartHistoryItem, 0, len(history.InitiatedPosts)+len(history.JoinedPosts))
	for _, post := range history.InitiatedPosts {
		if smartHistoryPostMatches(post, category, subCategory) {
			items = append(items, smartHistoryItem{Post: post, Role: "initiated", Weight: smartHistoryWeight(post, "initiated", now)})
		}
	}
	for _, post := range history.JoinedPosts {
		if smartHistoryPostMatches(post, category, subCategory) {
			items = append(items, smartHistoryItem{Post: post, Role: "joined", Weight: smartHistoryWeight(post, "joined", now)})
		}
	}
	return items
}

func filterSmartPostHistory(history smartPostHistoryReq, category, subCategory string) smartPostHistoryReq {
	filtered := smartPostHistoryReq{
		InitiatedPosts: make([]smartPostHistoryPostReq, 0, len(history.InitiatedPosts)),
		JoinedPosts:    make([]smartPostHistoryPostReq, 0, len(history.JoinedPosts)),
	}
	for _, post := range history.InitiatedPosts {
		if smartHistoryPostMatches(post, category, subCategory) {
			filtered.InitiatedPosts = append(filtered.InitiatedPosts, post)
		}
	}
	for _, post := range history.JoinedPosts {
		if smartHistoryPostMatches(post, category, subCategory) {
			filtered.JoinedPosts = append(filtered.JoinedPosts, post)
		}
	}
	return filtered
}

func smartHistoryPostMatches(post smartPostHistoryPostReq, category, subCategory string) bool {
	postCategory := strings.TrimSpace(post.Category)
	postSubCategory := strings.TrimSpace(post.SubCategory)
	targetCategory := strings.TrimSpace(category)
	targetSubCategory := strings.TrimSpace(subCategory)
	if targetSubCategory != "" {
		return postCategory == targetCategory && postSubCategory == targetSubCategory
	}
	return targetCategory != "" && postCategory == targetCategory && postSubCategory == ""
}

func smartHistoryWeight(post smartPostHistoryPostReq, role string, now time.Time) float64 {
	weight := 1.0
	if role == "initiated" {
		weight = 2.0
	}
	if post.CreatedAt > 0 {
		ageDays := math.Max(0, float64(now.UnixMilli()-post.CreatedAt)/86400000.0)
		weight *= 1 + 1/(1+ageDays/30)
	}
	return weight
}

func selectSmartHistoryLocation(items []smartHistoryItem) (smartHistoryLocation, bool) {
	type bucket struct {
		Score   float64
		Count   int
		Payload smartHistoryLocation
	}
	buckets := map[string]bucket{}
	for _, item := range items {
		address := strings.TrimSpace(item.Post.Address)
		if address == "" {
			continue
		}
		payload := smartHistoryLocation{Address: address}
		if validSmartCoords(item.Post.Coords) {
			coords := *item.Post.Coords
			payload.Coords = &coords
		}
		current := buckets[address]
		if current.Payload.Address == "" {
			current.Payload = payload
		}
		current.Score += item.Weight
		current.Count++
		buckets[address] = current
	}
	var best bucket
	for _, candidate := range buckets {
		if candidate.Score > best.Score || (candidate.Score == best.Score && candidate.Count > best.Count) {
			best = candidate
		}
	}
	if best.Payload.Address == "" {
		return smartHistoryLocation{}, false
	}
	return best.Payload, true
}

func selectSmartHistoryClock(items []smartHistoryItem, loc *time.Location) (smartHistoryClock, bool) {
	type bucket struct {
		Score   float64
		Count   int
		Payload smartHistoryClock
	}
	buckets := map[string]bucket{}
	for _, item := range items {
		if strings.TrimSpace(item.Post.TimeInfo.Mode) != "fixed" {
			continue
		}
		ts, err := parseFixedTimeToMS(item.Post.TimeInfo.FixedTime)
		if err != nil || ts <= 0 {
			continue
		}
		date := time.UnixMilli(ts).In(loc)
		payload := smartHistoryClock{Weekday: int(date.Weekday()), Hour: date.Hour(), Minute: date.Minute()}
		key := fmt.Sprintf("%d-%d-%d", payload.Weekday, payload.Hour, payload.Minute)
		current := buckets[key]
		if current.Count == 0 {
			current.Payload = payload
		}
		current.Score += item.Weight
		current.Count++
		buckets[key] = current
	}
	var best bucket
	for _, candidate := range buckets {
		if candidate.Score > best.Score || (candidate.Score == best.Score && candidate.Count > best.Count) {
			best = candidate
		}
	}
	if best.Count == 0 {
		return smartHistoryClock{}, false
	}
	return best.Payload, true
}

func selectSmartHistoryRange(items []smartHistoryItem) (int, bool) {
	type bucket struct {
		Score float64
		Count int
		Days  int
	}
	buckets := map[int]bucket{}
	for _, item := range items {
		if strings.TrimSpace(item.Post.TimeInfo.Mode) != "range" {
			continue
		}
		days := clampInt(item.Post.TimeInfo.Days, 1, 30)
		current := buckets[days]
		current.Days = days
		current.Score += item.Weight
		current.Count++
		buckets[days] = current
	}
	var best bucket
	for _, candidate := range buckets {
		if candidate.Score > best.Score || (candidate.Score == best.Score && candidate.Count > best.Count) {
			best = candidate
		}
	}
	if best.Count == 0 {
		return 0, false
	}
	return best.Days, true
}

func selectSmartHistoryMaxCount(items []smartHistoryItem) (int, bool) {
	type bucket struct {
		Score float64
		Count int
		Value int
	}
	buckets := map[int]bucket{}
	for _, item := range items {
		value := item.Post.MaxCount
		if value < 2 {
			continue
		}
		value = clampInt(value, 2, 99)
		current := buckets[value]
		current.Value = value
		current.Score += item.Weight
		current.Count++
		buckets[value] = current
	}
	var best bucket
	for _, candidate := range buckets {
		if candidate.Score > best.Score || (candidate.Score == best.Score && candidate.Count > best.Count) {
			best = candidate
		}
	}
	if best.Count == 0 {
		return 0, false
	}
	return best.Value, true
}

type smartDescriptionCandidate struct {
	Text  string
	Score float64
}

func selectSmartHistoryDescriptionHint(items []smartHistoryItem, fields smartPostDraftFields) string {
	candidates := []smartDescriptionCandidate{}
	seen := map[string]struct{}{}
	for _, item := range items {
		sources := []string{item.Post.Title, item.Post.Description}
		for _, source := range sources {
			for _, segment := range splitSmartDescriptionSegments(source) {
				segment = normalizeSmartDescriptionSegment(segment, fields)
				if segment == "" {
					continue
				}
				if _, ok := seen[segment]; ok {
					continue
				}
				score := smartDescriptionSegmentScore(segment, fields)
				if score <= 0 {
					continue
				}
				seen[segment] = struct{}{}
				candidates = append(candidates, smartDescriptionCandidate{
					Text:  segment,
					Score: item.Weight + score,
				})
			}
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})
	parts := make([]string, 0, 2)
	for _, candidate := range candidates {
		if len(parts) == 2 {
			break
		}
		if smartDescriptionOverlaps(parts, candidate.Text) {
			continue
		}
		parts = append(parts, candidate.Text)
	}
	return trimRunes(strings.Join(parts, "，"), 52)
}

func splitSmartDescriptionSegments(value string) []string {
	text := strings.TrimSpace(value)
	replacers := []string{"。", "\n", "；", "\n", ";", "\n", "，", "\n", ",", "\n", "、", "\n"}
	for i := 0; i < len(replacers); i += 2 {
		text = strings.ReplaceAll(text, replacers[i], replacers[i+1])
	}
	return strings.Split(text, "\n")
}

func normalizeSmartDescriptionSegment(segment string, fields smartPostDraftFields) string {
	text := strings.TrimSpace(segment)
	text = strings.Trim(text, " ，,。；;：:")
	if text == "" {
		return ""
	}
	text = strings.ReplaceAll(text, "活动", "")
	text = strings.TrimSpace(text)
	if text == "" || len([]rune(text)) < 2 {
		return ""
	}
	if len([]rune(text)) > 36 {
		text = trimRunes(text, 36)
	}
	location := strings.TrimSpace(fields.LocationText)
	if location != "" && strings.Contains(text, location) {
		return ""
	}
	badWords := []string{"待定", "欢迎", "报名", "感兴趣", "时间", "地点", "人数", "上限", "发布", "参加"}
	for _, word := range badWords {
		if strings.Contains(text, word) {
			return ""
		}
	}
	if smartDescriptionLooksLikeTimeVenue(text) {
		return ""
	}
	if text == fields.SubCategory || text == fields.Category {
		return ""
	}
	if smartDescriptionConflictsWithActivity(text, fields) {
		return ""
	}
	return text
}

func smartDescriptionConflictsWithActivity(description string, fields smartPostDraftFields) bool {
	text := strings.TrimSpace(description)
	if text == "" {
		return false
	}
	subCategory := strings.TrimSpace(fields.SubCategory)
	conflicts := map[string][]string{
		"羽毛球": {"羽毛球", "羽球", "男双", "女双", "混双", "双打", "单打", "微对抗", "球拍", "球友", "dd"},
		"篮球":  {"篮球", "分队", "投篮"},
		"足球":  {"足球", "分队", "射门"},
		"游泳":  {"游泳", "泳池", "泳馆", "泳道", "泳镜", "泳帽", "自由泳", "蛙泳", "深水区"},
		"跑步":  {"跑步", "夜跑", "晨跑", "配速", "公里"},
		"骑行":  {"骑行", "单车", "头盔"},
	}
	for activity, keywords := range conflicts {
		if activity == subCategory {
			continue
		}
		for _, keyword := range keywords {
			if strings.Contains(text, keyword) {
				return true
			}
		}
	}
	return false
}

func smartDescriptionSegmentScore(segment string, fields smartPostDraftFields) float64 {
	score := 0.0
	keywords := []string{"男双", "女双", "混双", "双打", "单打", "对抗", "微对抗", "新手", "进阶", "水平", "自带", "球拍", "热身", "轮换", "dd", "局"}
	for _, keyword := range keywords {
		if strings.Contains(segment, keyword) {
			score += 1.0
		}
	}
	if strings.Contains(segment, "级") && strings.ContainsAny(segment, "0123456789一二三四五六七八九十") {
		score += 1.2
	}
	if fields.SubCategory != "" && strings.Contains(segment, fields.SubCategory) {
		score += 0.4
	}
	if score == 0 && len([]rune(segment)) <= 12 {
		score = 0.2
	}
	return score
}

func smartDescriptionLooksLikeTimeVenue(segment string) bool {
	if strings.Contains(segment, "级") {
		return false
	}
	if strings.ContainsAny(segment, "0123456789") {
		timeVenueHints := []string{"晚", "早", "点", ":", "：", "-", "—", "到", "体育馆", "球馆", "校区", "本部", "海淀", "昌平"}
		for _, hint := range timeVenueHints {
			if strings.Contains(segment, hint) {
				return true
			}
		}
	}
	return false
}

func smartDescriptionOverlaps(parts []string, candidate string) bool {
	for _, part := range parts {
		if strings.Contains(part, candidate) || strings.Contains(candidate, part) {
			return true
		}
	}
	return false
}

func smartDescriptionLooksGeneric(description string) bool {
	text := strings.TrimSpace(description)
	if text == "" || len([]rune(text)) < 45 {
		return true
	}
	genericHints := []string{"时间待定", "地点待定", "欢迎球友加入", "欢迎一起参加", "具体安排进群后再确认", "人数约"}
	for _, hint := range genericHints {
		if strings.Contains(text, hint) {
			return true
		}
	}
	return false
}

func nextSmartHistoryTime(now time.Time, clock smartHistoryClock) time.Time {
	days := (clock.Weekday - int(now.Weekday()) + 7) % 7
	target := time.Date(now.Year(), now.Month(), now.Day(), clock.Hour, clock.Minute, 0, 0, now.Location()).AddDate(0, 0, days)
	if !target.After(now) {
		target = target.AddDate(0, 0, 7)
	}
	return target
}

func smartInputHasTimeHint(input string) bool {
	source := strings.TrimSpace(input)
	if source == "" {
		return false
	}
	timeHints := []string{"今天", "今晚", "明天", "明晚", "后天", "大后天", "周", "星期", "礼拜", "最近", "近期", "周末", "上午", "中午", "下午", "晚上", "早上", "点", "：", ":"}
	for _, hint := range timeHints {
		if strings.Contains(source, hint) {
			return true
		}
	}
	if strings.Contains(source, "月") || strings.Contains(source, "号") || strings.Contains(source, "日") || strings.Contains(source, "天内") {
		return true
	}
	return false
}

func smartInputHasCountHint(input string) bool {
	source := strings.TrimSpace(input)
	if source == "" {
		return false
	}
	return strings.Contains(source, "人") || strings.Contains(source, "位")
}

func normalizeSmartPostTime(fields *smartPostDraftFields, now time.Time) {
	mode := strings.ToLower(strings.TrimSpace(fields.TimeMode))
	if mode == "fixed" {
		if fixed, ok := parseSmartFixedTime(*fields, now.Location()); ok && fixed.After(now) {
			fixed = fixed.In(now.Location())
			fields.TimeMode = "fixed"
			fields.TimeRange = 7
			fields.FixedTime = fixed.Format(time.RFC3339)
			fields.FixedTimeDisplay = fixed.Format("2006-01-02 15:04")
			fields.SelectedDate = fixed.Format("2006-01-02")
			fields.SelectedClock = fixed.Format("15:04")
			return
		}
	}
	fields.TimeMode = "range"
	fields.TimeRange = clampInt(fields.TimeRange, 1, 30)
	fields.FixedTime = ""
	fields.FixedTimeDisplay = ""
	fields.SelectedDate = ""
	if strings.TrimSpace(fields.SelectedClock) == "" {
		fields.SelectedClock = "09:00"
	}
}

func parseSmartFixedTime(fields smartPostDraftFields, loc *time.Location) (time.Time, bool) {
	dateText := strings.TrimSpace(fields.SelectedDate)
	clockText := strings.TrimSpace(fields.SelectedClock)
	if dateText != "" && clockText != "" {
		if t, err := time.ParseInLocation("2006-01-02 15:04", dateText+" "+clockText, loc); err == nil {
			return t, true
		}
	}

	raw := strings.TrimSpace(fields.FixedTime)
	if raw == "" {
		return time.Time{}, false
	}
	for _, layout := range fixedTimeLayouts {
		if layout == time.RFC3339Nano || layout == time.RFC3339 {
			if t, err := time.Parse(layout, raw); err == nil {
				return t.In(loc), true
			}
			continue
		}
		if t, err := time.ParseInLocation(layout, raw, loc); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func normalizeSmartPostCategory(raw string) string {
	switch strings.TrimSpace(raw) {
	case "运动", "娱乐", "学习", "其他":
		return strings.TrimSpace(raw)
	default:
		return "其他"
	}
}

func normalizeSmartPostSubCategory(category, raw string) string {
	value := trimRunes(strings.TrimSpace(raw), 8)
	if category == "其他" {
		if value == "" {
			return "其他活动"
		}
		return value
	}
	if allowed, ok := smartPostSubCategories[category]; ok {
		if _, exists := allowed[value]; exists {
			return value
		}
		return "其他" + category
	}
	return value
}

func normalizeSmartLocationMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "current":
		return "current"
	default:
		return "manual"
	}
}

func normalizeSmartPostSummary(summary []string, fields smartPostDraftFields) []string {
	result := make([]string, 0, 5)
	for _, item := range summary {
		item = trimRunes(strings.TrimSpace(item), 60)
		if item == "" {
			continue
		}
		result = append(result, item)
		if len(result) == 5 {
			break
		}
	}
	if len(result) == 0 {
		result = append(result, "识别为："+fields.Category+" / "+fields.SubCategory)
		result = append(result, "时间："+smartPostTimeSummary(fields))
		result = append(result, fmt.Sprintf("人数：%d 人", fields.MaxCount))
		if fields.LocationText != "" {
			result = append(result, "地点："+fields.LocationText)
		}
	}
	return result
}

func smartPostTimeSummary(fields smartPostDraftFields) string {
	if fields.TimeMode == "fixed" && fields.FixedTimeDisplay != "" {
		return fields.FixedTimeDisplay
	}
	return fmt.Sprintf("未来 %d 天内", fields.TimeRange)
}

func buildSmartPostFallbackTitle(fields smartPostDraftFields, now time.Time) string {
	parts := []string{}
	if fields.TimeMode == "fixed" && fields.FixedTimeDisplay != "" {
		parts = append(parts, smartPostTitleTime(fields, now))
	}
	if fields.LocationText != "" {
		parts = append(parts, shortSmartPostLocation(fields.LocationText))
	}
	parts = append(parts, fields.SubCategory+"局")
	return normalizeSmartPostTitle(strings.Join(parts, ""), fields, now)
}

func normalizeSmartPostTitle(title string, fields smartPostDraftFields, now time.Time) string {
	text := strings.TrimSpace(title)
	if text == "" {
		return ""
	}
	if strings.TrimSpace(fields.LocationText) != "" {
		text = strings.ReplaceAll(text, fields.LocationText, shortSmartPostLocation(fields.LocationText))
	}
	text = strings.ReplaceAll(text, "明天晚上", "明晚")
	text = strings.ReplaceAll(text, "今天晚上", "今晚")
	text = strings.ReplaceAll(text, "后天晚上", "后晚")
	text = strings.ReplaceAll(text, " ", "")
	text = strings.ReplaceAll(text, "（", "(")
	text = strings.ReplaceAll(text, "）", ")")
	text = stripSmartPostDanglingParen(text)
	if len([]rune(text)) > 18 {
		fallback := fields.SubCategory + "局"
		if fields.TimeMode == "fixed" {
			fallback = smartPostTitleTime(fields, now) + fallback
		}
		if location := shortSmartPostLocation(fields.LocationText); location != "" {
			fallback = smartPostTitleTime(fields, now) + location + fields.SubCategory + "局"
		}
		text = fallback
	}
	return trimRunes(text, 18)
}

func shortSmartPostLocation(address string) string {
	text := strings.TrimSpace(address)
	if text == "" {
		return ""
	}
	replacements := []struct {
		Match string
		Short string
	}{
		{Match: "北京邮电大学", Short: "北邮"},
		{Match: "北京邮电", Short: "北邮"},
		{Match: "邮电大学", Short: "北邮"},
	}
	for _, item := range replacements {
		if strings.Contains(text, item.Match) {
			return item.Short
		}
	}
	text = strings.ReplaceAll(text, "（", "(")
	text = strings.ReplaceAll(text, "）", ")")
	if idx := strings.Index(text, "("); idx >= 0 {
		text = strings.TrimSpace(text[:idx])
	}
	cityWords := []string{"北京市", "上海市", "广州市", "深圳市", "杭州市", "成都市", "武汉市", "南京市", "苏州市", "西安市", "重庆市", "天津市"}
	for _, word := range cityWords {
		text = strings.ReplaceAll(text, word, "")
	}
	suffixes := []string{"体育馆", "球馆", "校区"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(text, suffix) && len([]rune(text)) > len([]rune(suffix))+1 {
			text = strings.TrimSuffix(text, suffix)
			break
		}
	}
	text = stripSmartPostDanglingParen(strings.TrimSpace(text))
	return trimRunes(text, 8)
}

func stripSmartPostDanglingParen(value string) string {
	text := strings.TrimSpace(value)
	open := strings.LastIndex(text, "(")
	close := strings.LastIndex(text, ")")
	if open >= 0 && close < open {
		text = strings.TrimSpace(text[:open])
	}
	return text
}

func buildSmartPostFallbackDescription(fields smartPostDraftFields, historyHint string) string {
	where := "地点待补充"
	if fields.LocationText != "" {
		where = fields.LocationText
	}
	action := "一起" + fields.SubCategory
	switch fields.SubCategory {
	case "羽毛球", "篮球", "足球":
		action = "打" + fields.SubCategory
	case "跑步", "骑行", "游泳":
		action = "一起" + fields.SubCategory
	case "电影":
		action = "看电影"
	case "桌游", "游戏":
		action = "玩" + fields.SubCategory
	case "英语", "考研", "编程", "读书":
		action = "一起" + fields.SubCategory
	}
	parts := []string{
		fmt.Sprintf("%s 在%s%s，人数上限 %d 人。", smartPostTimeSummary(fields), where, action, fields.MaxCount),
	}
	if historyHint != "" {
		parts = append(parts, "参考历史安排："+historyHint+"。")
	}
	parts = append(parts, smartPostDefaultDescriptionTail(fields))
	return trimRunes(strings.Join(parts, ""), 220)
}

func smartPostDefaultDescriptionTail(fields smartPostDraftFields) string {
	switch fields.SubCategory {
	case "羽毛球":
		return "建议自带球拍，提前到场热身，到场后按人数和水平灵活轮换。"
	case "篮球", "足球":
		return "建议提前到场热身，到场后按人数灵活分队。"
	case "跑步", "骑行", "游泳":
		return "强度可以现场协商，注意安全和节奏。"
	case "桌游", "游戏":
		return "新手友好，具体玩法可以到场后一起商量。"
	case "电影":
		return "场次和座位可以进群后再确认。"
	case "英语", "考研", "编程", "读书":
		return "互相监督，保持安静高效，有问题可以一起讨论。"
	default:
		return "具体安排进群后再确认。"
	}
}

func smartPostTitleTime(fields smartPostDraftFields, now time.Time) string {
	if fields.TimeMode != "fixed" || fields.FixedTime == "" {
		return ""
	}
	ts, err := parseFixedTimeToMS(fields.FixedTime)
	if err != nil {
		return trimRunes(fields.FixedTimeDisplay, 12)
	}
	date := time.UnixMilli(ts).In(now.Location())
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	targetDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, now.Location())
	offset := int(targetDay.Sub(today).Hours() / 24)
	period := "晚上"
	if date.Hour() < 11 {
		period = "早上"
	} else if date.Hour() < 13 {
		period = "中午"
	} else if date.Hour() < 18 {
		period = "下午"
	}
	switch offset {
	case 0:
		if period == "晚上" {
			return "今晚"
		}
		return "今天" + period
	case 1:
		if period == "晚上" {
			return "明晚"
		}
		return "明天" + period
	case 2:
		if period == "晚上" {
			return "后晚"
		}
		return "后天" + period
	default:
		return fmt.Sprintf("%d月%d日%s", date.Month(), date.Day(), period)
	}
}

func deepSeekChatCompletionsURL(base string) (string, error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		base = defaultDeepSeekBaseURL
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	if !parsed.IsAbs() {
		return "", errors.New("DEEPSEEK_BASE_URL must be absolute")
	}
	if strings.HasSuffix(parsed.Path, "/chat/completions") {
		return parsed.String(), nil
	}
	joined, err := url.JoinPath(parsed.String(), "chat/completions")
	if err != nil {
		return "", err
	}
	return joined, nil
}

func smartPostTimeLocation() *time.Location {
	name := envOrDefault("SMART_POST_TIMEZONE", "Asia/Shanghai")
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.Local
	}
	return loc
}

func validSmartCoords(coords *coordsReq) bool {
	if coords == nil {
		return false
	}
	lat := coords.Latitude
	lng := coords.Longitude
	return !math.IsNaN(lat) && !math.IsNaN(lng) &&
		lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180 &&
		!(lat == 0 && lng == 0)
}

func compactPromptJSON(value interface{}, maxRunes int) string {
	if value == nil {
		return "null"
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "null"
	}
	return trimRunes(string(raw), maxRunes)
}

func clampInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func trimRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
