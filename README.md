# newApi-yg

二开 newApi 路由 – 接入 Langfuse 可观测性平台

[newApi (new-api)](https://github.com/QuantumNous/new-api) 是一个开源的下一代大模型网关，本仓库在其基础上添加了
[Langfuse](https://langfuse.com) 集成，实现对每一次 LLM 调用的链路追踪、Token 用量记录和性能分析。

---

## 功能

| 特性 | 说明 |
|------|------|
| 链路追踪 | 每个 API 请求自动创建 Langfuse Trace |
| Generation 记录 | 记录模型名称、输入消息、输出内容、Token 用量及延迟 |
| 会话分组 | 通过请求头 `X-Session-Id` 将多次调用关联到同一 Session |
| 无侵入降级 | 未配置环境变量时中间件自动降级为空操作，不影响网关正常运行 |
| 流式支持 | 支持流式（SSE）和非流式两种响应模式 |

---

## 快速开始

### 1. 配置环境变量

```bash
export LANGFUSE_HOST="https://cloud.langfuse.com"    # 或自托管地址
export LANGFUSE_PUBLIC_KEY="pk-lf-..."
export LANGFUSE_SECRET_KEY="sk-lf-..."
```

### 2. 在 new-api 的路由中注册中间件

在 `router/` 目录中找到 relay 路由的注册处（通常是 `router/relay.go`），添加：

```go
import "github.com/DreamYG/newApi-yg/pkg/langfuse"

// 在 relay 路由组注册中间件
relayRouter.Use(langfuse.Middleware())
```

### 3. 在 relay handler 中记录 Generation

以 `relay/compatible_handler.go` 为例，在获取到 LLM 响应并完成 token 计数后调用：

```go
import "github.com/DreamYG/newApi-yg/pkg/langfuse"

// 在 postConsumeQuota / RecordConsumeLog 调用之后：
go langfuse.TrackGeneration(langfuse.GenerationParams{
    TraceID:          langfuse.TraceIDFromContext(c),
    Name:             "chat-completion",
    Model:            info.OriginModelName,
    Input:            request.Messages,
    Output:           completionContent,
    PromptTokens:     usage.PromptTokens,
    CompletionTokens: usage.CompletionTokens,
    StartTime:        langfuse.StartTimeFromContext(c),
    EndTime:          time.Now(),
    UserID:           strconv.Itoa(info.UserId),
    Metadata: map[string]any{
        "channel_id":   info.ChannelId,
        "token_name":   info.TokenKey,
        "relay_mode":   info.RelayMode,
        "is_stream":    info.IsStream,
    },
})
```

> `TrackGeneration` 的调用通过 goroutine 异步执行，不会阻塞响应返回给客户端。
> Langfuse SDK 内部使用批量发送机制，`Flush` 在调用后会等待队列清空。

---

## 包结构

```
pkg/langfuse/
├── doc.go          # 包文档
├── client.go       # Langfuse 客户端单例，读取环境变量初始化
├── middleware.go   # Gin 中间件：记录请求开始时间并创建 Trace
├── tracker.go      # TrackGeneration / StartTrace 高级封装
└── langfuse_test.go
```

### 核心 API

#### `Middleware() gin.HandlerFunc`

Gin 中间件，需挂载在 relay 路由组：

- 将请求开始时间写入 context（key: `langfuse_start_time`）
- 创建 Langfuse Trace，将 Trace ID 写入 context（key: `langfuse_trace_id`）

#### `TrackGeneration(params GenerationParams)`

异步记录一次 LLM Generation 到 Langfuse。若客户端未启用则为空操作。

#### `StartTrace(name, userID, sessionID string, input any, metadata map[string]any, tags []string) string`

手动创建一个 Trace，返回 Trace ID。供需要自定义 Trace 元数据的场景使用。

#### `TraceIDFromContext(c *gin.Context) string`

从 Gin context 中读取当前请求的 Langfuse Trace ID。

#### `StartTimeFromContext(c *gin.Context) time.Time`

从 Gin context 中读取请求开始时间。

---

## 开发

```bash
# 运行测试
go test ./pkg/langfuse/... -v

# 代码检查
go vet ./...
```

---

## 许可证

Apache 2.0，与上游 [new-api](https://github.com/QuantumNous/new-api) 保持一致。
