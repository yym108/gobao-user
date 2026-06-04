// Package application 定义地址簿用例的输入命令对象。
package application

// CreateAddressCommand 表示创建地址时由传输层传入的字段集合。
type CreateAddressCommand struct {
	ReceiverName  string // 收货人姓名
	ReceiverPhone string // 收货人手机号
	Province      string // 省
	City          string // 市
	District      string // 区
	AddressLine   string // 详细地址
	PostalCode    string // 邮编
	IsDefault     bool   // 是否设为默认地址
}

// UpdateAddressCommand 表示更新地址时可修改的字段集合。
type UpdateAddressCommand struct {
	ReceiverName  string // 收货人姓名
	ReceiverPhone string // 收货人手机号
	Province      string // 省
	City          string // 市
	District      string // 区
	AddressLine   string // 详细地址
	PostalCode    string // 邮编
	IsDefault     bool   // 是否设为默认地址
}
