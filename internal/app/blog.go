package app

import (
	"flag"
	"fmt"
	"io"

	"github.com/howiedata/aowugong-go/internal/blog"
)

// RunBlog 执行独立博客 CLI；文件校验不装配数据库、HTTP 或通知服务。
func RunBlog(args []string, output io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("用法: aowugong blog validate --dir <文章目录>")
	}
	flags := flag.NewFlagSet("blog "+args[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	directory := flags.String("dir", "", "文章目录")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *directory == "" {
		return fmt.Errorf("必须指定 --dir 且不能有额外参数")
	}
	if args[0] != "validate" {
		return fmt.Errorf("未知博客命令: %s", args[0])
	}
	store := blog.NewArticleStore(*directory)
	if err := store.Validate(); err != nil {
		return err
	}
	articles, err := store.List()
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(output, "已校验 %d 篇文章\n", len(articles))
	return err
}
