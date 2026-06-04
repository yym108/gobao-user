// Package domain 定义用户领域模型与仓储接口，不依赖任何框架。
package domain

import "time"

// User 是用户领域模型。
// 纯业务对象，不包含 GORM tag 或 proto 类型——框架相关细节由 adapter 层处理。
type User struct {
	ID           int64     // 用户唯一标识（数据库自增主键）
	Email        string    // 邮箱地址（全局唯一，用于登录）
	PasswordHash string    // bcrypt 哈希后的密码（明文密码不进入领域模型）
	Nickname     string    // 昵称
	AvatarURL    string    // 头像地址，本期先保存字符串字段，上传能力后续补齐
	CreatedAt    time.Time // 创建时间
	UpdatedAt    time.Time // 最后更新时间
}
