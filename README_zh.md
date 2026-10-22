# Nstdata Crawl Go SDK

[English](README.md)

使用 Go 标准库提供同步/异步单页抓取、受限整站抓取、任务查询和内容引用读取，无第三方运行依赖。

## 安装

保留 Go 1.20 兼容性。生产环境请使用[仍在维护的 Go 版本](https://go.dev/doc/devel/release#policy)。

`v1.0.0` 已在本地准备，尚未发布。正式发布后安装：

```sh
go get github.com/nstdata-ai/crawl-go@v1.0.0
```

源码安装和检查步骤见[贡献说明](CONTRIBUTING.md)。

## 快速开始

将获准使用的 Crawl Key 保存在 `NSTDATA_API_TOKEN`。以下示例提交一次收费的同步抓取，请使用有权访问的目标。

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"
    "time"

    crawl "github.com/nstdata-ai/crawl-go"
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
    defer cancel()
    client := crawl.NewClient(os.Getenv("NSTDATA_API_TOKEN"))
    result, err := client.SubmitScrapeTaskSync(ctx, &crawl.ScrapeRequestDto{
        URL: "https://example.com/", Formats: []string{crawl.FormatMarkdown}, OnlyMainContent: true,
    })
    if err != nil { log.Fatal(err) }
    if result.Status != crawl.StatusCompleted || !result.Success || result.Data == nil {
        log.Fatal("task did not return successful completed content")
    }
    text, err := result.Data.GetMarkdown(ctx)
    if err != nil { log.Fatal(err) }
    fmt.Println(text)
    if result.Data.Truncated != nil && *result.Data.Truncated {
        log.Print("content is truncated")
    }
}
```

## 操作

| 方法 | 用途 |
|---|---|
| `SubmitScrapeTaskSync` | 同步抓取一次，返回任务状态和可选内容 |
| `SubmitScrapeTask` | 异步提交；查询前保存 `Id` |
| `GetScrapeTaskResult` | 查询已保存的异步任务 ID |
| `SubmitCrawlTask` | 使用原生遍历参数提交整站任务 |
| `GetCrawlStatus` | 查询整站进度，保留失败数量 |
| `ListCrawlPages` | 读取一批游标结果，保留失败页面 |
| `ReadStorage` | 通过带认证的 API 入口读取不透明引用 |

每个方法只执行一个操作，不自动轮询、翻页或重试提交。任务 completed 也可能失败或没有内容；同步响应的 ID 不保证可以后续查询。

`ListCrawlPagesOptions` 只能使用 `AfterCursor`、`BeforeCursor` 其中一个；`Limit` 为 1–100，零值使用服务端默认值。游标原样传入。`MaxPages` 是上限，不保证返回指定数量的页面；URL 过滤和深度计数沿用服务端原生规则。

[可执行示例](example/example.go)提供 `scrape`、`submit-scrape`、`submit-crawl`、`resume-scrape`、`resume-crawl`。提交和继续查询分开；最多查询 20 次状态，最多读取 100 批页面，遇到重复游标停止，总 context 为两分钟。本地等待停止时保留任务 ID 和部分结果，不代表远端任务已取消。

## 内容和错误

- `GetHtml` / `GetMarkdown` 返回文本；`GetPdf` / `GetScreenshot` 返回解码后的字节；`GetRawData` 返回 UTF-8 字节，不代表结构化抽取。
- getter 优先使用内联内容，包括空值；只有没有内联值时才读取引用。成功读取会缓存；首次并发读取可能各自下载同一引用，后续使用缓存。修改返回的字节不会影响缓存。
- 缺失/null 内容、nil 结果或未关联客户端的引用返回 `ErrContentUnavailable`；存在但为空的内容读取成功。`Truncated == nil` 表示未知，与明确的 false 区分。
- 已知 metadata 字段有类型；其他字段保存在 `Metadata.AdditionalFields`，类型为 `json.RawMessage`。输出可用性和提取质量取决于服务端。
- HTTP/外层业务错误返回 `*APIError`，含 `StatusCode` 和可用的外层 `Code`；任务失败仍作为正常响应返回，由调用者检查。错误文字不包含响应正文、引用或传输 URL。
- context 取消和超时可通过 `errors.Is` 判断。非法 JSON、响应包装或内容类型返回错误；响应超限返回 `ErrResponseTooLarge`。

## 配置

并发使用前配置 `Client`：

- `BaseURL` 默认为 `https://api.nstdata.io`。自定义入口要求 HTTPS，仅本地回环测试允许 HTTP；不接受用户密码、路径、查询或片段。
- `HTTPClient` 默认为独立客户端，单次操作限时 120 秒；nil 使用此默认值。自定义客户端保留 transport 和 timeout，但始终拒绝重定向，避免转发认证和签名引用。保留 TLS 证书校验；自定义 transport 必须遵守请求 context。
- `MaxResponseBytes` 默认为每个解码后的 API/存储响应 1 MiB；更大输出需显式设置正数字节上限，与服务端内容截断是不同限制。
- 请求 `Timeout` 是可选整数毫秒，范围 1000–60000；`JsRender.WaitFor` 为 1000–30000。动作的毫秒和坐标须为有限整数。保留既有数值字段类型，零值省略可选设置。请求 DTO 超时与本地 context/HTTP 超时分别控制。

并发期间不要修改配置或结果的公开字段。Key 放在本地环境或密钥存储，不记录 Key、签名引用或私有正文。

## 兼容性

保留现有导入路径、构造函数、公开方法和动作 helper。相较 `v0.1.1`，默认域名已更新，畸形响应会报错，缺失内容返回错误，拒绝重定向，并设置默认响应大小上限。调用方须处理这些错误，并按需显式设置更大上限。见[变更记录](CHANGELOG.md)。

[API 文档](https://docs.nstdata.io/docs/crawl) · [许可](LICENSE)
