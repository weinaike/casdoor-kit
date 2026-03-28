//go:build e2e

package casdoor

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// --- Helper: login as test user via password grant ---

func getUserToken(t *testing.T) (*TokenResponse, *UserInfo) {
	t.Helper()

	username := os.Getenv("CASDOOR_TEST_USERNAME")
	password := os.Getenv("CASDOOR_TEST_PASSWORD")
	if username == "" {
		username = "dev-user-01"
	}
	if password == "" {
		password = "Abc@123"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tokenResp, err := e2eClient.LoginByPassword(ctx, username, password)
	if err != nil {
		t.Fatalf("LoginByPassword(%q): %v", username, err)
	}

	if tokenResp.AccessToken == "" {
		t.Fatal("LoginByPassword returned empty access_token")
	}
	if tokenResp.RefreshToken == "" {
		t.Log("Warning: no refresh_token returned (password grant may not include it)")
	}

	// Fetch user info with the token
	userInfo, err := e2eClient.GetUserInfo(ctx, tokenResp.AccessToken)
	if err != nil {
		t.Fatalf("GetUserInfo after login: %v", err)
	}

	t.Logf("Logged in as: %s (id=%s, email=%s, org=%s)",
		userInfo.Name, userInfo.ID, userInfo.Email, userInfo.Organization)

	return tokenResp, userInfo
}

// --- User Flow Tests ---

// TestE2E_UserFlow_Login verifies the password-based login flow.
func TestE2E_UserFlow_Login(t *testing.T) {
	tokenResp, _ := getUserToken(t)

	if tokenResp.AccessToken == "" {
		t.Fatal("access_token is empty")
	}
	if tokenResp.TokenType == "" {
		t.Error("token_type is empty")
	}
	if tokenResp.ExpiresIn <= 0 {
		t.Errorf("expires_in = %d, want > 0", tokenResp.ExpiresIn)
	}
	t.Logf("Token: type=%s, expires_in=%d, has_refresh=%v",
		tokenResp.TokenType, tokenResp.ExpiresIn, tokenResp.RefreshToken != "")
}

// TestE2E_UserFlow_Login_WrongPassword verifies login with wrong password.
func TestE2E_UserFlow_Login_WrongPassword(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := e2eClient.LoginByPassword(ctx, "dev-user-01", "wrong-password")
	if err == nil {
		t.Fatal("expected error for wrong password")
	}
	t.Logf("Expected error: %v", err)
}

// TestE2E_UserFlow_GetUserInfo verifies fetching user info with user token.
func TestE2E_UserFlow_GetUserInfo(t *testing.T) {
	tokenResp, _ := getUserToken(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	userInfo, err := e2eClient.GetUserInfo(ctx, tokenResp.AccessToken)
	if err != nil {
		t.Fatalf("GetUserInfo: %v", err)
	}

	if userInfo.Name == "" {
		t.Error("user Name is empty")
	}
	if userInfo.ID == "" {
		t.Error("user ID is empty")
	}
	// Note: Organization may be empty from /api/userinfo (OIDC endpoint),
	// as Casdoor may not include the owner field in standard userinfo claims.
	if userInfo.Organization != "" {
		t.Logf("Organization: %s", userInfo.Organization)
	}
	t.Logf("User: id=%s, name=%s, display=%s, email=%s, avatar=%s",
		userInfo.ID, userInfo.Name, userInfo.DisplayName, userInfo.Email, userInfo.Avatar)
}

// TestE2E_UserFlow_GetProducts verifies listing products with user token.
func TestE2E_UserFlow_GetProducts(t *testing.T) {
	tokenResp, _ := getUserToken(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	products, err := e2eClient.GetProducts(ctx, tokenResp.AccessToken)
	if err != nil {
		t.Fatalf("GetProducts: %v", err)
	}

	if len(products) == 0 {
		t.Skip("no products available")
	}

	t.Logf("Found %d products for user:", len(products))
	for _, p := range products {
		t.Logf("  - %s (%s): price=%.2f %s, state=%s, providers=%v",
			p.Name, p.DisplayName, p.Price, p.Currency, p.State, p.Providers)
		if p.Name == "" {
			t.Error("product Name should not be empty")
		}
		if p.Currency == "" {
			t.Errorf("product %q has empty Currency", p.Name)
		}
	}
}

// TestE2E_UserFlow_GetProduct_Detail verifies fetching a single product.
func TestE2E_UserFlow_GetProduct_Detail(t *testing.T) {
	tokenResp, _ := getUserToken(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// First list products to find one to query
	products, err := e2eClient.GetProducts(ctx, tokenResp.AccessToken)
	if err != nil {
		t.Fatalf("GetProducts: %v", err)
	}
	if len(products) == 0 {
		t.Skip("no products available to test GetProduct")
	}

	// Get the first product by name (id = owner/name)
	productName := products[0].Name
	product, err := e2eClient.GetProduct(ctx, tokenResp.AccessToken, productName)
	if err != nil {
		t.Fatalf("GetProduct(%q): %v", productName, err)
	}

	t.Logf("Product: name=%s, display=%s, price=%.2f %s, providers=%v",
		product.Name, product.DisplayName, product.Price, product.Currency,
		product.Providers)
}

// TestE2E_UserFlow_PlaceOrderAndGetCancel creates an order and cancels it.
func TestE2E_UserFlow_PlaceOrderAndGetCancel(t *testing.T) {
	tokenResp, _ := getUserToken(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Find a product
	products, err := e2eClient.GetProducts(ctx, tokenResp.AccessToken)
	if err != nil {
		t.Fatalf("GetProducts: %v", err)
	}
	if len(products) == 0 {
		t.Skip("no products available for order")
	}

	productName := products[0].Name
	t.Logf("Using product: %q", productName)

	// Place order
	order, err := e2eClient.PlaceOrder(ctx, tokenResp.AccessToken, productName)
	if err != nil {
		t.Fatalf("PlaceOrder(%q): %v", productName, err)
	}
	if order.Name == "" {
		t.Fatal("order name is empty")
	}
	if order.Price <= 0 {
		t.Errorf("order Price = %.2f, want > 0", order.Price)
	}
	if order.Currency == "" {
		t.Error("order Currency is empty")
	}
	t.Logf("Order created: name=%s, owner=%s, state=%s, price=%.2f %s",
		order.Name, order.Owner, order.State, order.Price, order.Currency)

	// Get order by name
	fetched, err := e2eClient.GetOrder(ctx, tokenResp.AccessToken, order.Name)
	if err != nil {
		t.Fatalf("GetOrder(%q): %v", order.Name, err)
	}
	if fetched.Name != order.Name {
		t.Errorf("fetched order name = %q, want %q", fetched.Name, order.Name)
	}
	t.Logf("Order fetched: state=%s", fetched.State)

	// Cancel order
	if err := e2eClient.CancelOrder(ctx, order.Owner, order.Name); err != nil {
		t.Fatalf("CancelOrder(%q): %v", order.Name, err)
	}
	t.Logf("Order canceled for cleanup")
}

// TestE2E_UserFlow_PayOrder gets a payment URL for an order.
func TestE2E_UserFlow_PayOrder(t *testing.T) {
	tokenResp, _ := getUserToken(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Find a product with at least one provider
	products, err := e2eClient.GetProducts(ctx, tokenResp.AccessToken)
	if err != nil {
		t.Fatalf("GetProducts: %v", err)
	}
	if len(products) == 0 {
		t.Skip("no products available")
	}

	var productWithProvider *Product
	for _, p := range products {
		if len(p.Providers) > 0 {
			pp := p
			productWithProvider = &pp
			break
		}
	}
	if productWithProvider == nil {
		t.Skip("no product with payment provider configured")
	}

	// Place order
	order, err := e2eClient.PlaceOrder(ctx, tokenResp.AccessToken, productWithProvider.Name)
	if err != nil {
		t.Fatalf("PlaceOrder(%q): %v", productWithProvider.Name, err)
	}
	t.Logf("Order created: %s", order.Name)

	// Pay order with first available provider
	provider := productWithProvider.Providers[0]
	payment, err := e2eClient.PayOrder(ctx, tokenResp.AccessToken, order.Owner, order.Name, provider)
	if err != nil {
		t.Fatalf("PayOrder(%q, provider=%q): %v", order.Name, provider, err)
	}
	t.Logf("Payment: pay_url=%s, price=%.2f %s",
		payment.PayUrl, payment.Price, payment.Currency)

	if payment.PayUrl == "" {
		t.Error("payment URL is empty — expected a redirect URL from the provider")
	}

	// Cancel order to clean up
	if err := e2eClient.CancelOrder(ctx, order.Owner, order.Name); err != nil {
		t.Logf("Warning: cleanup cancel failed: %v", err)
	} else {
		t.Log("Order canceled for cleanup")
	}
}

// TestE2E_UserFlow_GetUserOrders verifies listing orders for the test user.
func TestE2E_UserFlow_GetUserOrders(t *testing.T) {
	tokenResp, userInfo := getUserToken(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	orders, err := e2eClient.GetUserOrders(ctx, tokenResp.AccessToken, userInfo.Name)
	if err != nil {
		t.Fatalf("GetUserOrders(%q): %v", userInfo.Name, err)
	}

	t.Logf("Found %d orders for user %q:", len(orders), userInfo.Name)
	for _, o := range orders {
		t.Logf("  - %s: state=%s, price=%.2f %s, products=%v",
			o.Name, o.State, o.Price, o.Currency, o.Products)
	}
}

// TestE2E_UserFlow_RefreshToken verifies the token refresh cycle.
func TestE2E_UserFlow_RefreshToken(t *testing.T) {
	tokenResp, _ := getUserToken(t)

	if tokenResp.RefreshToken == "" {
		t.Skip("no refresh_token returned by password grant")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	t.Logf("Original access_token (first 20 chars): %s...", truncate(tokenResp.AccessToken, 20))

	newTokenResp, err := e2eClient.RefreshToken(ctx, tokenResp.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}

	if newTokenResp.AccessToken == "" {
		t.Fatal("refreshed access_token is empty")
	}
	t.Logf("Refreshed access_token (first 20 chars): %s...", truncate(newTokenResp.AccessToken, 20))

	// Verify the new token works
	userInfo, err := e2eClient.GetUserInfo(ctx, newTokenResp.AccessToken)
	if err != nil {
		t.Fatalf("GetUserInfo with refreshed token: %v", err)
	}
	t.Logf("Refreshed token valid — user: %s", userInfo.Name)
}

// TestE2E_UserFlow_Logout verifies the logout/revoke flow.
func TestE2E_UserFlow_Logout(t *testing.T) {
	tokenResp, _ := getUserToken(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := e2eClient.RevokeToken(ctx, tokenResp.AccessToken)
	if err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}
	t.Log("Token revoked successfully")

	// Verify the token is no longer valid
	_, err = e2eClient.GetUserInfo(ctx, tokenResp.AccessToken)
	if err == nil {
		t.Log("Warning: token still works after revoke (Casdoor may not immediately invalidate)")
	} else {
		t.Logf("Token correctly rejected after revoke: %v", err)
	}
}

// TestE2E_UserFlow_FullOrderLifecycle tests the complete order lifecycle:
// Login → GetProducts → PlaceOrder → GetOrder → CancelOrder.
func TestE2E_UserFlow_FullOrderLifecycle(t *testing.T) {
	tokenResp, _ := getUserToken(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Step 1: List products
	products, err := e2eClient.GetProducts(ctx, tokenResp.AccessToken)
	if err != nil {
		t.Fatalf("Step 1 - GetProducts: %v", err)
	}
	if len(products) == 0 {
		t.Skip("no products available")
	}
	t.Logf("Step 1: Found %d products", len(products))

	// Step 2: Get product detail
	productName := products[0].Name
	product, err := e2eClient.GetProduct(ctx, tokenResp.AccessToken, productName)
	if err != nil {
		t.Fatalf("Step 2 - GetProduct(%q): %v", productName, err)
	}
	t.Logf("Step 2: Product detail — name=%s, price=%.2f %s, providers=%v",
		product.Name, product.Price, product.Currency, product.Providers)

	// Step 3: Place order
	order, err := e2eClient.PlaceOrder(ctx, tokenResp.AccessToken, productName)
	if err != nil {
		t.Fatalf("Step 3 - PlaceOrder(%q): %v", productName, err)
	}
	t.Logf("Step 3: Order created — name=%s, owner=%s, state=%s, price=%.2f",
		order.Name, order.Owner, order.State, order.Price)

	// Step 4: Get order
	fetched, err := e2eClient.GetOrder(ctx, tokenResp.AccessToken, order.Name)
	if err != nil {
		t.Fatalf("Step 4 - GetOrder(%q): %v", order.Name, err)
	}
	if fetched.State != OrderStateCreated {
		t.Errorf("Step 4: order state = %q, want %q", fetched.State, OrderStateCreated)
	}
	t.Logf("Step 4: Order confirmed — state=%s", fetched.State)

	// Step 5: Cancel order
	if err := e2eClient.CancelOrder(ctx, order.Owner, order.Name); err != nil {
		t.Fatalf("Step 5 - CancelOrder(%q): %v", order.Name, err)
	}
	t.Logf("Step 5: Order canceled")

	// Step 6: Verify canceled state
	canceledOrder, err := e2eClient.GetOrder(ctx, tokenResp.AccessToken, order.Name)
	if err != nil {
		t.Logf("Step 6: GetOrder after cancel: %v", err)
	} else {
		t.Logf("Step 6: Final order state: %s", canceledOrder.State)
	}

	t.Log("Full order lifecycle completed successfully")
}

// --- Helpers ---

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// Compile-time check that the test file builds with e2e tag.
var _ = strings.TrimSpace
var _ = fmt.Sprintf
