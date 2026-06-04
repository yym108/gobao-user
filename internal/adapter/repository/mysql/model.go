// Package mysql 提供基于 GORM 的用户仓储实现。
package mysql

import (
	"time"

	"github.com/yym108/gobao-user/internal/domain"
)

// UserModel 是 GORM 模型，对应数据库 users 表。
// 包含 GORM tag 定义表结构约束；领域模型 domain.User 不含这些 tag。
type UserModel struct {
	ID           int64     `gorm:"primaryKey;autoIncrement"`      // 主键，自增
	Email        string    `gorm:"uniqueIndex;size:255;not null"` // 邮箱，唯一索引
	PasswordHash string    `gorm:"size:255;not null"`             // bcrypt 哈希后的密码
	Nickname     string    `gorm:"size:100;not null;default:''"`  // 昵称，默认空串
	AvatarURL    string    `gorm:"size:500;not null;default:''"`  // 头像地址，本期先保存 URL 字符串
	CreatedAt    time.Time // 创建时间（GORM 自动管理）
	UpdatedAt    time.Time // 更新时间（GORM 自动管理）
}

// TableName 指定 GORM 使用的表名。
func (UserModel) TableName() string { return "users" }

// AddressModel 是 GORM 地址簿模型，对应数据库 user_addresses 表。
type AddressModel struct {
	ID            int64     `gorm:"primaryKey;autoIncrement"`     // 主键，自增
	UserID        int64     `gorm:"index;not null"`               // 所属用户 ID
	ReceiverName  string    `gorm:"size:100;not null"`            // 收货人姓名
	ReceiverPhone string    `gorm:"size:32;not null"`             // 收货人手机号
	Province      string    `gorm:"size:100;not null"`            // 省
	City          string    `gorm:"size:100;not null"`            // 市
	District      string    `gorm:"size:100;not null;default:''"` // 区
	AddressLine   string    `gorm:"size:255;not null"`            // 详细地址
	PostalCode    string    `gorm:"size:32;not null;default:''"`  // 邮编
	IsDefault     bool      `gorm:"not null;default:false"`       // 是否默认地址
	CreatedAt     time.Time // 创建时间
	UpdatedAt     time.Time // 更新时间
}

// TableName 指定地址模型的表名。
func (AddressModel) TableName() string { return "user_addresses" }

// toDomain 将 GORM 模型转换为领域模型。
func toDomain(m *UserModel) *domain.User {
	return &domain.User{
		ID:           m.ID,
		Email:        m.Email,
		PasswordHash: m.PasswordHash,
		Nickname:     m.Nickname,
		AvatarURL:    m.AvatarURL,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}

// toModel 将领域模型转换为 GORM 模型。
func toModel(u *domain.User) *UserModel {
	return &UserModel{
		ID:           u.ID,
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
		Nickname:     u.Nickname,
		AvatarURL:    u.AvatarURL,
		CreatedAt:    u.CreatedAt,
		UpdatedAt:    u.UpdatedAt,
	}
}

// toAddressDomain 将 GORM 地址模型转换为领域地址对象。
func toAddressDomain(m *AddressModel) *domain.Address {
	return &domain.Address{
		ID:            m.ID,
		UserID:        m.UserID,
		ReceiverName:  m.ReceiverName,
		ReceiverPhone: m.ReceiverPhone,
		Province:      m.Province,
		City:          m.City,
		District:      m.District,
		AddressLine:   m.AddressLine,
		PostalCode:    m.PostalCode,
		IsDefault:     m.IsDefault,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
	}
}

// toAddressModel 将领域地址对象转换为 GORM 地址模型。
func toAddressModel(address *domain.Address) *AddressModel {
	return &AddressModel{
		ID:            address.ID,
		UserID:        address.UserID,
		ReceiverName:  address.ReceiverName,
		ReceiverPhone: address.ReceiverPhone,
		Province:      address.Province,
		City:          address.City,
		District:      address.District,
		AddressLine:   address.AddressLine,
		PostalCode:    address.PostalCode,
		IsDefault:     address.IsDefault,
		CreatedAt:     address.CreatedAt,
		UpdatedAt:     address.UpdatedAt,
	}
}
