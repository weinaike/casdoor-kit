package casdoor

// TokenResponse represents an OAuth2 token response from Casdoor.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
}

// UserInfo represents Casdoor user info (OIDC standard userinfo fields).
type UserInfo struct {
	ID           string `json:"sub"`
	Name         string `json:"preferred_username"`
	DisplayName  string `json:"name"`
	Email        string `json:"email"`
	Phone        string `json:"phone"`
	Avatar       string `json:"picture"`
	Organization string `json:"owner"`
}

// SubscriptionInfo represents Casdoor subscription information.
type SubscriptionInfo struct {
	PlanID    string `json:"plan_id"`
	PlanName  string `json:"plan_name"`
	Status    string `json:"status"`
	StartTime int64  `json:"start_time"`
	EndTime   int64  `json:"end_time"`
}

// PaymentRecord represents a payment history record.
type PaymentRecord struct {
	ID        string  `json:"id"`
	Amount    float64 `json:"amount"`
	Currency  string  `json:"currency"`
	Status    string  `json:"status"`
	CreatedAt int64   `json:"created_at"`
	PlanName  string  `json:"plan_name"`
}

// Casdoor product/order/payment state constants
const (
	ProductStatePublished = "Published"

	OrderStateCreated  = "Created"
	OrderStatePaid     = "Paid"
	OrderStateCanceled = "Canceled"
	OrderStatePending  = "Pending"

	PaymentStateCreated  = "Created"
	PaymentStatePaid     = "Paid"
	PaymentStateCanceled = "Canceled"
)

// Product represents a Casdoor product.
type Product struct {
	Owner               string    `json:"owner"`
	Name                string    `json:"name"`
	CreatedTime         string    `json:"createdTime"`
	DisplayName         string    `json:"displayName"`
	Image               string    `json:"image"`
	Detail              string    `json:"detail"`
	Description         string    `json:"description"`
	Tag                 string    `json:"tag"`
	Currency            string    `json:"currency"`
	Price               float64   `json:"price"`
	Quantity            int       `json:"quantity"`
	Sold                int       `json:"sold"`
	IsRecharge          bool      `json:"isRecharge"`
	RechargeOptions     []float64 `json:"rechargeOptions"`
	DisableCustomRecharge bool    `json:"disableCustomRecharge"`
	Providers           []string  `json:"providers"`
	SuccessUrl          string    `json:"successUrl"`
	State               string    `json:"state"`
}

// ProductInfo represents Casdoor product info in orders.
type ProductInfo struct {
	Owner       string  `json:"owner"`
	Name        string  `json:"name"`
	DisplayName string  `json:"displayName"`
	Image       string  `json:"image"`
	Detail      string  `json:"detail"`
	Price       float64 `json:"price"`
	Currency    string  `json:"currency"`
	IsRecharge  bool    `json:"isRecharge"`
	Quantity    int     `json:"quantity"`
}

// Order represents a Casdoor order.
type Order struct {
	Owner        string        `json:"owner"`
	Name         string        `json:"name"`
	CreatedTime  string        `json:"createdTime"`
	UpdateTime   string        `json:"updateTime"`
	DisplayName  string        `json:"displayName"`
	Products     []string      `json:"products"`
	ProductInfos []ProductInfo `json:"productInfos"`
	User         string        `json:"user"`
	Payment      string        `json:"payment"`
	Price        float64       `json:"price"`
	Currency     string        `json:"currency"`
	State        string        `json:"state"`
	Message      string        `json:"message"`
}

// Payment represents a Casdoor payment record.
type Payment struct {
	Owner       string    `json:"owner"`
	Name        string    `json:"name"`
	CreatedTime string    `json:"createdTime"`
	DisplayName string    `json:"displayName"`
	Provider    string    `json:"provider"`
	Type        string    `json:"type"`
	Products    []string  `json:"products"`
	Currency    string    `json:"currency"`
	Price       float64   `json:"price"`
	User        string    `json:"user"`
	State       string    `json:"state"`
	Message     string    `json:"message"`
	Order       string    `json:"order"`
	OutOrderId  string    `json:"outOrderId"`
	PayUrl      string    `json:"payUrl"`
}

// PlaceOrderProductInfo is used in place-order requests.
type PlaceOrderProductInfo struct {
	Name     string  `json:"name"`
	Price    float64 `json:"price,omitempty"`
	Quantity int     `json:"quantity,omitempty"`
}

// PlaceOrderRequest is used for creating Casdoor orders.
type PlaceOrderRequest struct {
	ProductInfos []PlaceOrderProductInfo `json:"productInfos"`
}
