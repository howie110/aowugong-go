package client

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/smtp"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/howiedata/aowugong-go/internal/config"
)

// EmailAttachment 描述一份从磁盘流式读取的邮件附件。
type EmailAttachment struct {
	Name string
	Path string
}

// EmailMessage 描述 SMTP 邮件正文、收件人和附件。
type EmailMessage struct {
	To          []string
	Subject     string
	Body        string
	Attachments []EmailAttachment
}

// EmailClient 通过 TLS SMTP 发送文本和文件邮件。
type EmailClient struct {
	config config.Email
}

// NewEmailClient 创建 QQ SMTP 兼容邮件客户端。
// 输入：cfg 包含服务器、端口、发件邮箱和 SMTP 授权码。
// 输出：返回可复用的邮件客户端。
// 副作用：无，不建立网络连接。
func NewEmailClient(cfg config.Email) *EmailClient {
	return &EmailClient{config: cfg}
}

// Send 发送一封文本邮件并流式附加指定文件。
// 输入：ctx 控制超时，message 包含收件人、主题、正文和附件路径。
// 输出：成功返回 nil；连接、认证、读取或发送失败时返回带上下文的错误。
// 副作用：连接 SMTP 服务并向外部邮箱发送邮件。
func (c *EmailClient) Send(ctx context.Context, message EmailMessage) error {
	// 1. 校验运行配置和邮件必要字段，避免建立无效连接。
	if c == nil || strings.TrimSpace(c.config.Host) == "" || c.config.Port < 1 ||
		strings.TrimSpace(c.config.Sender) == "" || strings.TrimSpace(c.config.Password) == "" {
		return fmt.Errorf("SMTP 配置不完整")
	}
	if len(message.To) == 0 || strings.TrimSpace(message.Subject) == "" {
		return fmt.Errorf("邮件收件人或主题为空")
	}

	// 2. 使用上下文建立 TLS 连接，并为后续 SMTP 操作设置统一截止时间。
	address := net.JoinHostPort(c.config.Host, fmt.Sprintf("%d", c.config.Port))
	dialer := &tls.Dialer{Config: &tls.Config{ServerName: c.config.Host, MinVersion: tls.VersionTLS12}}
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("连接 SMTP 服务: %w", err)
	}
	defer connection.Close()
	if deadline, ok := ctx.Deadline(); ok {
		if err := connection.SetDeadline(deadline); err != nil {
			return fmt.Errorf("设置 SMTP 截止时间: %w", err)
		}
	} else if err := connection.SetDeadline(time.Now().Add(5 * time.Minute)); err != nil {
		return fmt.Errorf("设置 SMTP 默认截止时间: %w", err)
	}

	// 3. 完成 SMTP 认证并逐个登记收件人。
	smtpClient, err := smtp.NewClient(connection, c.config.Host)
	if err != nil {
		return fmt.Errorf("创建 SMTP 客户端: %w", err)
	}
	defer smtpClient.Close()
	if err := smtpClient.Auth(smtp.PlainAuth("", c.config.Sender, c.config.Password, c.config.Host)); err != nil {
		return fmt.Errorf("认证 SMTP 账号: %w", err)
	}
	if err := smtpClient.Mail(c.config.Sender); err != nil {
		return fmt.Errorf("设置 SMTP 发件人: %w", err)
	}
	for _, recipient := range message.To {
		if err := smtpClient.Rcpt(strings.TrimSpace(recipient)); err != nil {
			return fmt.Errorf("设置 SMTP 收件人 %s: %w", recipient, err)
		}
	}

	// 4. 流式写入 MIME 正文和附件，避免把完整备份读入内存。
	dataWriter, err := smtpClient.Data()
	if err != nil {
		return fmt.Errorf("打开 SMTP 邮件正文: %w", err)
	}
	if err := writeEmailMessage(dataWriter, c.config.Sender, message); err != nil {
		_ = dataWriter.Close()
		return fmt.Errorf("写入 SMTP 邮件: %w", err)
	}
	if err := dataWriter.Close(); err != nil {
		return fmt.Errorf("提交 SMTP 邮件: %w", err)
	}
	if err := smtpClient.Quit(); err != nil {
		return fmt.Errorf("结束 SMTP 会话: %w", err)
	}
	return nil
}

// writeEmailMessage 写入符合 MIME 规范的文本和附件内容。
// 输入：writer 是 SMTP 数据流，sender 和 message 提供邮件字段。
// 输出：成功返回 nil；附件读取或 MIME 写入失败时返回错误。
// 副作用：读取附件文件并写入目标流。
func writeEmailMessage(writer io.Writer, sender string, message EmailMessage) error {
	// 1. 建立 MIME 边界并写入顶层邮件头。
	buffered := bufio.NewWriter(writer)
	multipartWriter := multipart.NewWriter(buffered)
	headers := []string{
		"From: " + sender,
		"To: " + strings.Join(message.To, ", "),
		"Subject: " + mime.QEncoding.Encode("UTF-8", message.Subject),
		"MIME-Version: 1.0",
		"Content-Type: multipart/mixed; boundary=" + multipartWriter.Boundary(),
		"",
		"",
	}
	if _, err := io.WriteString(buffered, strings.Join(headers, "\r\n")); err != nil {
		return fmt.Errorf("写入邮件头: %w", err)
	}

	// 2. 写入 UTF-8 纯文本正文。
	textHeader := make(textproto.MIMEHeader)
	textHeader.Set("Content-Type", "text/plain; charset=UTF-8")
	textHeader.Set("Content-Transfer-Encoding", "8bit")
	textPart, err := multipartWriter.CreatePart(textHeader)
	if err != nil {
		return fmt.Errorf("创建邮件正文: %w", err)
	}
	if _, err := io.WriteString(textPart, message.Body); err != nil {
		return fmt.Errorf("写入邮件正文: %w", err)
	}

	// 3. 逐份流式读取并编码附件，确保长内容符合邮件行宽约束。
	for _, attachment := range message.Attachments {
		if err := writeEmailAttachment(multipartWriter, attachment); err != nil {
			return err
		}
	}
	if err := multipartWriter.Close(); err != nil {
		return fmt.Errorf("关闭 MIME 内容: %w", err)
	}
	if err := buffered.Flush(); err != nil {
		return fmt.Errorf("刷新邮件内容: %w", err)
	}
	return nil
}

// writeEmailAttachment 把单个文件编码为 MIME 附件。
// 输入：writer 是邮件多段写入器，attachment 提供显示名称和磁盘路径。
// 输出：成功返回 nil；文件读取或编码失败时返回错误。
// 副作用：读取附件文件并写入邮件数据流。
func writeEmailAttachment(writer *multipart.Writer, attachment EmailAttachment) error {
	// 1. 打开附件并准备安全的 ASCII 文件名。
	file, err := os.Open(attachment.Path)
	if err != nil {
		return fmt.Errorf("打开邮件附件 %s: %w", attachment.Path, err)
	}
	defer file.Close()
	name := filepath.Base(attachment.Name)
	if name == "." || name == "" {
		name = filepath.Base(attachment.Path)
	}

	// 2. 创建附件段并使用 76 字符换行的 Base64 流式编码。
	header := make(textproto.MIMEHeader)
	header.Set("Content-Type", "application/octet-stream; name=\""+name+"\"")
	header.Set("Content-Disposition", "attachment; filename=\""+name+"\"")
	header.Set("Content-Transfer-Encoding", "base64")
	part, err := writer.CreatePart(header)
	if err != nil {
		return fmt.Errorf("创建邮件附件 %s: %w", name, err)
	}
	encoder := base64.NewEncoder(base64.StdEncoding, &mimeLineWriter{writer: part, width: 76})
	if _, err := io.Copy(encoder, file); err != nil {
		_ = encoder.Close()
		return fmt.Errorf("编码邮件附件 %s: %w", name, err)
	}
	if err := encoder.Close(); err != nil {
		return fmt.Errorf("关闭邮件附件编码 %s: %w", name, err)
	}
	return nil
}

type mimeLineWriter struct {
	writer io.Writer
	width  int
	column int
}

// Write 按 MIME 推荐宽度为 Base64 内容插入 CRLF。
// 输入：data 是 Base64 编码器产生的字节。
// 输出：返回已消费的输入字节数；写入失败时返回错误。
// 副作用：向底层邮件流写入数据。
func (w *mimeLineWriter) Write(data []byte) (int, error) {
	// 1. 按剩余行宽分块写入并在满行时追加 CRLF。
	written := 0
	for len(data) > 0 {
		remaining := w.width - w.column
		if remaining <= 0 {
			if _, err := io.WriteString(w.writer, "\r\n"); err != nil {
				return written, err
			}
			w.column = 0
			remaining = w.width
		}
		count := min(remaining, len(data))
		if _, err := w.writer.Write(data[:count]); err != nil {
			return written, err
		}
		written += count
		w.column += count
		data = data[count:]
	}
	return written, nil
}
