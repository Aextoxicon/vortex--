package main

import "errors"

// ==================== 会话 ID 解析（集中管理） ====================

// PrivateConvID 生成私聊会话 ID，格式为 "p_{较小的 publicID}_{较大的 publicID}"
func PrivateConvID(publicID1, publicID2 string) string {
	if publicID1 < publicID2 {
		return "p_" + publicID1 + "_" + publicID2
	}
	return "p_" + publicID2 + "_" + publicID1
}

// IsPrivateConv 检测是否为私聊会话
func IsPrivateConv(convID string) bool {
	return len(convID) > 0 && convID[0] == 'p'
}

// IsGroupConv 检测是否为群聊会话
func IsGroupConv(convID string) bool {
	return len(convID) >= 2 && convID[:2] == "g_"
}

// ExtractConversationType 提取会话类型前缀（"p" 或 "g"）
func ExtractConversationType(convID string) string {
	if len(convID) == 0 {
		return ""
	}
	if convID[0] == 'p' {
		return "p"
	}
	if convID[0] == 'g' {
		return "g"
	}
	return ""
}

// ParsePrivateConv 解析私聊会话 ID，返回两个 publicID
func ParsePrivateConv(convID string) (string, string, error) {
	if !IsPrivateConv(convID) {
		return "", "", errors.New("not a private conversation")
	}
	if len(convID) < 3 || convID[0] != 'p' || convID[1] != '_' {
		return "", "", errors.New("invalid private conversation format")
	}

	rest := convID[2:] // 跳过 "p_"
	lastUnderscore := -1
	for i := len(rest) - 1; i >= 0; i-- {
		if rest[i] == '_' {
			lastUnderscore = i
			break
		}
	}

	if lastUnderscore == -1 {
		return "", "", errors.New("invalid private conversation format")
	}

	return rest[:lastUnderscore], rest[lastUnderscore+1:], nil
}

// CanAccessPrivateConv 检查用户是否有权访问私聊会话
func CanAccessPrivateConv(convID string, publicID string) bool {
	a, b, err := ParsePrivateConv(convID)
	if err != nil {
		return false
	}
	return a == publicID || b == publicID
}

// GetOtherPublicID 获取私聊会话中的另一方 publicID
func GetOtherPublicID(convID string, myPublicID string) string {
	if !IsPrivateConv(convID) {
		return ""
	}
	a, b, err := ParsePrivateConv(convID)
	if err != nil {
		return ""
	}
	if a == myPublicID {
		return b
	}
	if b == myPublicID {
		return a
	}
	return ""
}

// ExtractGroupID 从群聊会话 ID 中提取群组 ID（去掉 "g_" 前缀）
func ExtractGroupID(convID string) string {
	if IsGroupConv(convID) {
		return convID[2:]
	}
	return ""
}
