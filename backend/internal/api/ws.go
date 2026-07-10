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
	Type    string            `json:"type"`
	Message model.ChatMessage `json:"message,omitempty"`
	Code    string            `json:"code,omitempty"`
	Error   string            `json:"error,omitempty"`
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

	userID, _, _, ok := userIDFromRequest(c, s.JWTSecret)
	if !ok {
		fail(c, http.StatusUnauthorized, "AUTH_REQUIRED", "missing user identity")
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
	event := wsEvent{Type: "chat_message", Message: message}
	raw, err := json.Marshal(event)
	if err != nil {
		return
	}
	if err := s.RedisClient.Publish(ctx, redisRoomChannel(message.PostID), string(raw)).Err(); err != nil {
		log.Printf("redis publish chat message failed post=%s err=%v", message.PostID, err)
	}
}
