package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

func newTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c, w
}

func parseBody(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}
	return body
}

func TestSuccess(t *testing.T) {
	c, w := newTestContext()
	Success(c, map[string]string{"name": "test"})

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusOK)
	}
	body := parseBody(t, w)
	if code, ok := body["code"].(float64); !ok || code != CodeSuccess {
		t.Errorf("code: got %v, want %d", body["code"], CodeSuccess)
	}
	if body["data"] == nil {
		t.Error("data should not be nil")
	}
}

func TestCreated(t *testing.T) {
	c, w := newTestContext()
	Created(c, "created-item")

	if w.Code != http.StatusCreated {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusCreated)
	}
	body := parseBody(t, w)
	if code, ok := body["code"].(float64); !ok || code != CodeSuccess {
		t.Errorf("code: got %v, want %d", body["code"], CodeSuccess)
	}
}

func TestBadRequest(t *testing.T) {
	c, w := newTestContext()
	BadRequest(c, "invalid input")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusBadRequest)
	}
	body := parseBody(t, w)
	if code, ok := body["code"].(float64); !ok || code != CodeBadRequest {
		t.Errorf("code: got %v, want %d", body["code"], CodeBadRequest)
	}
	if msg, ok := body["message"].(string); !ok || msg != "invalid input" {
		t.Errorf("message: got %v, want 'invalid input'", body["message"])
	}
}

func TestUnauthorized(t *testing.T) {
	c, w := newTestContext()
	Unauthorized(c, "not allowed")

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
	body := parseBody(t, w)
	if code, ok := body["code"].(float64); !ok || code != CodeUnauthorized {
		t.Errorf("code: got %v, want %d", body["code"], CodeUnauthorized)
	}
}

func TestInternalError(t *testing.T) {
	c, w := newTestContext()
	InternalError(c, "something broke")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusInternalServerError)
	}
	body := parseBody(t, w)
	if code, ok := body["code"].(float64); !ok || code != CodeInternalError {
		t.Errorf("code: got %v, want %d", body["code"], CodeInternalError)
	}
}

func TestBindError(t *testing.T) {
	c, w := newTestContext()
	BindError(c, errFake{"field required"})

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusBadRequest)
	}
	body := parseBody(t, w)
	msg, _ := body["message"].(string)
	want := "参数错误: field required"
	if msg != want {
		t.Errorf("message: got %q, want %q", msg, want)
	}
}

type errFake struct{ msg string }

func (e errFake) Error() string { return e.msg }

func TestSuccessWithTotal(t *testing.T) {
	c, w := newTestContext()
	SuccessWithTotal(c, []string{"a", "b"}, 2)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusOK)
	}
	body := parseBody(t, w)
	if code, ok := body["code"].(float64); !ok || code != CodeSuccess {
		t.Errorf("code: got %v, want %d", body["code"], CodeSuccess)
	}

	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatal("data is not a map")
	}
	if data["list"] == nil {
		t.Error("data.list should not be nil")
	}
	if total, ok := data["total"].(float64); !ok || total != 2 {
		t.Errorf("data.total: got %v, want 2", data["total"])
	}
}

func TestConstants(t *testing.T) {
	tests := []struct {
		name  string
		value int
		want  int
	}{
		{"CodeSuccess", CodeSuccess, 0},
		{"CodeBadRequest", CodeBadRequest, 400},
		{"CodeUnauthorized", CodeUnauthorized, 401},
		{"CodeForbidden", CodeForbidden, 403},
		{"CodeNotFound", CodeNotFound, 404},
		{"CodePaymentRequired", CodePaymentRequired, 402},
		{"CodeInternalError", CodeInternalError, 500},
	}
	for _, tt := range tests {
		if tt.value != tt.want {
			t.Errorf("%s: got %d, want %d", tt.name, tt.value, tt.want)
		}
	}
}
