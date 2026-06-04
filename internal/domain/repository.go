package domain

import "context"

// UserRepository 定义用户持久化的接口，由 adapter/repository 层实现。
// FindByID/FindByEmail 未找到时返回 (nil, nil)，由 application 层决定如何处理。
type UserRepository interface {
	// Create 创建新用户，成功后回写 user.ID、user.CreatedAt、user.UpdatedAt。
	Create(ctx context.Context, user *User) error

	// FindByID 按用户 ID 查找。未找到返回 (nil, nil)。
	FindByID(ctx context.Context, id int64) (*User, error)

	// FindByEmail 按邮箱查找。未找到返回 (nil, nil)。
	FindByEmail(ctx context.Context, email string) (*User, error)

	// ExistsByEmail 检查邮箱是否已注册。
	ExistsByEmail(ctx context.Context, email string) (bool, error)

	// UpdateProfile 更新用户昵称与头像地址。
	UpdateProfile(ctx context.Context, userID int64, nickname, avatarURL string) error

	// UpdatePasswordHash 更新用户密码哈希。
	UpdatePasswordHash(ctx context.Context, userID int64, passwordHash string) error

	// ListAddresses 按用户查询地址簿列表。
	ListAddresses(ctx context.Context, userID int64) ([]*Address, error)

	// FindAddressByID 按地址 ID 查询地址。
	FindAddressByID(ctx context.Context, addressID int64) (*Address, error)

	// CountAddressesByUserID 统计用户当前地址数量。
	CountAddressesByUserID(ctx context.Context, userID int64) (int64, error)

	// CreateAddress 创建地址记录。
	CreateAddress(ctx context.Context, address *Address) error

	// UpdateAddress 更新地址记录。
	UpdateAddress(ctx context.Context, address *Address) error

	// DeleteAddress 删除地址记录。
	DeleteAddress(ctx context.Context, addressID int64) error

	// ClearDefaultAddresses 清除当前用户全部默认地址标记。
	ClearDefaultAddresses(ctx context.Context, userID int64) error
}
