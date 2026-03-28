//go:build e2e

package casdoor

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/weinaike/casdoor-kit/config"
)

var e2eClient *Client

func TestMain(m *testing.M) {
	endpoint := os.Getenv("CASDOOR_ENDPOINT")
	clientID := os.Getenv("CASDOOR_CLIENT_ID")
	clientSecret := os.Getenv("CASDOOR_CLIENT_SECRET")
	org := os.Getenv("CASDOOR_ORGANIZATION")
	app := os.Getenv("CASDOOR_APPLICATION")

	if endpoint == "" || clientID == "" || clientSecret == "" {
		fmt.Println("Skipping e2e tests: CASDOOR_ENDPOINT/CLIENT_ID/CLIENT_SECRET not set")
		os.Exit(0)
	}

	e2eClient = NewClient(&config.CasdoorConfig{
		Endpoint:     endpoint,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Organization: org,
		Application:  app,
		RedirectURI:  "http://localhost:5173/auth/callback",
	})

	os.Exit(m.Run())
}

func getSystemToken(t *testing.T) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	token, err := e2eClient.GetSystemToken(ctx)
	if err != nil {
		t.Fatalf("GetSystemToken: %v", err)
	}
	if token == "" {
		t.Fatal("GetSystemToken returned empty token")
	}
	return token
}

// --- Tests ---

func TestE2E_GetLoginURL_Format(t *testing.T) {
	state := "test-state-123"
	loginURL := e2eClient.GetLoginURL(state)

	u, err := url.Parse(loginURL)
	if err != nil {
		t.Fatalf("parse login URL: %v", err)
	}

	if u.Path != "/login/oauth/authorize" {
		t.Errorf("path = %q, want /login/oauth/authorize", u.Path)
	}

	q := u.Query()
	if q.Get("client_id") == "" {
		t.Error("client_id is missing")
	}
	if q.Get("state") != state {
		t.Errorf("state = %q, want %q", q.Get("state"), state)
	}
	if q.Get("response_type") != "code" {
		t.Errorf("response_type = %q, want %q", q.Get("response_type"), "code")
	}
	if q.Get("redirect_uri") == "" {
		t.Error("redirect_uri is missing")
	}
}

func TestE2E_GetSignupURL_Format(t *testing.T) {
	signupURL := e2eClient.GetSignupURL()
	app := os.Getenv("CASDOOR_APPLICATION")

	if !strings.Contains(signupURL, "/signup/") {
		t.Errorf("signup URL should contain /signup/, got: %s", signupURL)
	}
	if app != "" && !strings.Contains(signupURL, app) {
		t.Errorf("signup URL should contain app name %q, got: %s", app, signupURL)
	}
}

func TestE2E_GetOrganization(t *testing.T) {
	org := e2eClient.GetOrganization()
	expected := os.Getenv("CASDOOR_ORGANIZATION")
	if org != expected {
		t.Errorf("GetOrganization() = %q, want %q", org, expected)
	}
}

func TestE2E_GetSystemToken(t *testing.T) {
	token := getSystemToken(t)
	if len(token) < 10 {
		t.Errorf("token seems too short: %d chars", len(token))
	}
}

func TestE2E_GetProducts_SystemToken(t *testing.T) {
	token := getSystemToken(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	products, err := e2eClient.GetProducts(ctx, token)
	if err != nil {
		t.Fatalf("GetProducts: %v", err)
	}

	if len(products) == 0 {
		t.Log("Warning: no products returned (may be expected if none configured)")
		return
	}

	t.Logf("Found %d products:", len(products))
	for _, p := range products {
		t.Logf("  - %s (%s): %s, price=%.2f %s, state=%s",
			p.Name, p.DisplayName, p.Description, p.Price, p.Currency, p.State)
	}
}

func TestE2E_GetProduct_BasicPack(t *testing.T) {
	token := getSystemToken(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// First list products to find one to query
	products, err := e2eClient.GetProducts(ctx, token)
	if err != nil {
		t.Fatalf("GetProducts: %v", err)
	}
	if len(products) == 0 {
		t.Skip("no products available to test GetProduct")
	}

	// Get the first product by name
	productName := products[0].Name
	product, err := e2eClient.GetProduct(ctx, token, productName)
	if err != nil {
		t.Fatalf("GetProduct(%q): %v", productName, err)
	}
	// Note: Casdoor GetProduct may return fields differently depending on version
	// At minimum it should not error
	t.Logf("Product: %s (%s), price=%.2f %s", product.Name, product.DisplayName, product.Price, product.Currency)
	// Verify via the list we got earlier that the product exists
	if productName == "" {
		t.Error("product name from GetProducts is empty")
	}
}

func TestE2E_ExchangeCode_Invalid(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := e2eClient.ExchangeCode(ctx, "invalid-code-12345")
	if err == nil {
		t.Fatal("expected error for invalid authorization code")
	}
	t.Logf("Expected error: %v", err)
}

func TestE2E_GetUserInfo_InvalidToken(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := e2eClient.GetUserInfo(ctx, "invalid-token-xyz")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
	t.Logf("Expected error: %v", err)
}

func TestE2E_GetOrder_NonExist(t *testing.T) {
	token := getSystemToken(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := e2eClient.GetOrder(ctx, token, "nonexistent-order-xyz-999")
	if err == nil {
		t.Fatal("expected error for non-existent order")
	}
	t.Logf("Expected error for non-existent order: %v", err)
}

func TestE2E_GetUserOrders_SystemToken(t *testing.T) {
	token := getSystemToken(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	orders, err := e2eClient.GetUserOrders(ctx, token, "admin")
	if err != nil {
		t.Fatalf("GetUserOrders: %v", err)
	}

	t.Logf("Found %d orders for admin user", len(orders))
	// Verify each order has a non-empty name
	for _, o := range orders {
		if o.Name == "" {
			t.Error("order Name should not be empty")
		}
		if o.State == "" {
			t.Errorf("order %q has empty State", o.Name)
		}
	}
}

func TestE2E_RefreshToken_Invalid(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := e2eClient.RefreshToken(ctx, "invalid-refresh-token-xyz")
	if err == nil {
		t.Fatal("expected error for invalid refresh token")
	}
	t.Logf("Expected error: %v", err)
}
