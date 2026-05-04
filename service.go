package main

import (
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	userStore      *UserStore
	msgStore       *MessageStore
	groupStore     *GroupStore
	groupMemStore  *GroupMemberStore
	friendStore    *FriendRequestStore
	convPartStore  *ConversationParticipantStore
	DeviceStore    *UserDeviceStore
	idGenStore     *IdGeneratorStateStore
	idGen          *IdGenerator
	rateLimiter    *RateLimiter
}

func NewService(
	userStore *UserStore,
	msgStore *MessageStore,
	groupStore *GroupStore,
	groupMemStore *GroupMemberStore,
	friendStore *FriendRequestStore,
	convPartStore *ConversationParticipantStore,
	deviceStore *UserDeviceStore,
	idGenStore *IdGeneratorStateStore,
	idGen *IdGenerator,
	rateLimiter *RateLimiter,
) *Service {
	return &Service{
		userStore:     userStore,
		msgStore:      msgStore,
		groupStore:    groupStore,
		groupMemStore: groupMemStore,
		friendStore:   friendStore,
		convPartStore: convPartStore,
		DeviceStore:   deviceStore,
		idGenStore:    idGenStore,
		idGen:         idGen,
		rateLimiter:   rateLimiter,
	}
}

// ==================== UserService ====================

func (s *Service) GetUserByID(userID int64) (*User, error) {
	user, err := s.userStore.GetByID(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrNotFound
	}
	return user, nil
}

func (s *Service) GetUserByUsername(username string) (*User, error) {
	user, err := s.userStore.GetByUsername(username)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrNotFound
	}
	return user, nil
}

func (s *Service) GetUserByPublicID(publicID string) (*User, error) {
	user, err := s.userStore.GetByPublicID(publicID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrNotFound
	}
	return user, nil
}

func (s *Service) CreateUser(username, password, email string) (int64, error) {
	exists, err := s.userStore.UsernameExists(username)
	if err != nil {
		return 0, err
	}
	if exists {
		return 0, ErrConflict
	}

	if email != "" {
		emailExists, err := s.userStore.EmailExists(email)
		if err != nil {
			return 0, err
		}
		if emailExists {
			return 0, ErrConflict
		}
	}

	pwdHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, fmt.Errorf("password hashing failed: %w", err)
	}

	publicID := GenerateNanoID(21)

	now := time.Now().UnixMilli()
	user := &User{
		Username:  username,
		PwdHash:   string(pwdHash),
		Email:     email,
		PublicID:  publicID,
		CreatedAt: now,
		UpdatedAt: now,
	}

	result, err := s.userStore.Insert(user)
	if err != nil {
		return 0, err
	}

	return result, nil
}

func (s *Service) UpdateUser(user *User) error {
	user.UpdatedAt = time.Now().UnixMilli()
	_, err := s.userStore.Update(user)
	return err
}

func (s *Service) DeleteUser(userID int64) error {
	user, err := s.userStore.GetByID(userID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrNotFound
	}

	// cascade delete
	if _, err := s.friendStore.DeleteByUser(userID); err != nil {
		return err
	}
	if _, err := s.groupMemStore.DeleteByUser(userID); err != nil {
		return err
	}
	if _, err := s.DeviceStore.DeleteByUser(userID); err != nil {
		return err
	}
	if _, err := s.userStore.Delete(userID); err != nil {
		return err
	}

	return nil
}

func (s *Service) ValidateCredentials(username, password string) (bool, error) {
	user, err := s.userStore.GetByUsername(username)
	if err != nil {
		return false, err
	}
	if user == nil {
		return false, ErrNotFound
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PwdHash), []byte(password)); err != nil {
		return false, ErrInvalidCredentials
	}

	return true, nil
}

// ==================== FriendRequestService ====================

func (s *Service) SendFriendRequest(fromUserID, toUserID int64, message string) (requestID int64, autoAccepted bool, err error) {
	if fromUserID == toUserID {
		return 0, false, ErrSelfRequest
	}

	targetUser, err := s.userStore.GetByID(toUserID)
	if err != nil {
		return 0, false, err
	}
	if targetUser == nil {
		return 0, false, ErrNotFound
	}

	existing, err := s.friendStore.GetByUsers(fromUserID, toUserID)
	if err != nil {
		return 0, false, err
	}
	if existing != nil {
		return 0, false, ErrConflict
	}

	// check for reverse pending request (auto-accept)
	reverse, err := s.friendStore.GetByUsers(toUserID, fromUserID)
	if err != nil {
		return 0, false, err
	}
	if reverse != nil && reverse.Status == "pending" {
		reverse.Status = "accepted"
		reverse.UpdatedAt = time.Now().UnixMilli()
		if _, err := s.friendStore.Update(reverse); err != nil {
			return 0, false, err
		}
		_ = s.SendFriendAcceptedNotification(fromUserID, toUserID, fmt.Sprintf("%d", reverse.ID))
		return reverse.ID, true, nil
	}

	req := &FriendRequest{
		FromUserID: fromUserID,
		ToUserID:   toUserID,
		Message:    message,
		Status:     "pending",
		CreatedAt:  time.Now().UnixMilli(),
		UpdatedAt:  time.Now().UnixMilli(),
	}

	result, err := s.friendStore.Insert(req)
	if err != nil {
		return 0, false, err
	}

	go func() {
		_ = s.SendFriendNotification(toUserID, fromUserID, fmt.Sprintf("%d", result))
	}()

	return result, false, nil
}

func (s *Service) GetFriendRequestByID(requestID int64) (*FriendRequest, error) {
	req, err := s.friendStore.GetByID(requestID)
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, ErrNotFound
	}
	return req, nil
}

func (s *Service) GetSentRequests(fromUserID int64) ([]*FriendRequest, error) {
	return s.friendStore.GetSentRequests(fromUserID)
}

func (s *Service) GetReceivedRequests(toUserID int64) ([]*FriendRequest, error) {
	return s.friendStore.GetReceivedRequests(toUserID)
}

func (s *Service) AcceptFriendRequest(requestID, userID int64) error {
	req, err := s.friendStore.GetByID(requestID)
	if err != nil {
		return err
	}
	if req == nil {
		return ErrNotFound
	}
	if req.ToUserID != userID {
		return ErrForbidden
	}
	if req.Status != "pending" {
		return ErrNotPending
	}

	req.Status = "accepted"
	req.UpdatedAt = time.Now().UnixMilli()
	_, err = s.friendStore.Update(req)
	if err != nil {
		return err
	}

	go func() {
		_ = s.SendFriendAcceptedNotification(req.FromUserID, req.ToUserID, fmt.Sprintf("%d", req.ID))
	}()

	return nil
}

func (s *Service) RejectFriendRequest(requestID, userID int64) error {
	req, err := s.friendStore.GetByID(requestID)
	if err != nil {
		return err
	}
	if req == nil {
		return ErrNotFound
	}
	if req.ToUserID != userID {
		return ErrForbidden
	}
	if req.Status != "pending" {
		return ErrNotPending
	}

	req.Status = "rejected"
	req.UpdatedAt = time.Now().UnixMilli()
	_, err = s.friendStore.Update(req)
	return err
}

func (s *Service) CancelFriendRequest(requestID, fromUserID int64) error {
	req, err := s.friendStore.GetByID(requestID)
	if err != nil {
		return err
	}
	if req == nil {
		return ErrNotFound
	}
	if req.FromUserID != fromUserID {
		return ErrForbidden
	}
	if req.Status != "pending" {
		return ErrNotPending
	}

	_, err = s.friendStore.Delete(requestID)
	return err
}

// ==================== GroupService ====================

func (s *Service) GetGroupByID(groupID string) (*Group, error) {
	group, err := s.groupStore.GetByID(groupID)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, ErrNotFound
	}
	return group, nil
}

func (s *Service) GetGroupByName(name string) (*Group, error) {
	group, err := s.groupStore.GetByName(name)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, ErrNotFound
	}
	return group, nil
}

func (s *Service) GetGroupsByOwner(ownerID int64) ([]*Group, error) {
	return s.groupStore.GetGroupsByOwner(ownerID)
}

func (s *Service) CreateGroup(name, description string, ownerID int64) (string, error) {
	exists, err := s.groupStore.NameExists(name)
	if err != nil {
		return "", err
	}
	if exists {
		return "", ErrConflict
	}

	now := time.Now().UnixMilli()
	groupID := GenerateGroupID()

	group := &Group{
		GroupID:     groupID,
		Name:        name,
		Description: description,
		OwnerID:     ownerID,
		CreatedAt:   now,
		UpdatedAt:   now,
		IsDeleted:   0,
	}

	_, err = s.groupStore.Insert(group)
	if err != nil {
		return "", err
	}

	return groupID, nil
}

func (s *Service) UpdateGroup(group *Group) error {
	group.UpdatedAt = time.Now().UnixMilli()
	_, err := s.groupStore.Update(group)
	return err
}

func (s *Service) DeleteGroup(groupID string) error {
	_, err := s.groupStore.Delete(groupID)
	return err
}

func (s *Service) AddMember(groupID string, userID int64, role string) error {
	isMember, err := s.groupMemStore.IsMember(groupID, userID)
	if err != nil {
		return err
	}
	if isMember {
		return ErrConflict
	}

	group, err := s.groupStore.GetByID(groupID)
	if err != nil {
		return err
	}
	if group == nil {
		return ErrNotFound
	}

	member := &GroupMember{
		GroupID:  groupID,
		UID:      userID,
		Role:     role,
		JoinedAt: time.Now().UnixMilli(),
	}

	_, err = s.groupMemStore.Insert(member)
	if err != nil {
		return err
	}

	go func() {
		_ = s.SendGroupInviteNotification(userID, groupID, group.Name, group.OwnerID)
	}()

	return nil
}

func (s *Service) RemoveMember(groupID string, userID int64) error {
	isMember, err := s.groupMemStore.IsMember(groupID, userID)
	if err != nil {
		return err
	}
	if !isMember {
		return ErrNotMember
	}

	member, err := s.groupMemStore.Get(groupID, userID)
	if err != nil {
		return err
	}
	if member.Role == "owner" {
		return ErrForbidden
	}

	_, err = s.groupMemStore.DeleteByGroupAndUser(groupID, userID)
	return err
}

func (s *Service) IsUserInGroup(groupID string, userID int64) (bool, error) {
	return s.groupMemStore.IsMember(groupID, userID)
}

// ==================== MessageService ====================

type SendMessageResult struct {
	MsgID      string `json:"msg_id"`
	ConvID     string `json:"conv_id"`
	FromUID    int64  `json:"from_uid"`
	Content    string `json:"content"`
	Ts         int64  `json:"ts"`
	IsRecalled int    `json:"is_recalled"`
}

func (s *Service) SendMessage(currentUser *User, targetPublicID, msgType, content string) (*SendMessageResult, error) {
	uid := currentUser.ID
	publicID := currentUser.PublicID

	if !s.rateLimiter.AllowRequest(publicID) {
		return nil, ErrRateLimitExceeded
	}

	convID, err := s.generateConversationID(msgType, uid, targetPublicID)
	if err != nil {
		return nil, err
	}

	if err := s.ensureSessionPermission(uid, convID, msgType, targetPublicID); err != nil {
		return nil, err
	}

	msgID, err := s.idGen.GenerateID()
	if err != nil {
		return nil, err
	}

	ts := time.Now().UnixMilli() - AppCfg.Time.EpochTime
	tableName := MessageTableNameByTs(ts)

	msg := &Message{
		MsgID:   msgID,
		ConvID:  convID,
		FromUID: uid,
		Content: content,
		Ts:      ts,
	}

	_, err = s.msgStore.InsertMessage(tableName, msg)
	if err != nil {
		return nil, err
	}

	result := &SendMessageResult{
		MsgID:   fmt.Sprintf("%d", msgID),
		ConvID:  convID,
		FromUID: uid,
		Content: content,
		Ts:      ts,
	}

	return result, nil
}

func (s *Service) GetMessage(msgID, msgTimestamp int64) (*Message, error) {
	tableName := MessageTableNameByTs(msgTimestamp)
	msg, err := s.msgStore.GetMessage(tableName, msgID)
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, ErrNotFound
	}
	return msg, nil
}

func (s *Service) GetConversationMessages(convID string, date time.Time, pageSize, offset int, userID int64) ([]*Message, error) {
	hasPerm, err := s.convPartStore.Exists(convID, userID)
	if err != nil {
		return nil, err
	}

	if !hasPerm {
		if groupID := ExtractGroupID(convID); groupID != "" {
			isMember, err := s.groupMemStore.IsMember(groupID, userID)
			if err != nil {
				return nil, err
			}
			if !isMember {
				return nil, ErrForbidden
			}
		} else {
			return nil, ErrForbidden
		}
	}

	tableName := MessageTableNameByDate(date)
	return s.msgStore.GetConversationMessages(tableName, convID, pageSize, offset)
}

func (s *Service) RecallMessage(msgID, msgTimestamp, userID int64) error {
	now := time.Now().UnixMilli()
	msgAge := now - msgTimestamp
	if msgAge > 120_000 {
		return ErrInvalidInput
	}

	tableName := MessageTableNameByTs(msgTimestamp)
	msg, err := s.msgStore.GetMessage(tableName, msgID)
	if err != nil {
		return err
	}
	if msg == nil {
		return ErrNotFound
	}
	if msg.IsRecalled == 1 {
		return ErrConflict
	}
	if msg.FromUID != userID {
		return ErrForbidden
	}

	msg.IsRecalled = 1
	msg.Content = ""
	_, err = s.msgStore.UpdateMessage(tableName, msg)
	return err
}

func (s *Service) generateConversationID(msgType string, uid int64, targetPublicID string) (string, error) {
	if msgType == "p" || msgType == "user" {
		targetUser, err := s.userStore.GetByPublicID(targetPublicID)
		if err != nil {
			return "", err
		}
		if targetUser == nil {
			return "", ErrInvalidTargetID
		}
		return PrivateConvID(uid, targetUser.ID), nil
	} else if msgType == "g" || msgType == "group" {
		if !IsValidGroupID(targetPublicID) {
			return "", ErrInvalidType
		}
		return targetPublicID, nil
	}
	return "", ErrInvalidType
}

func (s *Service) ensureSessionPermission(uid int64, convID, msgType, targetPublicID string) error {
	if msgType == "p" || msgType == "user" {
		if !CanAccessPrivateConv(convID, uid) {
			return ErrForbidden
		}

		hasPerm, err := s.convPartStore.Exists(convID, uid)
		if err != nil {
			return err
		}
		if !hasPerm {
			targetUser, err := s.userStore.GetByPublicID(targetPublicID)
			if err != nil {
				return err
			}
			if targetUser == nil {
				return ErrInvalidTargetID
			}

			now := time.Now().UnixMilli()
			participants := []*ConversationParticipant{
				{ConvID: convID, UserID: uid, JoinTs: now},
				{ConvID: convID, UserID: targetUser.ID, JoinTs: now},
			}
			_, err = s.convPartStore.InsertBatch(participants)
			if err != nil {
				return err
			}
		}
		return nil
	} else if msgType == "g" || msgType == "group" {
		isMember, err := s.groupMemStore.IsMember(targetPublicID, uid)
		if err != nil {
			return err
		}
		if !isMember {
			return ErrNotMember
		}
		return nil
	}
	return ErrInvalidType
}

// ==================== SystemNotification ====================

func (s *Service) SendNotificationToUser(uid int64, notifType string, data map[string]interface{}) (int64, error) {
	convID := fmt.Sprintf("system_%d", uid)

	payload := &NotificationPayload{
		Type:    "system_notification",
		SubType: notifType,
		Data:    data,
	}

	content, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("notification marshal failed: %w", err)
	}

	msgID, err := s.idGen.GenerateID()
	if err != nil {
		return 0, err
	}

	ts := time.Now().UnixMilli() - AppCfg.Time.EpochTime
	tableName := MessageTableNameByTs(ts)

	msg := &Message{
		MsgID:   msgID,
		ConvID:  convID,
		FromUID: 0,
		Content: string(content),
		Ts:      ts,
	}

	_, err = s.msgStore.InsertMessage(tableName, msg)
	if err != nil {
		return 0, err
	}

	return msgID, nil
}

func (s *Service) SendFriendNotification(receiverUID, senderUID int64, requestID string) error {
	_, err := s.SendNotificationToUser(receiverUID, "friend_request", map[string]interface{}{
		"request_id": requestID,
		"sender_uid": senderUID,
		"action":     "received",
	})
	return err
}

func (s *Service) SendFriendAcceptedNotification(senderUID, receiverUID int64, requestID string) error {
	_, err := s.SendNotificationToUser(senderUID, "friend_request", map[string]interface{}{
		"request_id":   requestID,
		"receiver_uid": receiverUID,
		"action":       "accepted",
	})
	return err
}

func (s *Service) SendGroupInviteNotification(uid int64, groupID, groupName string, inviterUID int64) error {
	_, err := s.SendNotificationToUser(uid, "group_invite", map[string]interface{}{
		"group_id":    groupID,
		"group_name":  groupName,
		"inviter_uid": inviterUID,
		"action":      "invited",
	})
	return err
}
