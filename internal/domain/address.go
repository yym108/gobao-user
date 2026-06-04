// Package domain 定义用户地址簿领域对象。
package domain

import "time"

// Address 表示用户地址簿中的一条收货地址。
// 该对象只表达业务语义，不携带任何持久化或传输层标签。
type Address struct {
	ID            int64     // 地址主键
	UserID        int64     // 所属用户 ID
	ReceiverName  string    // 收货人姓名
	ReceiverPhone string    // 收货人手机号
	Province      string    // 省
	City          string    // 市
	District      string    // 区
	AddressLine   string    // 详细地址
	PostalCode    string    // 邮编
	IsDefault     bool      // 是否默认地址
	CreatedAt     time.Time // 创建时间
	UpdatedAt     time.Time // 更新时间
}
