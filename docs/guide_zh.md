# Go SDK 使用指南

[English](guide.md) · [快速开始](../README_zh.md)

## 方法

所有操作接收 `context.Context`，返回类型化结果和错误。多个 goroutine 共用客户端前先完成配置。

| 方法 | 输入 → 结果 |
|---|---|
| `SubmitScrapeTaskSync` | `*ScrapeRequestDto` → `*TaskResultResponseDto` |
| `SubmitScrapeTask` | `*ScrapeRequestDto` → 含 `Id` 的 `*SubmitResponseDto` |
| `GetScrapeTaskResult` | 已保存的异步任务 ID → `*TaskResultResponseDto` |
| `SubmitCrawlTask` | `*CrawlRequestDto` → 含 `Id` 的 `*SubmitResponseDto` |
| `GetCrawlStatus` | 整站 ID → `*CrawlStatusResponseDto` |
| `ListCrawlPages` | 整站 ID、`ListCrawlPagesOptions` → `*CrawlPageListResponseDto` |
| `ReadStorage` | 不透明引用 → `[]byte` |

每个方法只执行一个操作。SDK 不自动轮询、遍历分页或重试创建。同步响应的可选 `Id` 只用于诊断关联，不保证可通过异步查询恢复结果。

## 异步任务

提交一次，继续之前先持久化返回的 ID。此函数只提交、查询各一次；查询或正文读取失败时仍返回 ID。任务未完成时，稍后用同一 ID 接续。

以下函数复用快速开始中的客户端和有总期限的 context；导入 `context`、`fmt`，并将 SDK 导入为 `crawl`。

```go
func submitAndInspect(ctx context.Context, client *crawl.Client, target string) (string, string, error) {
    submitted, err := client.SubmitScrapeTask(ctx, &crawl.ScrapeRequestDto{
        URL: target, Formats: []string{crawl.FormatMarkdown}, OnlyMainContent: true,
    })
    if err != nil { return "", "", err }
    if submitted.Id == "" { return "", "", fmt.Errorf("submission returned no ID; investigate before resubmitting") }
    id := submitted.Id // Persist this ID before further work.
    result, err := client.GetScrapeTaskResult(ctx, id)
    if err != nil { return id, "", err }
    if result.Status != crawl.StatusCompleted || !result.Success || result.Data == nil {
        return id, "", fmt.Errorf("task is %s; inspect or resume the saved ID", result.Status)
    }
    text, err := result.Data.GetMarkdown(ctx)
    return id, text, err
}
```

完整可执行流程见[示例](../example/example.go)：`submit-scrape URL` 输出 ID；`resume-scrape SAVED_TASK_ID` 轮询已有任务。两条命令分开执行。提交报错时结果可能未知，应核对任务历史，再决定是否重新提交。

## 整站抓取

使用 `SubmitCrawlTask` 创建，保存 `Id`，再以 `GetCrawlStatus` 查询。`StatusCompleted` 仅描述整站任务状态，还需检查各页状态和数量。`MaxPages` 是上限，不保证返回指定数量。

需要导航内容参与链接发现时使用 `OnlyMainContent: false`。`IncludeUrls`、`ExcludeUrls`、深度和查询控制沿用服务原生规则；核对实际返回 URL，不能从任务成功推断覆盖范围。

```go
func readBatch(ctx context.Context, client *crawl.Client, id, cursor string) (string, error) {
    pages, err := client.ListCrawlPages(ctx, id, crawl.ListCrawlPagesOptions{
        AfterCursor: cursor, Limit: 20,
    })
    if err != nil { return "", err }
    for _, page := range pages.Data {
        fmt.Println(page.Url, page.TaskId, page.Status, page.Success)
        if page.Status != crawl.StatusCompleted || !page.Success || page.Data == nil { continue }
        text, err := page.Data.GetMarkdown(ctx)
        if err != nil { return "", err }
        fmt.Println(text)
    }
    return pages.NextCursor, nil
}
```

`AfterCursor` 与 `BeforeCursor` 至多使用一个。`Limit` 为 1–100；零值使用服务默认值。游标原样传递。空批次也可能有下一游标；保留服务顺序、重复 URL 和失败行。后续读取失败时，先前输出仍是部分结果。

可执行的 `resume-crawl SAVED_CRAWL_ID` 最多查询20次状态、读取100批页面，拒绝重复游标，总 context 为两分钟。保留页面失败及部分正文；取消、超时或输出错误立即停止。`submit-crawl URL` 创建最多两页、深度一的任务并返回 ID，不自动轮询。

## 正文与引用

| getter | 结果 |
|---|---|
| `GetHtml`、`GetMarkdown` | UTF-8 文本 |
| `GetPdf`、`GetScreenshot` | 二进制 `[]byte` |
| `GetRawData` | 原始字节；内联文本编码为 UTF-8 |

getter 优先使用内联内容，包括存在但为空的值；否则通过同一客户端读取对应引用。缺失／null 内容、nil 结果及未关联客户端的引用返回 `ErrContentUnavailable`。空字符串或空字节且无错误，表示读取成功的空正文。

成功结果会缓存。失败读取，包括非法 UTF-8 的 HTML／Markdown，不缓存；调用方再次调用 getter 才会发起新尝试。首次并发读取可能各自下载同一引用。复制结果 DTO 会共享已初始化的缓存；返回的字节切片是独立副本。

`ReadStorage` 和二进制 getter 保留任意原始字节。JSON 内联 PDF／截图按 Base64 解码；原始数据不代表结构化 JSON 抽取。API JSON 和文本引用必须是有效 UTF-8。

向下游传递正文时保留 `Id`／`TaskId`、`Url`、状态／成功标志、`Metadata` 和 `Truncated`。`Truncated == nil` 表示未知；true 表示服务已截断。已知 metadata 字段有类型，`AdditionalFields` 以 `json.RawMessage` 保留未知字段。既有标量字段对缺失／null 使用 Go 零值，不单独保留字段是否存在。

并发使用前配置公开字段；getter 运行期间不要修改客户端配置或结果字段。

## 请求选项

| 字段 | 行为 |
|---|---|
| `URL`、`Formats` | 必填；格式含 `markdown`、`html`、`links`、`pdf`、`screenshot`、`rawData`，可使用导出的 `Format...` 常量。 |
| `OnlyMainContent` | 始终序列化为 true/false。零值 false 明确请求不做主正文过滤，需过滤时显式设置 true。 |
| `IncludeTags`、`ExcludeTags`、`RemoveSelectors`、`TargetSelector` | 服务定义的正文过滤。 |
| `Cookies`、`Headers`、`Proxy`、`Mobile` | Cookie、自定义请求头、代理出口国家和移动设备。 |
| `JsRender` | `WaitFor`、`WaitForSelector`、`Actions`；helper 支持点击、输入、等待、滚动、JavaScript 和截图。 |
| `Timeout` | 可选整数毫秒，1000–60000；零值省略。 |
| `MaxDepth`、`MaxPages`、`IncludeUrls`、`ExcludeUrls`、`IgnoreQuery` | 整站专用遍历控制。 |

`JsRender.WaitFor` 为1000–30000整数毫秒。动作的毫秒和坐标须为有限整数；保留已有 float64 字段类型。其他可选零值字段省略，由服务决定默认行为。服务端请求时间与本地 HTTP／context 期限独立。

## 配置与错误

| 客户端字段 | 默认值／约束 |
|---|---|
| `BaseURL` | `https://api.nstdata.io`；无凭据、路径、查询和片段的 HTTPS origin，仅回环测试允许 HTTP。 |
| `HTTPClient` | 独立客户端，操作限时120秒；nil 使用此默认值。保留自定义 transport／timeout，但始终拒绝重定向。 |
| `MaxResponseBytes` | 每个解码后的 API／存储响应1 MiB；更大获准内容需设置小于 MaxInt64 的正数字节数。 |

为完整流程设置有总期限的 context。取消／超时可用 `errors.Is` 判断；本地停止不取消远端任务。自定义 transport 必须遵守 context 并保留 TLS 证书校验。响应体会关闭；字节上限不包括解析和复制额外占用的内存。

| 错误 | 含义 |
|---|---|
| `*APIError` | HTTP 或 API 外层业务错误；通过 `errors.As` 检查 `StatusCode` 和可用的外层 `Code`。 |
| `ErrContentUnavailable` | 请求的内容不存在。 |
| `ErrResponseTooLarge` | 解码响应超出配置的字节上限。 |
| `context.Canceled`、`context.DeadlineExceeded` | 本地取消或期限，可通过 `errors.Is` 判断。 |
| 其他错误 | 本地参数不合法、JSON／字段畸形、非法文本编码或传输／读取失败。 |

任务失败保留为类型化结果，由调用方检查，不转换成 HTTP 错误。SDK 错误文字不包含服务原始正文、引用和传输 URL。网页内容及任务字段作为不可信数据处理；Key 和签名引用不写日志。

## API 路由

客户端在配置的 origin 使用 `x-api-key`。任务 ID 不能含路径分隔符；存储引用和游标进行查询参数编码。

| 操作 | 路由 |
|---|---|
| 同步／异步单页 | `POST /api/v1/crawl/scrape`；异步增加 `?async=true` |
| 单页结果 | `GET /api/v1/crawl/scrape/{id}` |
| 整站／状态 | `POST /api/v1/crawl`、`GET /api/v1/crawl/{id}` |
| 页面 | `GET /api/v1/crawl/{id}/pages`，带游标／limit |
| 存储 | `GET /api/v1/crawl/storage/read?st=...` |
