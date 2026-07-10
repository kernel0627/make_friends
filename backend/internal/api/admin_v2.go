package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"make_friends/backend/internal/model"
)

type adminUserUpsertReq struct {
	Nickname  string `json:"nickname"`
	Password  string `json:"password"`
	AvatarURL string `json:"avatarUrl"`
}

type adminPostUpsertReq struct {
	AuthorID       string      `json:"authorId"`
	AuthorNickname string      `json:"authorNickname"`
	Title          string      `json:"title"`
	Description    string      `json:"description"`
	Category       string      `json:"category"`
	SubCategory    string      `json:"subCategory"`
	TimeInfo       timeInfoReq `json:"timeInfo"`
	Address        string      `json:"address"`
	Coords         *coordsReq  `json:"coords"`
	MaxCount       int         `json:"maxCount"`
	Status         string      `json:"status"`
}

type adminResetPasswordReq struct {
	Password string `json:"password"`
}

type analyticsPoint struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
}

type adminDashboardAnalyticsResponse struct {
	Window                 string           `json:"window"`
	DailyUsers             []analyticsPoint `json:"dailyUsers"`
	DailyPosts             []analyticsPoint `json:"dailyPosts"`
	DailyCases             []analyticsPoint `json:"dailyCases"`
	DailyCreditDeltas      []analyticsPoint `json:"dailyCreditDeltas"`
	CategoryDistribution   []analyticsPoint `json:"categoryDistribution"`
	TopSubCategories       []analyticsPoint `json:"topSubCategories"`
	CaseStatusDistribution []analyticsPoint `json:"caseStatusDistribution"`
	PostStatusDistribution []analyticsPoint `json:"postStatusDistribution"`
}

type bucketRow struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
}

func (s *Server) CreateAdminUser(c *gin.Context) {
	s.createManagedUser(c, model.UserRoleUser)
}

func (s *Server) UpdateAdminUser(c *gin.Context) {
	s.updateManagedUser(c, model.UserRoleUser)
}

func (s *Server) DeleteAdminUser(c *gin.Context) {
	s.softDeleteManagedUser(c, model.UserRoleUser)
}

func (s *Server) RestoreAdminUser(c *gin.Context) {
	s.restoreManagedUser(c, model.UserRoleUser)
}

func (s *Server) ResetAdminUserPassword(c *gin.Context) {
	s.resetManagedUserPassword(c, model.UserRoleUser)
}

func (s *Server) CreateAdminAccount(c *gin.Context) {
	s.createManagedUser(c, model.UserRoleAdmin)
}

func (s *Server) UpdateAdminAccount(c *gin.Context) {
	s.updateManagedUser(c, model.UserRoleAdmin)
}

func (s *Server) DeleteAdminAccount(c *gin.Context) {
	s.softDeleteManagedUser(c, model.UserRoleAdmin)
}

func (s *Server) RestoreAdminAccount(c *gin.Context) {
	s.restoreManagedUser(c, model.UserRoleAdmin)
}

func (s *Server) ResetAdminAccountPassword(c *gin.Context) {
	s.resetManagedUserPassword(c, model.UserRoleAdmin)
}

func (s *Server) CreateAdminPost(c *gin.Context) {
	var req adminPostUpsertReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	authorID := strings.TrimSpace(req.AuthorID)
	authorNickname := strings.TrimSpace(req.AuthorNickname)
	if authorID == "" && authorNickname == "" {
		fail(c, http.StatusBadRequest, "INVALID_REQUEST", "authorNickname is required")
		return
	}
	if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Category) == "" || strings.TrimSpace(req.Address) == "" {
		fail(c, http.StatusBadRequest, "INVALID_REQUEST", "title, category and address are required")
		return
	}
	if req.MaxCount < 2 {
		fail(c, http.StatusBadRequest, "INVALID_REQUEST", "maxCount must be >= 2")
		return
	}
	now := time.Now().UnixMilli()
	if err := validateTimeInfo(req.TimeInfo, now); err != nil {
		fail(c, http.StatusBadRequest, "INVALID_TIME_INFO", err.Error())
		return
	}
	var author model.User
	if err := s.findActiveAuthorForAdminPost(authorID, authorNickname, &author); err != nil {
		fail(c, http.StatusBadRequest, "INVALID_AUTHOR", err.Error())
		return
	}
	authorID = author.ID
	post := model.Post{
		ID:           "post_" + uuid.NewString()[:8],
		AuthorID:     authorID,
		Title:        strings.TrimSpace(req.Title),
		Description:  strings.TrimSpace(req.Description),
		Category:     strings.TrimSpace(req.Category),
		SubCategory:  strings.TrimSpace(req.SubCategory),
		TimeMode:     strings.TrimSpace(req.TimeInfo.Mode),
		TimeDays:     req.TimeInfo.Days,
		FixedTime:    strings.TrimSpace(req.TimeInfo.FixedTime),
		Address:      strings.TrimSpace(req.Address),
		MaxCount:     req.MaxCount,
		CurrentCount: 1,
		Status:       normalizePostStatus(req.Status),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if req.Coords != nil {
		post.Lat = req.Coords.Latitude
		post.Lng = req.Coords.Longitude
	}
	if post.Status == "closed" {
		post.ClosedAt = now
	}
	if err := s.DB.Create(&post).Error; err != nil {
		fail(c, http.StatusInternalServerError, "CREATE_POST_FAILED", "create post failed")
		return
	}
	s.invalidatePostsCache(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{"post": post})
}

func (s *Server) UpdateAdminPost(c *gin.Context) {
	postID := strings.TrimSpace(c.Param("id"))
	var req adminPostUpsertReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	now := time.Now().UnixMilli()
	if err := validateTimeInfo(req.TimeInfo, now); err != nil {
		fail(c, http.StatusBadRequest, "INVALID_TIME_INFO", err.Error())
		return
	}
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var post model.Post
		if err := tx.First(&post, "id = ?", postID).Error; err != nil {
			return err
		}
		if post.DeletedAt > 0 {
			return errors.New("deleted post cannot be edited")
		}
		if req.MaxCount < post.CurrentCount {
			return errors.New("maxCount must be >= currentCount")
		}
		authorID := strings.TrimSpace(req.AuthorID)
		authorNickname := strings.TrimSpace(req.AuthorNickname)
		if authorID == "" && authorNickname == "" {
			authorID = post.AuthorID
		}
		var author model.User
		if err := s.findActiveAuthorForAdminPostTx(tx, authorID, authorNickname, &author); err != nil {
			return err
		}
		post.AuthorID = author.ID
		post.Title = strings.TrimSpace(req.Title)
		post.Description = strings.TrimSpace(req.Description)
		post.Category = strings.TrimSpace(req.Category)
		post.SubCategory = strings.TrimSpace(req.SubCategory)
		post.TimeMode = strings.TrimSpace(req.TimeInfo.Mode)
		post.TimeDays = req.TimeInfo.Days
		post.FixedTime = strings.TrimSpace(req.TimeInfo.FixedTime)
		post.Address = strings.TrimSpace(req.Address)
		post.MaxCount = req.MaxCount
		post.UpdatedAt = now
		if req.Coords != nil {
			post.Lat = req.Coords.Latitude
			post.Lng = req.Coords.Longitude
		}
		nextStatus := normalizePostStatus(req.Status)
		post.Status = nextStatus
		if nextStatus == "closed" {
			if post.ClosedAt == 0 {
				post.ClosedAt = now
			}
		} else {
			post.ClosedAt = 0
		}
		return tx.Save(&post).Error
	})
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			fail(c, http.StatusNotFound, "POST_NOT_FOUND", "post not found")
		default:
			fail(c, http.StatusBadRequest, "UPDATE_POST_FAILED", err.Error())
		}
		return
	}
	s.invalidatePostsCache(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) DeleteAdminPost(c *gin.Context) {
	postID := strings.TrimSpace(c.Param("id"))
	adminUserID := mustUserID(c)
	now := time.Now().UnixMilli()
	result := s.DB.Model(&model.Post{}).
		Where("id = ? AND deleted_at = 0", postID).
		Updates(map[string]any{
			"deleted_at": now,
			"deleted_by": adminUserID,
			"updated_at": now,
		})
	if result.Error != nil {
		fail(c, http.StatusInternalServerError, "DELETE_POST_FAILED", "delete post failed")
		return
	}
	if result.RowsAffected == 0 {
		fail(c, http.StatusNotFound, "POST_NOT_FOUND", "post not found")
		return
	}
	s.invalidatePostsCache(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) RestoreAdminPost(c *gin.Context) {
	postID := strings.TrimSpace(c.Param("id"))
	now := time.Now().UnixMilli()
	result := s.DB.Model(&model.Post{}).
		Where("id = ? AND deleted_at > 0", postID).
		Updates(map[string]any{
			"deleted_at": 0,
			"deleted_by": "",
			"updated_at": now,
		})
	if result.Error != nil {
		fail(c, http.StatusInternalServerError, "RESTORE_POST_FAILED", "restore post failed")
		return
	}
	if result.RowsAffected == 0 {
		fail(c, http.StatusNotFound, "POST_NOT_FOUND", "post not found")
		return
	}
	s.invalidatePostsCache(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) AdminDashboardAnalytics(c *gin.Context) {
	window := strings.TrimSpace(strings.ToLower(c.DefaultQuery("window", "30d")))
	windowDays := int64(30)
	switch window {
	case "7d":
		windowDays = 7
	case "90d":
		windowDays = 90
	default:
		window = "30d"
		windowDays = 30
	}
	now := time.Now()
	fromMS := now.AddDate(0, 0, -int(windowDays-1)).UnixMilli()

	resp := adminDashboardAnalyticsResponse{Window: window}
	resp.DailyUsers = s.analyticsBuckets(activeUsersQuery(s.DB.Model(&model.User{})), "created_at", fromMS, int(windowDays))
	resp.DailyPosts = s.analyticsBuckets(activePostsQuery(s.DB.Model(&model.Post{})), "created_at", fromMS, int(windowDays))
	resp.DailyCases = s.analyticsBuckets(s.DB.Model(&model.AdminCase{}), "created_at", fromMS, int(windowDays))
	resp.DailyCreditDeltas = s.analyticsDeltaBuckets(s.DB.Model(&model.CreditLedger{}), "created_at", fromMS, int(windowDays))
	resp.CategoryDistribution = s.analyticsDistribution(activePostsQuery(s.DB.Model(&model.Post{})).Where("created_at >= ?", fromMS), "category", 8)
	resp.TopSubCategories = s.analyticsDistribution(activePostsQuery(s.DB.Model(&model.Post{})).Where("created_at >= ?", fromMS), "sub_category", 10)
	resp.CaseStatusDistribution = s.analyticsDistribution(s.DB.Model(&model.AdminCase{}).Where("created_at >= ?", fromMS), "status", 10)
	resp.PostStatusDistribution = s.postStatusDistribution(fromMS)
	c.JSON(http.StatusOK, resp)
}

func (s *Server) createManagedUser(c *gin.Context, targetRole string) {
	var req adminUserUpsertReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	nickname := strings.TrimSpace(req.Nickname)
	password := strings.TrimSpace(req.Password)
	if nickname == "" || len(password) < 6 {
		fail(c, http.StatusBadRequest, "INVALID_REQUEST", "nickname required and password must be at least 6 chars")
		return
	}
	var existed model.User
	if err := s.DB.First(&existed, "nickname = ?", nickname).Error; err == nil {
		fail(c, http.StatusConflict, "NICKNAME_ALREADY_EXISTS", "nickname already exists")
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		fail(c, http.StatusInternalServerError, "QUERY_USER_FAILED", "query user failed")
		return
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		fail(c, http.StatusInternalServerError, "PASSWORD_HASH_FAILED", "password hash failed")
		return
	}
	now := time.Now().UnixMilli()
	user := model.User{
		ID:           "user_pwd_" + strings.ReplaceAll(strings.ToLower(uuid.NewString()[:8]), "-", ""),
		Platform:     "password",
		OpenID:       "pwd_" + nickname,
		Nickname:     nickname,
		PasswordHash: string(hashed),
		AvatarURL:    normalizedAvatar(req.AvatarURL, nickname),
		Role:         targetRole,
		RootAdmin:    false,
		CreditScore:  100,
		RatingScore:  5,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.DB.Create(&user).Error; err != nil {
		fail(c, http.StatusInternalServerError, "CREATE_USER_FAILED", "create user failed")
		return
	}
	normalizeUserModel(&user)
	c.JSON(http.StatusOK, gin.H{"user": user})
}

func loadManagedUserForRoleTx(tx *gorm.DB, userID, targetRole string) (model.User, error) {
	var user model.User
	if err := tx.First(&user, "id = ?", strings.TrimSpace(userID)).Error; err != nil {
		return user, err
	}
	if model.NormalizeUserRole(user.Role) != targetRole {
		return user, errors.New("target role mismatch")
	}
	return user, nil
}

func (s *Server) updateManagedUser(c *gin.Context, targetRole string) {
	userID := strings.TrimSpace(c.Param("id"))
	var req adminUserUpsertReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	now := time.Now().UnixMilli()
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		user, err := loadManagedUserForRoleTx(tx, userID, targetRole)
		if err != nil {
			return err
		}
		if user.RootAdmin && targetRole == model.UserRoleAdmin {
			return errors.New("root admin cannot be edited here")
		}
		updates := map[string]any{
			"updated_at": now,
		}
		if nickname := strings.TrimSpace(req.Nickname); nickname != "" && nickname != user.Nickname {
			var count int64
			if err := tx.Model(&model.User{}).Where("nickname = ? AND id <> ?", nickname, user.ID).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return errors.New("nickname already exists")
			}
			updates["nickname"] = nickname
			updates["open_id"] = "pwd_" + nickname
		}
		if avatarURL := strings.TrimSpace(req.AvatarURL); avatarURL != "" {
			updates["avatar_url"] = avatarURL
		}
		return tx.Model(&model.User{}).Where("id = ?", user.ID).Updates(updates).Error
	})
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			fail(c, http.StatusNotFound, "USER_NOT_FOUND", "user not found")
		default:
			fail(c, http.StatusBadRequest, "UPDATE_USER_FAILED", err.Error())
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) softDeleteManagedUser(c *gin.Context, targetRole string) {
	userID := strings.TrimSpace(c.Param("id"))
	adminUserID := mustUserID(c)
	now := time.Now().UnixMilli()
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		user, err := loadManagedUserForRoleTx(tx, userID, targetRole)
		if err != nil {
			return err
		}
		if user.ID == adminUserID {
			return errors.New("cannot delete yourself")
		}
		if user.RootAdmin {
			return errors.New("cannot delete the root admin")
		}
		return tx.Model(&model.User{}).Where("id = ? AND deleted_at = 0", user.ID).Updates(map[string]any{
			"deleted_at": now,
			"deleted_by": adminUserID,
			"updated_at": now,
		}).Error
	})
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			fail(c, http.StatusNotFound, "USER_NOT_FOUND", "user not found")
		default:
			fail(c, http.StatusBadRequest, "DELETE_USER_FAILED", err.Error())
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) restoreManagedUser(c *gin.Context, targetRole string) {
	userID := strings.TrimSpace(c.Param("id"))
	now := time.Now().UnixMilli()
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		user, err := loadManagedUserForRoleTx(tx, userID, targetRole)
		if err != nil {
			return err
		}
		return tx.Model(&model.User{}).Where("id = ?", user.ID).Updates(map[string]any{
			"deleted_at": 0,
			"deleted_by": "",
			"updated_at": now,
		}).Error
	})
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			fail(c, http.StatusNotFound, "USER_NOT_FOUND", "user not found")
		default:
			fail(c, http.StatusBadRequest, "RESTORE_USER_FAILED", err.Error())
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) resetManagedUserPassword(c *gin.Context, targetRole string) {
	userID := strings.TrimSpace(c.Param("id"))
	var req adminResetPasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	password := strings.TrimSpace(req.Password)
	if len(password) < 6 {
		fail(c, http.StatusBadRequest, "INVALID_PASSWORD", "password must be at least 6 chars")
		return
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		fail(c, http.StatusInternalServerError, "PASSWORD_HASH_FAILED", "password hash failed")
		return
	}
	now := time.Now().UnixMilli()
	err = s.DB.Transaction(func(tx *gorm.DB) error {
		user, err := loadManagedUserForRoleTx(tx, userID, targetRole)
		if err != nil {
			return err
		}
		return tx.Model(&model.User{}).Where("id = ?", user.ID).Updates(map[string]any{
			"password_hash": string(hashed),
			"updated_at":    now,
		}).Error
	})
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			fail(c, http.StatusNotFound, "USER_NOT_FOUND", "user not found")
		default:
			fail(c, http.StatusBadRequest, "RESET_PASSWORD_FAILED", err.Error())
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) analyticsBuckets(query *gorm.DB, column string, fromMS int64, days int) []analyticsPoint {
	points := emptyDailyPoints(days)
	var rows []bucketRow
	sql := "strftime('%Y-%m-%d', " + column + "/1000, 'unixepoch', 'localtime') AS label, COUNT(*) AS value"
	_ = query.Select(sql).Where(column+" >= ?", fromMS).Group("label").Order("label ASC").Scan(&rows).Error
	mergeBucketRows(points, rows)
	return points
}

func (s *Server) analyticsDeltaBuckets(query *gorm.DB, column string, fromMS int64, days int) []analyticsPoint {
	points := emptyDailyPoints(days)
	var rows []bucketRow
	sql := "strftime('%Y-%m-%d', " + column + "/1000, 'unixepoch', 'localtime') AS label, ABS(SUM(delta)) AS value"
	_ = query.Select(sql).Where(column+" >= ?", fromMS).Group("label").Order("label ASC").Scan(&rows).Error
	mergeBucketRows(points, rows)
	return points
}

func (s *Server) analyticsDistribution(query *gorm.DB, column string, limit int) []analyticsPoint {
	var rows []bucketRow
	_ = query.Select(column + " AS label, COUNT(*) AS value").Where(column + " <> ''").Group(column).Order("value DESC").Limit(limit).Scan(&rows).Error
	result := make([]analyticsPoint, 0, len(rows))
	for _, row := range rows {
		result = append(result, analyticsPoint{Label: row.Label, Value: row.Value})
	}
	return result
}

func (s *Server) postStatusDistribution(fromMS int64) []analyticsPoint {
	type row struct {
		Label string  `json:"label"`
		Value float64 `json:"value"`
	}
	var rows []row
	_ = s.DB.Model(&model.Post{}).
		Select("CASE WHEN deleted_at > 0 THEN 'deleted' ELSE status END AS label, COUNT(*) AS value").
		Where("created_at >= ?", fromMS).
		Group("label").
		Order("value DESC").
		Scan(&rows).Error
	result := make([]analyticsPoint, 0, len(rows))
	for _, item := range rows {
		result = append(result, analyticsPoint{Label: item.Label, Value: item.Value})
	}
	return result
}

func emptyDailyPoints(days int) []analyticsPoint {
	out := make([]analyticsPoint, 0, days)
	start := time.Now().AddDate(0, 0, -(days - 1))
	for i := 0; i < days; i++ {
		day := start.AddDate(0, 0, i)
		out = append(out, analyticsPoint{
			Label: day.Format("01-02"),
			Value: 0,
		})
	}
	return out
}

func mergeBucketRows(points []analyticsPoint, rows []bucketRow) {
	indexByLabel := make(map[string]int, len(points))
	for index, point := range points {
		indexByLabel[point.Label] = index
	}
	for _, row := range rows {
		if row.Label == "" {
			continue
		}
		shortLabel := row.Label
		if len(shortLabel) >= 5 {
			shortLabel = shortLabel[5:]
		}
		if index, ok := indexByLabel[shortLabel]; ok {
			points[index].Value = row.Value
		}
	}
}

func normalizedAvatar(raw, fallbackSeed string) string {
	value := strings.TrimSpace(raw)
	if value != "" {
		return value
	}
	return avatarURLFromSeed(fallbackSeed)
}

func normalizePostStatus(raw string) string {
	if strings.TrimSpace(strings.ToLower(raw)) == "closed" {
		return "closed"
	}
	return "open"
}

func (s *Server) findActiveAuthorForAdminPost(authorID, authorNickname string, out *model.User) error {
	return s.findActiveAuthorForAdminPostTx(s.DB, authorID, authorNickname, out)
}

func (s *Server) findActiveAuthorForAdminPostTx(tx *gorm.DB, authorID, authorNickname string, out *model.User) error {
	authorID = strings.TrimSpace(authorID)
	authorNickname = strings.TrimSpace(authorNickname)
	switch {
	case authorNickname != "":
		if err := activeUsersQuery(tx).First(out, "nickname = ?", authorNickname).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("author nickname not found")
			}
			return err
		}
		return nil
	case authorID != "":
		if err := activeUsersQuery(tx).First(out, "id = ?", authorID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("author not found")
			}
			return err
		}
		return nil
	default:
		return errors.New("author nickname is required")
	}
}
