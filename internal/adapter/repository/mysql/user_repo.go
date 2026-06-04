package mysql

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/yym108/gobao-user/internal/domain"
)

// UserRepo 是 domain.UserRepository 的 GORM/MySQL 实现。
type UserRepo struct {
	db *gorm.DB // GORM 数据库连接实例
}

// NewUserRepo 构造 UserRepo。
//   - db: 已初始化的 GORM 数据库连接
func NewUserRepo(db *gorm.DB) *UserRepo {
	return &UserRepo{db: db}
}

// Create 创建新用户记录。
// 成功后将数据库自增 ID、CreatedAt、UpdatedAt 回写到 user 对象。
//   - user: 待创建的用户（ID 字段在写库后被填充）
func (r *UserRepo) Create(ctx context.Context, user *domain.User) error {
	m := toModel(user)
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return err
	}
	user.ID = m.ID
	user.CreatedAt = m.CreatedAt
	user.UpdatedAt = m.UpdatedAt
	return nil
}

// FindByID 按用户 ID 查找。
// 未找到时返回 (nil, nil)，由 application 层决定是否视为错误。
//   - id: 用户 ID
func (r *UserRepo) FindByID(ctx context.Context, id int64) (*domain.User, error) {
	var m UserModel
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // 未找到不视为错误
		}
		return nil, err
	}
	return toDomain(&m), nil
}

// FindByEmail 按邮箱查找用户。
// 未找到时返回 (nil, nil)。
//   - email: 邮箱地址
func (r *UserRepo) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	var m UserModel
	if err := r.db.WithContext(ctx).Where("email = ?", email).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toDomain(&m), nil
}

// ExistsByEmail 检查邮箱是否已存在。
// 使用 COUNT 查询，比 First 更高效（不需要读取完整行）���
//   - email: 邮箱地址
func (r *UserRepo) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&UserModel{}).Where("email = ?", email).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// UpdateProfile 更新用户昵称与头像地址。
func (r *UserRepo) UpdateProfile(ctx context.Context, userID int64, nickname, avatarURL string) error {
	return r.db.WithContext(ctx).
		Model(&UserModel{}).
		Where("id = ?", userID).
		Updates(map[string]any{
			"nickname":   nickname,
			"avatar_url": avatarURL,
		}).Error
}

// UpdatePasswordHash 更新用户密码哈希。
func (r *UserRepo) UpdatePasswordHash(ctx context.Context, userID int64, passwordHash string) error {
	return r.db.WithContext(ctx).
		Model(&UserModel{}).
		Where("id = ?", userID).
		Update("password_hash", passwordHash).Error
}

// ListAddresses 查询当前用户的地址簿列表。
func (r *UserRepo) ListAddresses(ctx context.Context, userID int64) ([]*domain.Address, error) {
	var models []*AddressModel
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("is_default DESC, id ASC").
		Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]*domain.Address, 0, len(models))
	for _, model := range models {
		result = append(result, toAddressDomain(model))
	}
	return result, nil
}

// FindAddressByID 按地址 ID 查询地址。
func (r *UserRepo) FindAddressByID(ctx context.Context, addressID int64) (*domain.Address, error) {
	var model AddressModel
	if err := r.db.WithContext(ctx).Where("id = ?", addressID).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toAddressDomain(&model), nil
}

// CountAddressesByUserID 统计用户地址数量。
func (r *UserRepo) CountAddressesByUserID(ctx context.Context, userID int64) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&AddressModel{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// CreateAddress 创建地址记录。
func (r *UserRepo) CreateAddress(ctx context.Context, address *domain.Address) error {
	model := toAddressModel(address)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	address.ID = model.ID
	address.CreatedAt = model.CreatedAt
	address.UpdatedAt = model.UpdatedAt
	return nil
}

// UpdateAddress 更新地址记录。
func (r *UserRepo) UpdateAddress(ctx context.Context, address *domain.Address) error {
	return r.db.WithContext(ctx).
		Model(&AddressModel{}).
		Where("id = ?", address.ID).
		Updates(map[string]any{
			"receiver_name":  address.ReceiverName,
			"receiver_phone": address.ReceiverPhone,
			"province":       address.Province,
			"city":           address.City,
			"district":       address.District,
			"address_line":   address.AddressLine,
			"postal_code":    address.PostalCode,
			"is_default":     address.IsDefault,
		}).Error
}

// DeleteAddress 删除地址记录。
func (r *UserRepo) DeleteAddress(ctx context.Context, addressID int64) error {
	return r.db.WithContext(ctx).Delete(&AddressModel{}, addressID).Error
}

// ClearDefaultAddresses 清空当前用户所有默认地址标记。
func (r *UserRepo) ClearDefaultAddresses(ctx context.Context, userID int64) error {
	return r.db.WithContext(ctx).
		Model(&AddressModel{}).
		Where("user_id = ?", userID).
		Update("is_default", false).Error
}
