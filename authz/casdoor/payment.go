package casdoor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// GetProducts fetches the product list from Casdoor.
func (c *Client) GetProducts(ctx context.Context, accessToken string) ([]Product, error) {
	apiURL := fmt.Sprintf("%s/api/get-products?owner=%s",
		strings.TrimSuffix(c.cfg.Endpoint, "/"),
		c.cfg.Organization)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("获取产品列表失败: status %d", resp.StatusCode)
	}

	var response struct {
		Data []Product `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	return response.Data, nil
}

// GetProduct fetches a single product from Casdoor.
func (c *Client) GetProduct(ctx context.Context, accessToken string, productName string) (*Product, error) {
	apiURL := fmt.Sprintf("%s/api/get-product?id=%s/%s",
		strings.TrimSuffix(c.cfg.Endpoint, "/"),
		c.cfg.Organization,
		productName)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("获取产品失败: status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var product Product
	if err := json.Unmarshal(bodyBytes, &product); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	return &product, nil
}

// PlaceOrder creates an order in Casdoor.
func (c *Client) PlaceOrder(ctx context.Context, accessToken string, productName string) (*Order, error) {
	product, err := c.GetProduct(ctx, accessToken, productName)
	if err != nil {
		return nil, fmt.Errorf("获取产品信息失败: %w", err)
	}

	productInfo := PlaceOrderProductInfo{
		Name:     productName,
		Quantity: 1,
	}

	if product.IsRecharge {
		productInfo.Price = product.Price
	}

	apiURL := fmt.Sprintf("%s/api/place-order?owner=%s",
		strings.TrimSuffix(c.cfg.Endpoint, "/"),
		c.cfg.Organization)

	reqBody := PlaceOrderRequest{
		ProductInfos: []PlaceOrderProductInfo{productInfo},
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("创建订单失败: status %d, %s", resp.StatusCode, string(errBody))
	}

	respBody, _ := io.ReadAll(resp.Body)

	var response struct {
		Status string `json:"status"`
		Msg    string `json:"msg"`
		Data   Order  `json:"data"`
	}
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if response.Status != "ok" {
		return nil, fmt.Errorf("创建订单失败: %s", response.Msg)
	}

	return &response.Data, nil
}

// PayOrder initiates payment for an order.
func (c *Client) PayOrder(ctx context.Context, accessToken string, orderOwner string, orderName string, provider string) (*Payment, error) {
	apiURL := fmt.Sprintf("%s/api/pay-order?id=%s/%s&providerName=%s",
		strings.TrimSuffix(c.cfg.Endpoint, "/"),
		orderOwner,
		orderName,
		provider)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("支付订单失败: status %d, %s", resp.StatusCode, string(errBody))
	}

	respBody, _ := io.ReadAll(resp.Body)

	var response struct {
		Status string                 `json:"status"`
		Msg    string                 `json:"msg"`
		Data   Payment                `json:"data"`
		Data2  map[string]interface{} `json:"data2"`
	}
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if response.Status != "ok" {
		return nil, fmt.Errorf("支付失败: %s", response.Msg)
	}

	payment := response.Data
	if response.Data2 != nil {
		if payUrl, ok := response.Data2["payUrl"].(string); ok && payUrl != "" {
			payment.PayUrl = payUrl
		}
	}

	return &payment, nil
}

// GetUserOrders fetches user orders from Casdoor.
func (c *Client) GetUserOrders(ctx context.Context, accessToken string, userName string) ([]Order, error) {
	apiURL := fmt.Sprintf("%s/api/get-user-orders?owner=%s&user=%s",
		strings.TrimSuffix(c.cfg.Endpoint, "/"),
		c.cfg.Organization,
		userName)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取订单列表失败: status %d, %s", resp.StatusCode, string(errBody))
	}

	respBody, _ := io.ReadAll(resp.Body)

	var response struct {
		Status string  `json:"status"`
		Msg    string  `json:"msg"`
		Data   []Order `json:"data"`
	}
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if response.Status != "ok" {
		return nil, fmt.Errorf("获取订单列表失败: %s", response.Msg)
	}

	return response.Data, nil
}

// GetOrder fetches a single order from Casdoor.
func (c *Client) GetOrder(ctx context.Context, accessToken string, orderName string) (*Order, error) {
	apiURL := fmt.Sprintf("%s/api/get-order?id=%s/%s",
		strings.TrimSuffix(c.cfg.Endpoint, "/"),
		c.cfg.Organization,
		orderName)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("获取订单失败: status %d", resp.StatusCode)
	}

	var response struct {
		Status string `json:"status"`
		Msg    string `json:"msg"`
		Data   Order  `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if response.Status == "error" {
		return nil, fmt.Errorf("获取订单失败: %s", response.Msg)
	}

	if response.Data.Name == "" {
		return nil, fmt.Errorf("订单不存在")
	}

	return &response.Data, nil
}

// NotifyPayment notifies Casdoor of a payment.
func (c *Client) NotifyPayment(ctx context.Context, owner string, paymentName string) error {
	apiURL := fmt.Sprintf("%s/api/notify-payment/%s/%s",
		strings.TrimSuffix(c.cfg.Endpoint, "/"),
		owner,
		paymentName)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("支付通知失败: status %d", resp.StatusCode)
	}

	return nil
}

// CancelOrder cancels an order in Casdoor using Basic Auth.
func (c *Client) CancelOrder(ctx context.Context, orderOwner string, orderName string) error {
	apiURL := fmt.Sprintf("%s/api/cancel-order?id=%s/%s",
		strings.TrimSuffix(c.cfg.Endpoint, "/"),
		orderOwner,
		orderName)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	req.SetBasicAuth(c.cfg.ClientID, c.cfg.ClientSecret)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("取消订单失败: status %d, %s", resp.StatusCode, string(errBody))
	}

	respBody, _ := io.ReadAll(resp.Body)

	var response struct {
		Status string `json:"status"`
		Msg    string `json:"msg"`
	}
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil
	}

	if response.Status == "error" {
		return fmt.Errorf("取消订单失败: %s", response.Msg)
	}

	return nil
}
