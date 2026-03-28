# casdoor-kit

Casdoor-Kit 是一个围绕 Casdoor 的共享工具库，为微服务提供 **认证授权** 和 **配额计费** 能力。基于 Casdoor OAuth2 + JWT (RS256) 实现用户认证，支持基于时间配额的冻结/消费/解冻两阶段计费模型。

## 功能概览

| 模块 | 功能 |
|------|------|
| `authz/casdoor` | Casdoor OAuth2 客户端（登录/注册/登出/Token 刷新/产品/订单） |
| `authz/session` | Redis 会话存储（Casdoor Token 持久化，默认 TTL 7 天） |
| `billing` | 权益管理 + 支付服务（配额冻结/消费/解冻，订单同步，权益发放） |
| `billing/repo` | GORM 数据访问层（PostgreSQL，乐观锁并发安全） |
| `billing/model` | 数据模型 + AutoMigrate 自动建表 |
| `handler` | Gin HTTP Handler（认证 + 支付/权益 API） |
| `middleware` | Gin 中间件（JWT 鉴权，用户上下文注入） |
| `response` | 统一 HTTP 响应格式 |
| `config` | 配置结构体定义 |

## 外部依赖

| 依赖 | 用途 |
|------|------|
| **Casdoor** | OAuth2 认证服务 + 支付网关 |
| **PostgreSQL** | 计费数据持久化（6 张表，AutoMigrate 自动创建） |
| **Redis** | 会话存储 |
| **RSA 密钥对** | JWT RS256 签名/验证 |

## 快速开始

### 1. 安装

```bash
go get github.com/weinaike/casdoor-kit
```

### 2. 准备配置

参考 `config.example.yaml`，准备以下配置：

```yaml
jwt:
  public_key_path: "./configs/public.key"
  private_key_path: "./configs/private.key"
  issuer: "your-service"
  access_token_ttl: 86400

casdoor:
  endpoint: "https://your-casdoor-instance.com"
  client_id: "your-client-id"
  client_secret: "your-client-secret"
  organization: "your-org"
  application: "your-app"
  redirect_uri: "http://localhost:5173/auth/callback"

entitlement:
  product_mappings:
    basic-pack:
      quota_seconds: 3600
      entitlement_type: "TOP_UP"
      period_months: 0
      description: "1小时加油包"
```

### 3. 初始化

```go
package main

import (
    kitauthz "github.com/weinaike/casdoor-kit/authz"
    kitcasdoor "github.com/weinaike/casdoor-kit/authz/casdoor"
    kitsession "github.com/weinaike/casdoor-kit/authz/session"
    kitbilling "github.com/weinaike/casdoor-kit/billing"
    kitmodel "github.com/weinaike/casdoor-kit/billing/model"
    kitrepo "github.com/weinaike/casdoor-kit/billing/repo"
    kitconfig "github.com/weinaike/casdoor-kit/config"
    gokit "github.com/weinaike/casdoor-kit"

    "github.com/redis/go-redis/v9"
    "gorm.io/gorm"
)

func setup(db *gorm.DB, redisClient *redis.Client) error {
    // 1. 设置日志（可选，默认 NoOp）
    gokit.SetLogger(yourLogger)

    // 2. 自动建表
    if err := kitmodel.AutoMigrate(db); err != nil {
        return err
    }

    // 3. 初始化 Casdoor 客户端
    casdoorClient := kitcasdoor.NewClient(&kitconfig.CasdoorConfig{
        Endpoint:     "https://your-casdoor.com",
        ClientID:     "xxx",
        ClientSecret: "xxx",
        Organization: "org",
        Application:  "app",
        RedirectURI:  "http://localhost:5173/auth/callback",
    })

    // 4. 初始化会话存储（Redis）
    sessionStore := kitsession.NewRedisStore(redisClient)

    // 5. 初始化认证服务
    authService, err := kitauthz.NewAuthService(casdoorClient, &kitconfig.JWTConfig{
        PublicKeyPath:  "./configs/public.key",
        PrivateKeyPath: "./configs/private.key",
        Issuer:         "your-service",
        AccessTokenTTL: 86400,
    }, sessionStore)
    if err != nil {
        return err
    }

    // 6. 初始化计费服务
    billingRepo := kitrepo.NewBillingRepository(db)
    entitlementCfg := &kitconfig.EntitlementConfig{
        ProductMappings: map[string]kitconfig.ProductMapping{
            "basic-pack": {QuotaSeconds: 3600, EntitlementType: "TOP_UP", PeriodMonths: 0, Description: "1小时加油包"},
        },
    }
    entitlementService := kitbilling.NewEntitlementService(billingRepo, entitlementCfg)
    paymentService := kitbilling.NewPaymentService(casdoorClient, authService, billingRepo, entitlementService)

    return nil
}
```

### 4. 注册路由

使用 casdoor-kit 提供的 Handler：

```go
import (
    "github.com/weinaike/casdoor-kit/handler"
    "github.com/weinaike/casdoor-kit/middleware"
    "github.com/gin-gonic/gin"
)

func registerRoutes(r *gin.Engine, authService authz.AuthService,
    paymentService billing.PaymentService, entitlementService billing.EntitlementService,
    publicKeyPath string) {

    authHandler := handler.NewAuthHandler(authService)
    paymentHandler := handler.NewPaymentHandler(paymentService, entitlementService)

    v1 := r.Group("/api/v1")

    // 公开路由
    auth := v1.Group("/auth")
    {
        auth.GET("/login-url", authHandler.GetLoginURL)
        auth.Any("/callback", authHandler.Callback)
    }

    // 需要认证的路由
    authorized := v1.Group("")
    authorized.Use(middleware.JWTAuth(publicKeyPath))
    {
        authorized.GET("/auth/me", authHandler.GetCurrentUser)
        authorized.POST("/auth/logout", authHandler.Logout)

        authorized.GET("/products", paymentHandler.GetProducts)
        authorized.POST("/orders", paymentHandler.CreateOrder)
        authorized.GET("/orders", paymentHandler.GetOrders)
        authorized.POST("/orders/:order_name/pay", paymentHandler.PayOrder)
        authorized.POST("/orders/:order_name/cancel", paymentHandler.CancelOrder)
        authorized.POST("/orders/:order_name/sync", paymentHandler.SyncOrder)
        authorized.GET("/balance", paymentHandler.GetBalance)
        authorized.GET("/entitlements", paymentHandler.ListEntitlements)
        authorized.GET("/billing/history", paymentHandler.GetBillingHistory)
    }

    // 支付回调（Casdoor 回调，无需 JWT）
    v1.POST("/payment/callback", paymentHandler.PaymentCallback)
}
```

## 核心概念

### 认证流程

```
用户点击登录 → 前端获取 LoginURL → 跳转 Casdoor → 回调带 code
    → casdoor-kit 用 code 换取 Casdoor Token → 获取用户信息
    → 存储 Casdoor Token 到 Redis → 签发本地 JWT (RS256) → 返回给前端
```

前端后续请求携带 `Authorization: Bearer <jwt_token>`，由 `middleware.JWTAuth()` 验证。

### 计费模型（两阶段提交）

```
任务开始 → FreezeForTask(userID, taskRef, seconds)   冻结配额
任务成功 → ConsumeTask(taskRef)                       核销配额
任务失败 → UnfreezeTask(taskRef)                      解冻配额
```

冻结时按权益包优先级消耗：**GIFT → SUBSCRIPTION → TOP_UP**，先到期先用。

### 权益类型

| 类型 | 说明 | 有效期 |
|------|------|--------|
| `TOP_UP` | 加油包，一次性购买 | `period_months=0` 永久有效 |
| `SUBSCRIPTION` | 订阅套餐，周期续费 | `period_months>0` 按月计 |
| `GIFT` | 赠送，运营活动发放 | 由配置决定 |

## 数据库表

`billing/model.AutoMigrate(db)` 自动创建以下 6 张表：

| 表 | 用途 |
|----|------|
| `user_wallet` | 用户钱包（余额/冻结，乐观锁 version） |
| `user_entitlement` | 用户权益包（总量/已用/冻结，状态/有效期） |
| `user_order` | 订单记录 |
| `product_entitlement_mapping` | Casdoor 产品 → 权益映射 |
| `billing_transaction_log` | 交易流水（不可变审计日志） |
| `task_billing` | 任务级计费记录（冻结明细 JSONB） |

## 日志

casdoor-kit 使用接口日志，默认 NoOp（不输出日志）。接入方可通过 `gokit.SetLogger()` 注入自己的日志实现：

```go
type Logger interface {
    Debug(msg string, keys ...any)
    Info(msg string, keys ...any)
    Warn(msg string, keys ...any)
    Error(msg string, keys ...any)
}
```

例如包装 zap：

```go
type zapLogger struct { l *zap.SugaredLogger }

func (z *zapLogger) Info(msg string, keys ...any)  { z.l.Infow(msg, keys...) }
// ... 其他方法
```

## API 端点一览

### 认证

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/auth/login-url` | 获取 Casdoor 登录链接 |
| GET/POST | `/api/v1/auth/callback` | OAuth2 回调 |
| GET | `/api/v1/auth/me` | 获取当前用户信息（需 JWT） |
| POST | `/api/v1/auth/logout` | 登出（需 JWT） |

### 支付 & 权益

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/products` | 获取产品列表（需 JWT） |
| POST | `/api/v1/orders` | 创建订单并发起支付（需 JWT） |
| GET | `/api/v1/orders` | 获取订单列表（需 JWT） |
| POST | `/api/v1/orders/:order_name/pay` | 为已有订单发起支付（需 JWT） |
| POST | `/api/v1/orders/:order_name/cancel` | 取消订单（需 JWT） |
| POST | `/api/v1/orders/:order_name/sync` | 同步订单状态并发放权益（需 JWT） |
| GET | `/api/v1/balance` | 获取余额（需 JWT） |
| GET | `/api/v1/entitlements` | 获取权益包列表（需 JWT） |
| GET | `/api/v1/billing/history` | 获取计费历史（需 JWT） |
| POST | `/api/v1/payment/callback` | 支付回调（Casdoor 调用，无需 JWT） |

## 技术栈

| 组件 | 库 |
|------|-----|
| Web Framework | Gin |
| JWT | golang-jwt/jwt/v5 (RS256) |
| ORM | GORM (PostgreSQL, pgx driver) |
| Redis | go-redis/v9 |
| OAuth2 | Casdoor |

## License

MIT
