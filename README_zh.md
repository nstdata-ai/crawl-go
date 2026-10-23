# Nstdata Crawl Go SDK

[English](README.md)

通过 Nstdata Crawl 获取网页内容的 Go 客户端，使用 context 控制本地请求，无第三方运行依赖。

## 功能

| 能力 | 指南 |
|---|---|
| 单页抓取 | 读取 Markdown、HTML、链接、PDF、截图或原始字节。 |
| 异步任务 | [提交一次，按已保存的 ID 接续](docs/guide_zh.md#异步任务)。 |
| 整站抓取 | [限制遍历范围，按游标读取页面](docs/guide_zh.md#整站抓取)。 |
| 延迟正文 | [按需读取并缓存内容引用](docs/guide_zh.md#正文与引用)。 |
| 请求选项 | [渲染、动作、过滤、Cookie 和代理国家](docs/guide_zh.md#请求选项)。 |

## 安装

需要 Go 1.20+。生产环境使用[仍在维护的 Go 版本](https://go.dev/doc/devel/release#policy)。

```sh
go get github.com/nstdata-ai/crawl-go@v1.0.0
```

`v1.0.0` 尚未发布，本地安装见[贡献说明](CONTRIBUTING.md)。从 `v0.1.1` 升级请阅读[变更记录](CHANGELOG.md)，包括 `OnlyMainContent` 的显式序列化变化。

## 快速开始

设置环境变量 `NSTDATA_API_TOKEN`。此例创建一个抓取任务，可能消耗额度。

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

缺失正文返回 `ErrContentUnavailable`；存在但为空的正文读取成功。完整任务流程、方法输入／结果及失败处理见[使用指南](docs/guide_zh.md)。

[API 文档](https://docs.nstdata.io/docs/crawl) · [贡献说明](CONTRIBUTING.md) · [MIT 许可](LICENSE)
