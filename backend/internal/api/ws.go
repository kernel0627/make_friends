package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"make_friends/backend/internal/model"
)

const (
	onlineSessionTTL = 90 * time.Second
	onlineHeartBeat  = 30 * time.Second
	// wsReadLimit caps a single inbound frame. Clients only send small
	// keepalive envelopes, so this is generous.
	wsReadLimit   = 8 * 1024
	wsWriteWait   = 10 * time.Second
	wsReadTimeout = 3 * onlineHeartBeat
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type wsIncoming struct {
	Type string `json:"type"`
}

type wsEvent struct {
	Type string `json:"type"`
	// Message carries the same shape the REST chat endpoints return, so a
	// client can render a pushed message without a follow-up fetch for the
	// sender's profile.
	Message *chatMessageView `json:"message,omitempty"`
	Code    string           `json:"code,omitempty"`
	Error   string           `json:"error,omitempty"`
}

func (s *Server) WSChat(c *gin.Context) {
	if !s.WSEnabled {
		fail(c, http.StatusNotFound, "WS_DISABLED", "websocket disabled")
		return
	}
	if !s.UseRedis || s.RedisClient == nil {
		fail(c, http.StatusServiceUnavailable, "REDIS_REQUIRED", "websocket requires redis")
		return
	}

	postID := strings.TrimSpace(c.Query("postId"))
	if postID == "" {
		fail(c, http.StatusBadRequest, "POST_ID_REQUIRED", "postId required")
		return
	}

	// This handler is registered outside the authed group, so it has to repeat
	// the checks RequireAuth performs. Skipping them let a logged-out user
	// (token revoked) or a banned user (soft-deleted) keep opening chat
	// sockets until their access token expired, up to a week later.
	userID, _, jti, ok := userIDFromRequest(c, s.JWTSecret)
	if !ok {
		fail(c, http.StatusUnauthorized, "AUTH_REQUIRED", "missing user identity")
		return
	}
	if jti != "" {
		revoked, err := s.isAccessTokenRevoked(jti)
		if err != nil {
			fail(c, http.StatusInternalServerError, "AUTH_CHECK_FAILED", "auth check failed")
			return
		}
		if revoked {
			fail(c, http.StatusUnauthorized, "ACCESS_TOKEN_REVOKED", "access token revoked")
			return
		}
	}
	_, _, deleted, suspended, found := s.resolveUserAccess(userID)
	if !found {
		fail(c, http.StatusUnauthorized, "USER_NOT_FOUND", "user no longer exists")
		return
	}
	if deleted || suspended {
		fail(c, http.StatusUnauthorized, "USER_DISABLED", "account has been disabled")
		return
	}
	isMember, err := s.isPostMember(postID, userID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "QUERY_MEMBER_FAILED", "query member failed")
		return
	}
	if !isMember {
		fail(c, http.StatusForbidden, "CHAT_ROOM_DENIED", "not post member")
		return
	}

	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	// Without these a slow or silent peer holds a goroutine and a connection
	// open indefinitely, and an oversized frame is read into memory whole.
	conn.SetReadLimit(wsReadLimit)
	_ = conn.SetReadDeadline(time.Now().Add(wsReadTimeout))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	channel := redisRoomChannel(postID)
	pubsub := s.RedisClient.Subscribe(ctx, channel)
	defer pubsub.Close()

	s.touchOnline(ctx, userID, postID)
	heartbeatTicker := time.NewTicker(onlineHeartBeat)
	defer heartbeatTicker.Stop()

	readErrCh := make(chan error, 1)
	go func() {
		for {
			var incoming wsIncoming
			if err := conn.ReadJSON(&incoming); err != nil {
				readErrCh <- err
				return
			}
			_ = conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
			s.touchOnline(ctx, userID, postID)
		}
	}()

	redisMsgCh := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			s.clearOnline(context.Background(), userID, postID)
			return
		case <-heartbeatTicker.C:
			s.touchOnline(ctx, userID, postID)
		case err := <-readErrCh:
			if err != nil {
				s.clearOnline(context.Background(), userID, postID)
				return
			}
		case redisMsg := <-redisMsgCh:
			if redisMsg == nil {
				continue
			}
			var event wsEvent
			if err := json.Unmarshal([]byte(redisMsg.Payload), &event); err != nil {
				log.Printf("ws payload decode failed: %v", err)
				continue
			}
			_ = conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			if err := conn.WriteJSON(event); err != nil {
				s.clearOnline(context.Background(), userID, postID)
				return
			}
		}
	}
}

func redisRoomMembersKey(postID string) string {
	return "zgbe:chat:room:" + postID + ":members"
}

func redisRoomChannel(postID string) string {
	return "zgbe:chat:pubsub:room:" + postID
}

func redisUserOnlineKey(userID string) string {
	return "zgbe:chat:online:" + userID
}

func (s *Server) touchOnline(ctx context.Context, userID, postID string) {
	if !s.UseRedis || s.RedisClient == nil {
		return
	}
	pipe := s.RedisClient.Pipeline()
	pipe.Set(ctx, redisUserOnlineKey(userID), postID, onlineSessionTTL)
	pipe.SAdd(ctx, redisRoomMembersKey(postID), userID)
	pipe.Expire(ctx, redisRoomMembersKey(postID), onlineSessionTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		log.Printf("redis touch online failed uid=%s post=%s err=%v", userID, postID, err)
	}
}

func (s *Server) clearOnline(ctx context.Context, userID, postID string) {
	if !s.UseRedis || s.RedisClient == nil {
		return
	}
	pipe := s.RedisClient.Pipeline()
	pipe.SRem(ctx, redisRoomMembersKey(postID), userID)
	pipe.Del(ctx, redisUserOnlineKey(userID))
	if _, err := pipe.Exec(ctx); err != nil {
		log.Printf("redis clear online failed uid=%s post=%s err=%v", userID, postID, err)
	}
}

func (s *Server) publishChatMessage(ctx context.Context, message model.ChatMessage) {
	if !s.UseRedis || s.RedisClient == nil {
		return
	}
	views, err := s.buildChatMessageViews([]model.ChatMessage{message})
	if err != nil || len(views) == 0 {
		log.Printf("ws publish skipped: build view failed post=%s err=%v", message.PostID, err)
		return
	}
	event := wsEvent{Type: "chat_message", Message: &views[0]}
	raw, err := json.Marshal(event)
	if err != nil {
		return
	}
	if err := s.RedisClient.Publish(ctx, redisRoomChannel(message.PostID), string(raw)).Err(); err != nil {
		log.Printf("redis publish chat message failed post=%s err=%v", message.PostID, err)
	}
}
