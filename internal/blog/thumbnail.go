package blog

import (
	"strings"

	"golang.org/x/net/html"
)

// firstBodyImage 读取已清理的正文首图，不使用元信息封面或额外请求。
func firstBodyImage(body string) string {
	tokenizer := html.NewTokenizer(strings.NewReader(body))
	for {
		switch tokenizer.Next() {
		case html.ErrorToken:
			return ""
		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokenizer.Token()
			if token.Data != "img" {
				continue
			}
			for _, attr := range token.Attr {
				if attr.Key == "src" {
					return attr.Val
				}
			}
			return ""
		}
	}
}
