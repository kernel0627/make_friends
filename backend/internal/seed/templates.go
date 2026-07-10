package seed

import (
	"fmt"
	"net/url"
	"time"

	"make_friends/backend/internal/model"
)

type seedLocation struct {
	Name    string
	Address string
	Lat     float64
	Lng     float64
}

type seedActivityTemplate struct {
	Category     string
	SubCategory  string
	TitlePattern string
	Focus        string
	Requirement  string
	Duration     string
}

type seedActivityContent struct {
	Category    string
	SubCategory string
	Title       string
	Description string
	Address     string
	Lat         float64
	Lng         float64
}

var seedNicknamePrefixes = []string{"青", "白", "星", "木", "南", "北", "云", "夏", "秋", "冬"}
var seedNicknameSuffixes = []string{"岚", "禾", "桃", "舟", "林", "川", "宁", "芽", "月", "屿"}

var seedLocations = []seedLocation{
	{Name: "徐汇滨江", Address: "上海市徐汇区龙腾大道滨江步道 1888 号", Lat: 31.1839, Lng: 121.4547},
	{Name: "静安寺商圈", Address: "上海市静安区愚园路 68 号静安公园北门", Lat: 31.2238, Lng: 121.4465},
	{Name: "五角场万达", Address: "上海市杨浦区国宾路 58 号", Lat: 31.3016, Lng: 121.5116},
	{Name: "望京体育公园", Address: "北京市朝阳区望京街道阜通东大街 6 号", Lat: 39.9975, Lng: 116.4808},
	{Name: "国贸三里屯", Address: "北京市朝阳区三里屯路 19 号", Lat: 39.9375, Lng: 116.4543},
	{Name: "奥林匹克森林公园", Address: "北京市朝阳区科荟路 33 号", Lat: 40.0184, Lng: 116.3976},
	{Name: "珠江新城", Address: "广州市天河区花城大道 85 号", Lat: 23.1192, Lng: 113.3217},
	{Name: "大学城广工南门", Address: "广州市番禺区大学城外环西路 100 号", Lat: 23.0508, Lng: 113.3921},
	{Name: "深圳湾公园", Address: "深圳市南山区滨海大道深圳湾公园白鹭坡", Lat: 22.5238, Lng: 113.9469},
	{Name: "南山科技园", Address: "深圳市南山区高新南一道 008 号", Lat: 22.5403, Lng: 113.9547},
	{Name: "西湖文化广场", Address: "杭州市拱墅区展览东路 150 号", Lat: 30.2876, Lng: 120.1516},
	{Name: "滨江星光大道", Address: "杭州市滨江区江南大道 228 号", Lat: 30.2066, Lng: 120.2093},
	{Name: "成都东郊记忆", Address: "成都市成华区建设南支路 4 号", Lat: 30.6637, Lng: 104.1211},
	{Name: "成都金融城", Address: "成都市高新区交子大道 300 号", Lat: 30.5812, Lng: 104.0654},
	{Name: "武汉光谷步行街", Address: "武汉市洪山区关山大道 519 号", Lat: 30.5096, Lng: 114.4107},
	{Name: "南京仙林大学城", Address: "南京市栖霞区学衡路 8 号", Lat: 32.1128, Lng: 118.9183},
}

var seedActivityTemplates = []seedActivityTemplate{
	{Category: "运动", SubCategory: "羽毛球", TitlePattern: "%s 晚场羽毛球组队", Focus: "两小时友好对打，优先照顾新手和久没活动的人。", Requirement: "自带球拍更方便，现场也准备了两支备用拍。", Duration: "约 2 小时"},
	{Category: "运动", SubCategory: "篮球", TitlePattern: "%s 半场篮球局", Focus: "主打下班后放松，不拼强度，凑够人就开。", Requirement: "建议穿运动鞋，来之前简单热身。", Duration: "约 90 分钟"},
	{Category: "运动", SubCategory: "跑步", TitlePattern: "%s 夜跑搭子集合", Focus: "配速不卷，边跑边聊天，结束后一起拉伸。", Requirement: "能接受 3 到 5 公里慢跑即可。", Duration: "约 1 小时"},
	{Category: "运动", SubCategory: "骑行", TitlePattern: "%s 城市骑行小队", Focus: "走城市风景线，路程控制在 20 公里内。", Requirement: "自带单车和头盔，雨天自动改期。", Duration: "约 2 小时"},
	{Category: "娱乐", SubCategory: "电影", TitlePattern: "%s 一起看场新片", Focus: "先在门口集合买票，结束后愿意的话再去吃点东西。", Requirement: "不抢座，能接受影院现场排片调整。", Duration: "约 3 小时"},
	{Category: "娱乐", SubCategory: "桌游", TitlePattern: "%s 桌游轻社交局", Focus: "主打阿瓦隆、狼人杀、卡卡颂这些上手快的桌游。", Requirement: "新手也欢迎，现场会讲规则。", Duration: "约 3 小时"},
	{Category: "娱乐", SubCategory: "KTV", TitlePattern: "%s 晚上 K 歌放松局", Focus: "曲风不限，麦霸和气氛组选手都欢迎。", Requirement: "希望大家轮麦友好，不抢歌不冷场。", Duration: "约 2.5 小时"},
	{Category: "娱乐", SubCategory: "摄影", TitlePattern: "%s 城市散步拍照", Focus: "边逛边拍，想拍人像、街景和小店都可以。", Requirement: "手机和相机都行，愿意互拍最好。", Duration: "约 2 小时"},
	{Category: "学习", SubCategory: "自习", TitlePattern: "%s 一起自习打卡", Focus: "番茄钟模式，自习中尽量少闲聊。", Requirement: "请带上自己今天要完成的任务。", Duration: "约 2 小时"},
	{Category: "学习", SubCategory: "读书", TitlePattern: "%s 读书分享小组", Focus: "每人带一本到两本最近想聊的书，轻分享。", Requirement: "不需要做正式 PPT，愿意讲五分钟就够。", Duration: "约 90 分钟"},
	{Category: "学习", SubCategory: "编程", TitlePattern: "%s 编程互助共学", Focus: "适合下班后补项目、刷题或改简历。", Requirement: "自带电脑，问题可以提前丢到群里。", Duration: "约 2 小时"},
	{Category: "学习", SubCategory: "英语", TitlePattern: "%s 英语口语练习", Focus: "轻量话题轮聊，重点练输出，不纠结语法。", Requirement: "敢开口就行，大家互相包容。", Duration: "约 90 分钟"},
	{Category: "其他", SubCategory: "探店", TitlePattern: "%s 新店打卡计划", Focus: "一起去试试最近口碑不错的新店，顺便拍拍照。", Requirement: "默认 AA，临时改店会提前在群里说。", Duration: "约 2 小时"},
	{Category: "其他", SubCategory: "宠物", TitlePattern: "%s 带宠社交散步", Focus: "边遛边聊，给毛孩子放电，也给人放松。", Requirement: "请提前确认宠物性格稳定，牵引绳必带。", Duration: "约 90 分钟"},
	{Category: "其他", SubCategory: "志愿", TitlePattern: "%s 周末志愿活动", Focus: "一起做轻量公益，结束后复盘交流。", Requirement: "准时集合，临时有事请尽早说。", Duration: "约 3 小时"},
	{Category: "其他", SubCategory: "逛展", TitlePattern: "%s 一起看展慢逛", Focus: "不赶时间，边看边聊，适合想找同好的人。", Requirement: "默认现场买票，学生票自己带证件。", Duration: "约 2 小时"},
}

var seedTimeHints = []string{
	"工作日晚上 7 点集合", "周六下午 2 点集合", "周日早上 10 点集合", "周五晚上 8 点集合",
	"周三下班后集合", "周末上午 9 点集合", "周日下午 4 点集合", "今晚 7 点半集合",
}

var seedOutfits = []string{
	"穿舒服一点就好", "建议带一件薄外套", "运动场地记得穿运动鞋", "如果会久坐，带个水杯会更舒服",
	"晚上可能有风，最好别穿太单薄", "拍照局建议穿自己喜欢的颜色", "要久站的话鞋子尽量轻便", "现场有空调，怕冷的话带件开衫",
}

var seedMessageFormats = []string{
	"我会提前十分钟到 %s，如果有人先到可以在门口等等。",
	"这次活动节奏比较轻松，第一次来的人不用紧张，到 %s 找我就行。",
	"我已经把集合点发在群公告里了，大家到 %s 之后如果找不到位置直接群里说。",
	"如果临时迟到，记得在群里说一声，我们会在 %s 这边等五分钟。",
	"活动结束后如果大家还有精力，可以在 %s 附近顺路吃点东西。",
}

var seedReviewComments = []string{
	"人很守时，现场沟通也很顺，活动节奏舒服，下次还愿意一起。",
	"整体体验很好，临时有变化也会提前说明，比较靠谱。",
	"会主动照顾新来的同学，氛围很自然，互动也不尴尬。",
	"细节考虑得挺周到，集合、提醒和现场安排都比较清晰。",
}

func buildAvatarURL(seed string) string {
	rawSeed := seed
	if rawSeed == "" {
		rawSeed = "default"
	}
	return "https://api.dicebear.com/7.x/avataaars/svg?seed=" + url.QueryEscape(rawSeed)
}

func seedNicknameAt(index int) string {
	if len(seedNicknamePrefixes) == 0 || len(seedNicknameSuffixes) == 0 {
		return "同学"
	}
	return seedNicknamePrefixes[(index/len(seedNicknameSuffixes))%len(seedNicknamePrefixes)] + seedNicknameSuffixes[index%len(seedNicknameSuffixes)]
}

func buildSeedUsers(count int, passwordHash string, now int64, includeAdmin bool) []model.User {
	users := make([]model.User, 0, count)
	windowStart := now - int64(120*24*time.Hour/time.Millisecond)
	step := int64(0)
	if count > 0 {
		step = int64((110 * 24 * time.Hour / time.Millisecond) / time.Duration(count))
	}
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("user_seed_%03d", i+1)
		nickname := seedNicknameAt(i)
		createdAt := windowStart + int64(i)*step
		deletedAt := int64(0)
		deletedBy := ""
		if i >= count-4 {
			deletedAt = createdAt + int64(7*24*time.Hour/time.Millisecond)
			deletedBy = "system_seed"
		}

		users = append(users, model.User{
			ID:           id,
			Platform:     "password",
			OpenID:       "pwd_" + id,
			Nickname:     nickname,
			PasswordHash: passwordHash,
			AvatarURL:    buildAvatarURL(id),
			Role:         model.UserRoleUser,
			CreditScore:  78 + (i % 19),
			RatingScore:  3.8 + float64(i%8)*0.15,
			DeletedAt:    deletedAt,
			DeletedBy:    deletedBy,
			CreatedAt:    createdAt,
			UpdatedAt:    maxInt64(createdAt, deletedAt),
		})
	}
	if includeAdmin {
		adminNames := []string{"admin", "admin1", "admin2"}
		for index, nickname := range adminNames {
			id := fmt.Sprintf("user_admin_%02d", index+1)
			createdAt := now - int64(5-index)*int64(24*time.Hour/time.Millisecond)
			users = append(users, model.User{
				ID:           id,
				Platform:     "password",
				OpenID:       "pwd_" + nickname,
				Nickname:     nickname,
				PasswordHash: passwordHash,
				AvatarURL:    buildAvatarURL(nickname),
				Role:         model.UserRoleAdmin,
				RootAdmin:    nickname == "admin",
				CreditScore:  96 - index,
				RatingScore:  4.7 + float64(index)*0.1,
				CreatedAt:    createdAt,
				UpdatedAt:    createdAt,
			})
		}
	}
	return users
}

func buildSeedActivity(index int, author model.User) seedActivityContent {
	return buildSeedActivityFromChoice(index, index, index, index, author)
}

func buildSeedActivityFromChoice(templateIndex, locationIndex, timeHintIndex, outfitIndex int, author model.User) seedActivityContent {
	template := seedActivityTemplates[positiveModulo(templateIndex, len(seedActivityTemplates))]
	location := seedLocations[positiveModulo(locationIndex, len(seedLocations))]
	timeHint := seedTimeHints[positiveModulo(timeHintIndex, len(seedTimeHints))]
	outfit := seedOutfits[positiveModulo(outfitIndex, len(seedOutfits))]

	title := fmt.Sprintf(template.TitlePattern, location.Name)
	description := fmt.Sprintf(
		"%s，集合地点就在 %s。%s %s 活动预计 %s，默认大家在群里确认到场后统一开局。发起人是 %s，现场会负责点名和节奏提醒。%s 如果临时有事，请尽量提前在群里说明。",
		timeHint,
		location.Address,
		template.Focus,
		template.Requirement,
		template.Duration,
		author.Nickname,
		outfit,
	)

	return seedActivityContent{
		Category:    template.Category,
		SubCategory: template.SubCategory,
		Title:       title,
		Description: description,
		Address:     location.Address,
		Lat:         location.Lat,
		Lng:         location.Lng,
	}
}

func buildSeedMessages(post model.Post, author model.User, participantUsers []model.User, count int, index int) []model.ChatMessage {
	if count <= 0 {
		return nil
	}

	senders := make([]model.User, 0, len(participantUsers)+1)
	senders = append(senders, author)
	senders = append(senders, participantUsers...)
	if len(senders) == 0 {
		return nil
	}

	location := seedLocations[index%len(seedLocations)]
	messages := make([]model.ChatMessage, 0, count)
	for i := 0; i < count; i++ {
		sender := senders[i%len(senders)]
		content := seedMessageFormats[i%len(seedMessageFormats)]
		messages = append(messages, model.ChatMessage{
			ID:          fmt.Sprintf("msg_seed_%03d_%03d", index+1, i+1),
			PostID:      post.ID,
			SenderID:    sender.ID,
			Content:     fmt.Sprintf("%s：%s", sender.Nickname, fmt.Sprintf(content, location.Name)),
			ClientMsgID: fmt.Sprintf("client_seed_%03d_%03d", index+1, i+1),
			CreatedAt:   post.CreatedAt + int64((i+1)*100),
		})
	}
	return messages
}

func buildSeedReview(postID, fromUserID, toUserID string, createdAt int64, index int) model.Review {
	return model.Review{
		PostID:     postID,
		FromUserID: fromUserID,
		ToUserID:   toUserID,
		Rating:     4 + (index % 2),
		Comment:    seedReviewComments[index%len(seedReviewComments)],
		CreatedAt:  createdAt,
		UpdatedAt:  createdAt,
	}
}

func futureFixedTime(index int) string {
	return time.Now().Add(time.Duration(index+4) * time.Hour).Format(time.RFC3339)
}

func positiveModulo(index, size int) int {
	if size <= 0 {
		return 0
	}
	value := index % size
	if value < 0 {
		value += size
	}
	return value
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}
