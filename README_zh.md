# nstdata crawl SDK (Golang)

[English](README.md)

nstdata crawl 的 Golang SDK，提供高级的网页抓取和爬虫服务接口。

## 安装

```bash
go get github.com/nstdata-ai/crawl-go
```

## 功能特性

- **基础抓取:** 同步获取页面的 HTML 和 Markdown 格式内容。
- **截图与 PDF:** 支持整页截图与页面导出为 PDF。
- **浏览器动作编排:** 支持等待元素、表单填充、点击、滚动以及执行自定义 JavaScript。
- **自定义配置:** 携带 Cookie、自定义请求头、模拟移动端等。
- **正文提取与过滤:** 自动提取高质量文章正文，去除广告、导航栏和页脚。
- **代理位置:** 可指定代理出口国家。
- **异步提交与轮询:** 针对耗时任务，支持异步提交并轮询查询结果。
- **批量抓取 (Crawl):** 按深度、最大页数等条件遍历站点。

## 快速开始

您可以参考 [example.go](./example/example.go) 了解所有场景的完整使用示例。

```go
package main

import (
	"context"
	"fmt"
	"github.com/nstdata-ai/crawl-go"
	"log"
)

func main() {
	// 使用您的 API Token 初始化客户端
	client := crawl.NewClient("YOUR_API_TOKEN")
	
	// 提交基础的抓取任务
	res, err := client.SubmitScrapeTaskSync(context.Background(), &crawl.ScrapeRequestDto{
		URL:     "https://example.com/",
		Formats: []string{crawl.FormatMarkdown},
		Timeout: 60000,
	})
	
	if err != nil {
		log.Fatal(err)
	}
	
	// 输出抓取到的 Markdown 内容
	md, _ := res.Data.GetMarkdown(context.Background())
	fmt.Println(md)
}
```
