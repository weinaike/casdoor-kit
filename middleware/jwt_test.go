package middleware

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

var (
	testPrivKey *rsa.PrivateKey
	testPubFile string
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)

	// Generate a single test RSA key pair shared by all tests
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate RSA key: %v\n", err)
		os.Exit(1)
	}
	testPrivKey = privKey

	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal public key: %v\n", err)
		os.Exit(1)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubKeyBytes})

	tmpFile, err := os.CreateTemp("", "jwt_test_pub_*.pem")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create temp file: %v\n", err)
		os.Exit(1)
	}
	if _, err := tmpFile.Write(pubPEM); err != nil {
		fmt.Fprintf(os.Stderr, "write temp file: %v\n", err)
		os.Exit(1)
	}
	tmpFile.Close()
	testPubFile = tmpFile.Name()

	// Initialize the public key once (matches sync.Once behavior)
	JWTAuth(testPubFile)

	code := m.Run()
	os.Remove(testPubFile)
	os.Exit(code)
}

// signTestJWT creates a signed JWT with the test private key.
func signTestJWT(claims jwt.MapClaims) string {
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	s, _ := token.SignedString(testPrivKey)
	return s
}

func TestGetUserID_Present(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(string(UserIDKey), "user-123")

	got := GetUserID(c)
	if got != "user-123" {
		t.Errorf("GetUserID = %q, want %q", got, "user-123")
	}
}

func TestGetUserID_Absent(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	got := GetUserID(c)
	if got != "" {
		t.Errorf("GetUserID = %q, want empty", got)
	}
}

func TestJWTAuth_ValidToken(t *testing.T) {
	handler := JWTAuth(testPubFile)

	token := signTestJWT(jwt.MapClaims{
		"user_id": "user-abc",
		"exp":     time.Now().Add(time.Hour).Unix(),
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler(c)

	if c.IsAborted() {
		t.Error("handler should not abort for valid token")
	}

	userID, _ := c.Get(string(UserIDKey))
	if userID != "user-abc" {
		t.Errorf("user_id in context = %v, want %q", userID, "user-abc")
	}
}

func TestJWTAuth_MissingHeader(t *testing.T) {
	handler := JWTAuth(testPubFile)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler(c)

	if !c.IsAborted() {
		t.Error("handler should abort for missing header")
	}
	if w.Code != 401 {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestJWTAuth_InvalidFormat(t *testing.T) {
	handler := JWTAuth(testPubFile)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Basic abc123")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler(c)

	if !c.IsAborted() {
		t.Error("handler should abort for invalid format")
	}
	if w.Code != 401 {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestJWTAuth_ExpiredToken(t *testing.T) {
	handler := JWTAuth(testPubFile)

	token := signTestJWT(jwt.MapClaims{
		"user_id": "user-exp",
		"exp":     time.Now().Add(-time.Hour).Unix(),
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler(c)

	if !c.IsAborted() {
		t.Error("handler should abort for expired token")
	}
	if w.Code != 401 {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestJWTAuth_WrongSigningMethod(t *testing.T) {
	handler := JWTAuth(testPubFile)

	// Sign with HMAC instead of RSA
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": "user-hmac",
		"exp":     time.Now().Add(time.Hour).Unix(),
	})
	tokenString, _ := token.SignedString([]byte("secret"))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tokenString))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler(c)

	if !c.IsAborted() {
		t.Error("handler should abort for wrong signing method")
	}
	if w.Code != 401 {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestParseTokenFromQueryString(t *testing.T) {
	token := signTestJWT(jwt.MapClaims{
		"user_id": "user-sse",
		"exp":     time.Now().Add(time.Hour).Unix(),
	})

	userID, err := ParseTokenFromQueryString(token)
	if err != nil {
		t.Fatalf("ParseTokenFromQueryString error: %v", err)
	}
	if userID != "user-sse" {
		t.Errorf("userID = %q, want %q", userID, "user-sse")
	}
}
