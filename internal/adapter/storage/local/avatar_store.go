// Package local 提供 User 服务的本地文件存储实现。
// 当前仅用于用户头像：物理文件保存在 user 服务独占挂载目录，公开 URL 与内部存储路径解耦。
package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// AvatarStore 是本地文件系统头像存储实现。
type AvatarStore struct {
	rootDir string // 本地文件根目录
	baseURL string // 对外访问前缀，例如 /avatars
}

// NewAvatarStore 构造本地头像存储。
//   - rootDir: 头像文件根目录
//   - baseURL: 对外访问前缀，空值回退为 /avatars
func NewAvatarStore(rootDir string, baseURL string) *AvatarStore {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = "/avatars"
	}
	return &AvatarStore{
		rootDir: filepath.Clean(rootDir),
		baseURL: "/" + strings.Trim(baseURL, "/"),
	}
}

// Save 保存头像文件并返回存储键与公开 URL。
func (s *AvatarStore) Save(_ context.Context, folder string, originalName string, payload []byte) (string, string, error) {
	if len(payload) == 0 {
		return "", "", fmt.Errorf("payload is empty")
	}
	ext := filepath.Ext(strings.TrimSpace(originalName))
	fileName := fmt.Sprintf("%d-%d%s", time.Now().Unix(), time.Now().UnixNano()%1000000, ext)
	cleanFolder := strings.Trim(filepath.ToSlash(filepath.Clean(folder)), "/.")
	storageKey := filepath.ToSlash(filepath.Join(cleanFolder, fileName))
	targetDir := filepath.Join(s.rootDir, filepath.FromSlash(cleanFolder))
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", "", err
	}
	targetPath := filepath.Join(s.rootDir, filepath.FromSlash(storageKey))
	if err := os.WriteFile(targetPath, payload, 0o644); err != nil {
		return "", "", err
	}
	publicURL := s.baseURL + "/" + strings.TrimLeft(storageKey, "/")
	return storageKey, publicURL, nil
}
