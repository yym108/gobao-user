package grpc

import (
	"net/http"
	"path"
	"strings"
)

// RegisterAvatarHTTP 在 HTTP mux 上挂载头像静态文件服务。
// 头像文件由 user 服务自有存储管理，通过该静态前缀对外暴露，与 avatar_url 中的前缀一致。
//   - mux:           HTTP 多路复用器
//   - avatarBaseURL: 对外访问前缀，例如 /avatars
//   - avatarRoot:    头像文件根目录
func RegisterAvatarHTTP(mux *http.ServeMux, avatarBaseURL string, avatarRoot string) {
	if mux == nil || avatarRoot == "" {
		return
	}
	// 前缀属于 URL 路径，需用 path.Clean 处理，避免 "/avatars" 被误清洗成 "//avatars"。
	cleanBase := path.Clean("/" + strings.TrimSpace(avatarBaseURL))
	if cleanBase == "." || cleanBase == "/" {
		cleanBase = "/avatars"
	}
	fs := http.FileServer(http.Dir(avatarRoot))
	mux.Handle(cleanBase+"/", http.StripPrefix(cleanBase+"/", fs))
}
