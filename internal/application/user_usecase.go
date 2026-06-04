// Package application 编排用户业务用例，协调 domain 层和 pkg 层完成具体功能。
package application

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/yym108/gobao-pkg/authn"
	"github.com/yym108/gobao-pkg/errors"
	"github.com/yym108/gobao-user/internal/domain"
)

// AvatarStorage 抽象头像文件存储。
// 由 adapter 层注入具体实现（如本地文件存储），便于用例解耦与单元测试。
type AvatarStorage interface {
	// Save 保存文件并返回存储键与对外公开 URL。
	Save(ctx context.Context, folder, originalName string, payload []byte) (storageKey string, publicURL string, err error)
}

// UserUseCase 是用户业务用例的编排器。
// 依赖 domain.UserRepository（持久化）和 authn.JWTManager（JWT 签发/校验）。
type UserUseCase struct {
	repo  domain.UserRepository // 用户仓储接口
	jwt   *authn.JWTManager     // JWT 管理器
	rdb   *redis.Client         // Redis 客户端，用于验证码暂存
	store AvatarStorage         // 头像文件存储，可为 nil（未配置时上传头像返回错误）
	logf  func(string, ...any)  // 轻量日志函数，用于打印联调验证码
}

// NewUserUseCase 构造用例编排器。
//   - repo:  用户仓储实现（由 adapter 层注入）
//   - jwt:   JWT 管理器（由 main.go 构造并注入）
//   - store: 头像文件存储，允许为 nil（未启用头像上传的环境）
func NewUserUseCase(repo domain.UserRepository, jwt *authn.JWTManager, rdb *redis.Client, store AvatarStorage, logf func(string, ...any)) *UserUseCase {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &UserUseCase{repo: repo, jwt: jwt, rdb: rdb, store: store, logf: logf}
}

// Register 注册新用户。
// 流程：检查邮箱唯一性 → bcrypt 哈希密码 → 写库。
//   - email:    邮箱地址
//   - password: 明文密码
//   - nickname: 昵称
//
// 返回值:
//   - int64: 新用户的 ID
//   - error: 邮箱已注册返回 CodeConflict，其他错误返回 CodeInternal
func (uc *UserUseCase) Register(ctx context.Context, email, password, nickname string) (int64, error) {
	exists, err := uc.repo.ExistsByEmail(ctx, email)
	if err != nil {
		return 0, errors.Wrap(errors.CodeInternal, "check email", err)
	}
	if exists {
		return 0, errors.New(errors.CodeConflict, "email already registered")
	}

	hash, err := authn.HashPassword(password)
	if err != nil {
		return 0, errors.Wrap(errors.CodeInternal, "hash password", err)
	}

	user := &domain.User{
		Email:        email,
		PasswordHash: hash,
		Nickname:     nickname,
	}
	if err := uc.repo.Create(ctx, user); err != nil {
		return 0, errors.Wrap(errors.CodeInternal, "create user", err)
	}
	return user.ID, nil
}

// Login 用户登录。
// 流程：按 email 查用户 → 比对密码 → 签发 JWT。
// 安全设计：用户不存在和密码错误都返回相同的 "invalid credentials"，避免泄露用户是否注册。
//   - email:    邮箱地址
//   - password: 明文密码
//
// 返回值:
//   - token:     JWT 字符串
//   - expiresAt: 过期时间（Unix 秒）
//   - userID:    用户 ID
//   - err:       认证失败返回 CodeUnauth
func (uc *UserUseCase) Login(ctx context.Context, email, password string) (string, int64, int64, error) {
	user, err := uc.repo.FindByEmail(ctx, email)
	if err != nil {
		return "", 0, 0, errors.Wrap(errors.CodeInternal, "find user", err)
	}
	if user == nil {
		return "", 0, 0, errors.New(errors.CodeUnauth, "invalid credentials")
	}

	if err := authn.ComparePassword(user.PasswordHash, password); err != nil {
		return "", 0, 0, errors.New(errors.CodeUnauth, "invalid credentials")
	}

	token, expiresAt, err := uc.jwt.Sign(user.ID, user.Email)
	if err != nil {
		return "", 0, 0, errors.Wrap(errors.CodeInternal, "sign token", err)
	}
	return token, expiresAt, user.ID, nil
}

// VerifyToken 校验 JWT 并返回用户信息。
// 纯本地校验，不查数据库——JWT 是自包含的。
//   - token: JWT 字符串
//
// 返回值:
//   - userID: 用户 ID
//   - email:  用户邮箱
//   - err:    token 无效返回 CodeUnauth
func (uc *UserUseCase) VerifyToken(_ context.Context, token string) (int64, string, error) {
	claims, err := uc.jwt.Verify(token)
	if err != nil {
		return 0, "", errors.New(errors.CodeUnauth, "invalid token")
	}
	return claims.UserID, claims.Email, nil
}

// GetUser 按 ID 获取用户信息。
//   - id: 用户 ID
//
// 返回值:
//   - *domain.User: 用户领域对象
//   - error:        未找到返回 CodeNotFound
func (uc *UserUseCase) GetUser(ctx context.Context, id int64) (*domain.User, error) {
	user, err := uc.repo.FindByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(errors.CodeInternal, "find user", err)
	}
	if user == nil {
		return nil, errors.New(errors.CodeNotFound, "user not found")
	}
	return user, nil
}

// FindUserByEmail 按邮箱精确查找用户，未找到时返回 (nil, nil) 由调用方决定语义。
// 供后台订单按买家邮箱筛选时解析 user_id。
func (uc *UserUseCase) FindUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return nil, errors.New(errors.CodeInvalidArg, "email is required")
	}
	user, err := uc.repo.FindByEmail(ctx, email)
	if err != nil {
		return nil, errors.Wrap(errors.CodeInternal, "find user by email", err)
	}
	return user, nil
}

// 头像上传大小上限：5MB，避免单张头像占用过多存储与带宽。
const maxAvatarBytes = 5 << 20

// validateAvatarUpload 校验上传头像的类型与大小。
// 仅接受图片类型且不超过上限，校验失败返回带语义的非法参数错误。
func validateAvatarUpload(mimeType string, size int) error {
	if size <= 0 {
		return errors.New(errors.CodeInvalidArg, "avatar content is empty")
	}
	if size > maxAvatarBytes {
		return errors.New(errors.CodeInvalidArg, "avatar too large")
	}
	if !strings.HasPrefix(mimeType, "image/") {
		return errors.New(errors.CodeInvalidArg, "avatar must be an image")
	}
	return nil
}

// UploadAvatar 保存裁剪后的头像并回写当前用户的头像地址。
// 文件由 user 服务自有存储管理，落库的 avatar_url 指向 user 服务对外的静态前缀。
func (uc *UserUseCase) UploadAvatar(ctx context.Context, userID int64, fileName, mimeType string, content []byte) (*domain.User, error) {
	if err := validateAvatarUpload(mimeType, len(content)); err != nil {
		return nil, err
	}
	if uc.store == nil {
		return nil, errors.New(errors.CodeInternal, "avatar storage not configured")
	}

	user, err := uc.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	folder := fmt.Sprintf("avatars/%d", userID)
	_, publicURL, err := uc.store.Save(ctx, folder, fileName, content)
	if err != nil {
		return nil, errors.Wrap(errors.CodeInternal, "save avatar", err)
	}
	if err := uc.repo.UpdateProfile(ctx, userID, user.Nickname, publicURL); err != nil {
		return nil, errors.Wrap(errors.CodeInternal, "update avatar url", err)
	}
	user.AvatarURL = publicURL
	return user, nil
}

// UpdateProfile 更新当前用户的昵称与头像地址。
func (uc *UserUseCase) UpdateProfile(ctx context.Context, userID int64, nickname, avatarURL string) (*domain.User, error) {
	nickname = strings.TrimSpace(nickname)
	avatarURL = strings.TrimSpace(avatarURL)
	if len([]rune(nickname)) < 2 || len([]rune(nickname)) > 20 {
		return nil, errors.New(errors.CodeInvalidArg, "nickname must be 2-20 characters")
	}
	if len(avatarURL) > 500 {
		return nil, errors.New(errors.CodeInvalidArg, "avatar_url too long")
	}

	user, err := uc.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := uc.repo.UpdateProfile(ctx, userID, nickname, avatarURL); err != nil {
		return nil, errors.Wrap(errors.CodeInternal, "update profile", err)
	}
	user.Nickname = nickname
	user.AvatarURL = avatarURL
	return user, nil
}

// ListAddresses 返回当前用户地址簿列表。
func (uc *UserUseCase) ListAddresses(ctx context.Context, userID int64) ([]*domain.Address, error) {
	if _, err := uc.GetUser(ctx, userID); err != nil {
		return nil, err
	}
	addresses, err := uc.repo.ListAddresses(ctx, userID)
	if err != nil {
		return nil, errors.Wrap(errors.CodeInternal, "list addresses", err)
	}
	return addresses, nil
}

// GetAddress 返回当前用户的一条地址记录，并校验地址归属。
func (uc *UserUseCase) GetAddress(ctx context.Context, userID, addressID int64) (*domain.Address, error) {
	if userID <= 0 || addressID <= 0 {
		return nil, errors.New(errors.CodeInvalidArg, "user_id and address_id must be positive")
	}
	address, err := uc.repo.FindAddressByID(ctx, addressID)
	if err != nil {
		return nil, errors.Wrap(errors.CodeInternal, "find address", err)
	}
	if address == nil {
		return nil, errors.New(errors.CodeNotFound, "address not found")
	}
	if address.UserID != userID {
		return nil, errors.New(errors.CodeForbidden, "address does not belong to user")
	}
	return address, nil
}

// CreateAddress 创建用户地址，并处理“首地址自动默认”与“显式默认清空旧默认”规则。
func (uc *UserUseCase) CreateAddress(ctx context.Context, userID int64, cmd CreateAddressCommand) (*domain.Address, error) {
	if _, err := uc.GetUser(ctx, userID); err != nil {
		return nil, err
	}
	address, err := uc.buildAddress(userID, cmd.ReceiverName, cmd.ReceiverPhone, cmd.Province, cmd.City, cmd.District, cmd.AddressLine, cmd.PostalCode, cmd.IsDefault)
	if err != nil {
		return nil, err
	}

	count, err := uc.repo.CountAddressesByUserID(ctx, userID)
	if err != nil {
		return nil, errors.Wrap(errors.CodeInternal, "count addresses", err)
	}
	if count == 0 {
		address.IsDefault = true
	}
	if address.IsDefault {
		if err := uc.repo.ClearDefaultAddresses(ctx, userID); err != nil {
			return nil, errors.Wrap(errors.CodeInternal, "clear default addresses", err)
		}
	}
	if err := uc.repo.CreateAddress(ctx, address); err != nil {
		return nil, errors.Wrap(errors.CodeInternal, "create address", err)
	}
	return address, nil
}

// UpdateAddress 更新用户地址，并在需要时切换默认地址。
func (uc *UserUseCase) UpdateAddress(ctx context.Context, userID, addressID int64, cmd UpdateAddressCommand) (*domain.Address, error) {
	existing, err := uc.GetAddress(ctx, userID, addressID)
	if err != nil {
		return nil, err
	}
	address, err := uc.buildAddress(userID, cmd.ReceiverName, cmd.ReceiverPhone, cmd.Province, cmd.City, cmd.District, cmd.AddressLine, cmd.PostalCode, cmd.IsDefault)
	if err != nil {
		return nil, err
	}
	address.ID = existing.ID
	address.CreatedAt = existing.CreatedAt
	if existing.IsDefault {
		address.IsDefault = true
	}
	if cmd.IsDefault {
		address.IsDefault = true
	}
	if address.IsDefault {
		if err := uc.repo.ClearDefaultAddresses(ctx, userID); err != nil {
			return nil, errors.Wrap(errors.CodeInternal, "clear default addresses", err)
		}
	}
	if err := uc.repo.UpdateAddress(ctx, address); err != nil {
		return nil, errors.Wrap(errors.CodeInternal, "update address", err)
	}
	return uc.GetAddress(ctx, userID, addressID)
}

// DeleteAddress 删除当前用户地址。
func (uc *UserUseCase) DeleteAddress(ctx context.Context, userID, addressID int64) error {
	if _, err := uc.GetAddress(ctx, userID, addressID); err != nil {
		return err
	}
	if err := uc.repo.DeleteAddress(ctx, addressID); err != nil {
		return errors.Wrap(errors.CodeInternal, "delete address", err)
	}
	return nil
}

// SetDefaultAddress 将指定地址设置为默认地址。
func (uc *UserUseCase) SetDefaultAddress(ctx context.Context, userID, addressID int64) (*domain.Address, error) {
	address, err := uc.GetAddress(ctx, userID, addressID)
	if err != nil {
		return nil, err
	}
	if err := uc.repo.ClearDefaultAddresses(ctx, userID); err != nil {
		return nil, errors.Wrap(errors.CodeInternal, "clear default addresses", err)
	}
	address.IsDefault = true
	if err := uc.repo.UpdateAddress(ctx, address); err != nil {
		return nil, errors.Wrap(errors.CodeInternal, "set default address", err)
	}
	return uc.GetAddress(ctx, userID, addressID)
}

// SendPasswordResetCode 为当前用户生成并保存邮箱验证码。
// 本期验证码不走真实邮件，而是写 Redis 并输出到日志，方便前后端联调。
func (uc *UserUseCase) SendPasswordResetCode(ctx context.Context, userID int64) error {
	if uc.rdb == nil {
		return errors.New(errors.CodeInternal, "redis not configured")
	}

	user, err := uc.GetUser(ctx, userID)
	if err != nil {
		return err
	}

	cooldownKey := fmt.Sprintf("user:password_reset_code_cooldown:%d", userID)
	exists, err := uc.rdb.Exists(ctx, cooldownKey).Result()
	if err != nil {
		return errors.Wrap(errors.CodeInternal, "check code cooldown", err)
	}
	if exists > 0 {
		return errors.New(errors.CodeConflict, "password reset code requested too frequently")
	}

	code, err := randomDigits(6)
	if err != nil {
		return errors.Wrap(errors.CodeInternal, "generate password reset code", err)
	}

	codeKey := fmt.Sprintf("user:password_reset_code:%d", userID)
	if err := uc.rdb.Set(ctx, codeKey, code, 5*time.Minute).Err(); err != nil {
		return errors.Wrap(errors.CodeInternal, "store password reset code", err)
	}
	if err := uc.rdb.Set(ctx, cooldownKey, "1", time.Minute).Err(); err != nil {
		return errors.Wrap(errors.CodeInternal, "store password reset cooldown", err)
	}
	uc.logf("password reset code for %s(user_id=%d): %s", user.Email, userID, code)
	return nil
}

// PeekPasswordResetCode 读取当前用户待用的改密验证码。
// 仅供开发/演示环境联调：验证码并未真正发送邮件，这里直接从 Redis 读回，
// 生产环境必须关闭该能力，否则会绕过邮箱验证带来账号安全风险。
func (uc *UserUseCase) PeekPasswordResetCode(ctx context.Context, userID int64) (string, error) {
	if uc.rdb == nil {
		return "", errors.New(errors.CodeInternal, "redis not configured")
	}
	codeKey := fmt.Sprintf("user:password_reset_code:%d", userID)
	code, err := uc.rdb.Get(ctx, codeKey).Result()
	if err == redis.Nil {
		return "", errors.New(errors.CodeNotFound, "password reset code not found or expired")
	}
	if err != nil {
		return "", errors.Wrap(errors.CodeInternal, "read password reset code", err)
	}
	return code, nil
}

// SendPasswordResetCodeByEmail 为未登录找回密码流程按邮箱生成并保存验证码。
func (uc *UserUseCase) SendPasswordResetCodeByEmail(ctx context.Context, email string) error {
	if uc.rdb == nil {
		return errors.New(errors.CodeInternal, "redis not configured")
	}

	email = strings.TrimSpace(email)
	if email == "" {
		return errors.New(errors.CodeInvalidArg, "email is required")
	}

	user, err := uc.repo.FindByEmail(ctx, email)
	if err != nil {
		return errors.Wrap(errors.CodeInternal, "find user", err)
	}
	if user == nil {
		return errors.New(errors.CodeNotFound, "user not found")
	}

	cooldownKey := fmt.Sprintf("user:password_reset_code_email_cooldown:%s", email)
	exists, err := uc.rdb.Exists(ctx, cooldownKey).Result()
	if err != nil {
		return errors.Wrap(errors.CodeInternal, "check code cooldown", err)
	}
	if exists > 0 {
		return errors.New(errors.CodeConflict, "password reset code requested too frequently")
	}

	code, err := randomDigits(6)
	if err != nil {
		return errors.Wrap(errors.CodeInternal, "generate password reset code", err)
	}

	codeKey := fmt.Sprintf("user:password_reset_code_email:%s", email)
	if err := uc.rdb.Set(ctx, codeKey, code, 5*time.Minute).Err(); err != nil {
		return errors.Wrap(errors.CodeInternal, "store password reset code", err)
	}
	if err := uc.rdb.Set(ctx, cooldownKey, "1", time.Minute).Err(); err != nil {
		return errors.Wrap(errors.CodeInternal, "store password reset cooldown", err)
	}
	uc.logf("password reset code for %s(email_flow): %s", email, code)
	return nil
}

// ChangePassword 校验邮箱验证码后更新当前用户密码。
func (uc *UserUseCase) ChangePassword(ctx context.Context, userID int64, code, newPassword string) error {
	code = strings.TrimSpace(code)
	newPassword = strings.TrimSpace(newPassword)
	if len(code) != 6 {
		return errors.New(errors.CodeInvalidArg, "code must be 6 digits")
	}
	if len(newPassword) < 6 {
		return errors.New(errors.CodeInvalidArg, "password must be at least 6 characters")
	}
	if uc.rdb == nil {
		return errors.New(errors.CodeInternal, "redis not configured")
	}

	codeKey := fmt.Sprintf("user:password_reset_code:%d", userID)
	storedCode, err := uc.rdb.Get(ctx, codeKey).Result()
	if err != nil {
		if err == redis.Nil {
			return errors.New(errors.CodeInvalidArg, "password reset code expired")
		}
		return errors.Wrap(errors.CodeInternal, "load password reset code", err)
	}
	if storedCode != code {
		return errors.New(errors.CodeInvalidArg, "invalid password reset code")
	}

	user, err := uc.GetUser(ctx, userID)
	if err != nil {
		return err
	}
	if err := authn.ComparePassword(user.PasswordHash, newPassword); err == nil {
		return errors.New(errors.CodeInvalidArg, "new password must differ from current password")
	}

	hash, err := authn.HashPassword(newPassword)
	if err != nil {
		return errors.Wrap(errors.CodeInternal, "hash password", err)
	}
	if err := uc.repo.UpdatePasswordHash(ctx, userID, hash); err != nil {
		return errors.Wrap(errors.CodeInternal, "update password hash", err)
	}
	if err := uc.rdb.Del(ctx, codeKey).Err(); err != nil {
		return errors.Wrap(errors.CodeInternal, "delete password reset code", err)
	}
	return nil
}

// ResetPasswordByEmail 校验邮箱验证码后按邮箱重置密码。
func (uc *UserUseCase) ResetPasswordByEmail(ctx context.Context, email, code, newPassword string) error {
	email = strings.TrimSpace(email)
	code = strings.TrimSpace(code)
	newPassword = strings.TrimSpace(newPassword)
	if email == "" {
		return errors.New(errors.CodeInvalidArg, "email is required")
	}
	if len(code) != 6 {
		return errors.New(errors.CodeInvalidArg, "code must be 6 digits")
	}
	if len(newPassword) < 6 {
		return errors.New(errors.CodeInvalidArg, "password must be at least 6 characters")
	}
	if uc.rdb == nil {
		return errors.New(errors.CodeInternal, "redis not configured")
	}

	user, err := uc.repo.FindByEmail(ctx, email)
	if err != nil {
		return errors.Wrap(errors.CodeInternal, "find user", err)
	}
	if user == nil {
		return errors.New(errors.CodeNotFound, "user not found")
	}

	codeKey := fmt.Sprintf("user:password_reset_code_email:%s", email)
	storedCode, err := uc.rdb.Get(ctx, codeKey).Result()
	if err != nil {
		if err == redis.Nil {
			return errors.New(errors.CodeInvalidArg, "password reset code expired")
		}
		return errors.Wrap(errors.CodeInternal, "load password reset code", err)
	}
	if storedCode != code {
		return errors.New(errors.CodeInvalidArg, "invalid password reset code")
	}
	if err := authn.ComparePassword(user.PasswordHash, newPassword); err == nil {
		return errors.New(errors.CodeInvalidArg, "new password must differ from current password")
	}

	hash, err := authn.HashPassword(newPassword)
	if err != nil {
		return errors.Wrap(errors.CodeInternal, "hash password", err)
	}
	if err := uc.repo.UpdatePasswordHash(ctx, user.ID, hash); err != nil {
		return errors.Wrap(errors.CodeInternal, "update password hash", err)
	}
	if err := uc.rdb.Del(ctx, codeKey).Err(); err != nil {
		return errors.Wrap(errors.CodeInternal, "delete password reset code", err)
	}
	return nil
}

// randomDigits 生成固定长度的数字验证码。
func randomDigits(length int) (string, error) {
	if length <= 0 {
		return "", nil
	}
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	for i := range buf {
		buf[i] = '0' + (buf[i] % 10)
	}
	return string(buf), nil
}

// buildAddress 统一校验并组装地址对象，避免创建与更新逻辑重复。
func (uc *UserUseCase) buildAddress(userID int64, receiverName, receiverPhone, province, city, district, addressLine, postalCode string, isDefault bool) (*domain.Address, error) {
	receiverName = strings.TrimSpace(receiverName)
	receiverPhone = strings.TrimSpace(receiverPhone)
	province = strings.TrimSpace(province)
	city = strings.TrimSpace(city)
	district = strings.TrimSpace(district)
	addressLine = strings.TrimSpace(addressLine)
	postalCode = strings.TrimSpace(postalCode)

	if userID <= 0 {
		return nil, errors.New(errors.CodeInvalidArg, "user_id must be positive")
	}
	if receiverName == "" {
		return nil, errors.New(errors.CodeInvalidArg, "receiver_name is required")
	}
	if receiverPhone == "" {
		return nil, errors.New(errors.CodeInvalidArg, "receiver_phone is required")
	}
	if province == "" || city == "" {
		return nil, errors.New(errors.CodeInvalidArg, "province and city are required")
	}
	if addressLine == "" {
		return nil, errors.New(errors.CodeInvalidArg, "address_line is required")
	}

	return &domain.Address{
		UserID:        userID,
		ReceiverName:  receiverName,
		ReceiverPhone: receiverPhone,
		Province:      province,
		City:          city,
		District:      district,
		AddressLine:   addressLine,
		PostalCode:    postalCode,
		IsDefault:     isDefault,
	}, nil
}
