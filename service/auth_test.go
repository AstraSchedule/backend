package service

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("test123")
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.NotEqual(t, "test123", hash, "密码不能明文存储")

	assert.True(t, CheckPassword("test123", hash))
	assert.False(t, CheckPassword("wrong", hash))
}

func TestGenerateToken_ParseTokenRoundTrip(t *testing.T) {
	token, err := GenerateToken("test-secret", 42, "ns-a", "admin", "admin", "ALL", 1)
	require.NoError(t, err)

	claims, err := ParseToken("test-secret", token)
	require.NoError(t, err)
	assert.Equal(t, uint(42), claims.UserID)
	assert.Equal(t, "ns-a", claims.Namespace)
	assert.Equal(t, "admin", claims.Username)
	assert.Equal(t, "admin", claims.Role)
	assert.Equal(t, "ALL", claims.Scope)
}

func TestParseToken_WrongSecret(t *testing.T) {
	token, err := GenerateToken("test-secret", 1, "", "admin", "admin", "", 1)
	require.NoError(t, err)

	_, err = ParseToken("other-secret", token)
	assert.Error(t, err, "错误密钥必须解析失败")
}

func TestParseToken_Garbage(t *testing.T) {
	_, err := ParseToken("test-secret", "not-a-token")
	assert.Error(t, err)
}

func TestParseToken_Expired(t *testing.T) {
	// 手工构造 exp 已过的令牌（GenerateToken 对非正时长会回退 24 小时，无法直接生成过期令牌）
	claims := JWTClaims{
		UserID:    1,
		Namespace: "ns-a",
		Username:  "admin",
		Role:      "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test-secret"))
	require.NoError(t, err)

	_, err = ParseToken("test-secret", token)
	assert.Error(t, err, "过期令牌必须解析失败")
}
