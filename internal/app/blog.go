package app

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/howiedata/aowugong-go/internal/blog"
	"github.com/howiedata/aowugong-go/internal/config"
	"github.com/howiedata/aowugong-go/internal/database"
)

// RunBlog 执行独立博客 CLI；文件校验不装配数据库、HTTP 或通知服务。
func RunBlog(args []string, output io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("用法: aowugong blog validate --dir <文章目录>")
	}
	flags := flag.NewFlagSet("blog "+args[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	directory := flags.String("dir", "", "文章目录")
	archive := flags.String("archive", "", "内容压缩包")
	root := flags.String("root", "", "内容根目录")
	sequence := flags.Int64("sequence", 0, "发布序号")
	sourceFile := flags.String("file", "", "历史动态文件")
	apply := flags.Bool("apply", false, "执行数据库导入")
	authorID := flags.Int64("author-id", 0, "作者主键")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("必须指定 --dir 且不能有额外参数")
	}
	if args[0] == "import-status" {
		return importBlogStatuses(*sourceFile, *apply, *authorID, output)
	}
	if args[0] == "publish" {
		if *root == "" || *sequence < 1 || ((*directory == "") == (*archive == "")) {
			return fmt.Errorf("publish 需要 --root、--sequence，以及 --dir 或 --archive 之一")
		}
		var err error
		if *archive != "" {
			err = blog.PublishArchive(*archive, *root, *sequence)
		} else {
			err = blog.Publish(*directory, *root, *sequence)
		}
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(output, "文章内容已发布，序号 %d\n", *sequence)
		return err
	}
	if *directory == "" {
		return fmt.Errorf("必须指定 --dir")
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

// importBlogStatuses 先核对全部内容，只有显式 --apply 才打开数据库。
func importBlogStatuses(filename string, apply bool, authorID int64, output io.Writer) error {
	if filename == "" {
		return fmt.Errorf("必须指定 --file")
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	rows, err := blog.ParseLegacyStatuses(data)
	if err != nil {
		return err
	}
	if !apply {
		_, err = fmt.Fprintf(output, "核对完成：%d 条历史动态，未写入数据库\n", len(rows))
		return err
	}
	if authorID < 1 {
		return fmt.Errorf("导入必须指定 --author-id")
	}
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return err
	}
	db, err := database.OpenPostgres(context.Background(), cfg.Database)
	if err != nil {
		return err
	}
	defer db.Close()
	var exists bool
	if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM aowugong_fastapi_users WHERE id = ?)`, authorID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("作者不存在")
	}
	if err := blog.NewStatusService(blog.NewRepository(db)).ImportLegacy(context.Background(), authorID, rows); err != nil {
		return err
	}
	_, err = fmt.Fprintf(output, "已导入 %d 条历史动态（重复执行不会新增）\n", len(rows))
	return err
}
