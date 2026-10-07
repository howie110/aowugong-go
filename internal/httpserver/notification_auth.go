package httpserver

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
)

type notificationCredential struct {
	project string
	digest  [sha256.Size]byte
}

// notificationCredentials 将启动配置快照为哈希，不让处理器保留明文 Token。
func notificationCredentials(tokens map[string]string) []notificationCredential {
	credentials := make([]notificationCredential, 0, len(tokens))
	for project, token := range tokens {
		credentials = append(credentials, notificationCredential{project: project, digest: sha256.Sum256([]byte(token))})
	}
	return credentials
}

func authenticateNotification(r *http.Request, credentials []notificationCredential) string {
	if len(r.Header.Values("Authorization")) != 1 {
		return ""
	}
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || len(parts[1]) < 32 || len(parts[1]) > 256 {
		return ""
	}
	digest := sha256.Sum256([]byte(parts[1]))
	project := ""
	for _, credential := range credentials {
		if subtle.ConstantTimeCompare(digest[:], credential.digest[:]) == 1 {
			project = credential.project
		}
	}
	return project
}
