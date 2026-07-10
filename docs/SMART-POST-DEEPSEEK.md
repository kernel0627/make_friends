# 智能发布 DeepSeek 接入

## 后端环境变量

- `DEEPSEEK_API_KEY`：必填，DeepSeek API Key。
- `DEEPSEEK_BASE_URL`：可选，默认 `https://api.deepseek.com`。
- `DEEPSEEK_MODEL`：可选，默认 `deepseek-chat`。
- `SMART_POST_TIMEZONE`：可选，默认 `Asia/Shanghai`。

前端只请求本项目后端的 `POST /api/v1/posts/smart-draft`，不要在小程序端保存 DeepSeek Key。

## JSON 输出约束

DeepSeek 请求使用 `response_format: {"type":"json_object"}`，服务端提示词要求模型只返回一个 JSON object。模型内部输出使用 `intent + form + fieldDecisions`，服务端会再次解析、清洗、校验，并在模型没填好时从历史同类活动补全时间、地点、人数和描述细节。

```json
{
  "intent": {
    "activityText": "羽毛球",
    "category": "运动",
    "subCategory": "羽毛球",
    "explicitTime": false,
    "explicitLocation": false,
    "explicitMaxCount": false,
    "needsUserConfirm": false,
    "ambiguities": []
  },
  "form": {
    "title": "4-32 个中文字符",
    "description": "40-180 个中文字符，包含时间/地点/人数信息，并参考历史同类活动里的玩法、水平、对抗强度、装备提醒等具体信息",
    "category": "运动 | 娱乐 | 学习 | 其他",
    "subCategory": "按分类枚举，其他分类时为 2-8 字具体类型",
    "timeMode": "fixed | range",
    "timeRange": 7,
    "selectedDate": "YYYY-MM-DD",
    "selectedClock": "HH:mm",
    "fixedTime": "",
    "fixedTimeDisplay": "",
    "locationMode": "manual | current",
    "locationText": "地点文本",
    "locationCoords": null,
    "maxCount": 4
  },
  "fieldDecisions": {
    "category": {
      "value": "运动 / 羽毛球",
      "source": "input",
      "confidence": 0.95,
      "reason": "用户输入中直接出现羽毛球"
    },
    "time": {
      "value": "下一个历史常用羽毛球时间",
      "source": "history",
      "confidence": 0.72,
      "reason": "用户未写时间，参考历史同类活动"
    },
    "location": {
      "value": "历史常用地点",
      "source": "history",
      "confidence": 0.74,
      "reason": "用户未写地点，参考历史同类活动"
    },
    "maxCount": {
      "value": 4,
      "source": "history",
      "confidence": 0.8,
      "reason": "用户未写人数，参考历史同类活动最常见人数"
    },
    "description": {
      "value": "4-5级男双，微对抗",
      "source": "history",
      "confidence": 0.76,
      "reason": "历史同类活动描述中出现可复用玩法细节"
    }
  },
  "missingFields": [],
  "summary": [
    "识别为：运动 / 羽毛球",
    "时间：2026-05-23 19:30"
  ]
}
```

对前端返回时仍然是原来的 `{ok, fields, summary}`，页面不用关心内部复杂结构。服务端会把 `selectedDate + selectedClock` 转成可入库的 ISO `fixedTime`，并保证固定时间晚于当前时间；如果用户没有明确说时间，服务端会优先使用本地时区解析后的历史常用时间，避免模型把 UTC 时间直接当成本地时间。
