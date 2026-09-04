// Package id 为基础设施边界提供时间有序 UUIDv7 字符串。
package id

import "uuid"

// New 返回由 Go 标准库生成的小写 UUIDv7 字符串。
func New() string {
	return uuid.NewV7().String()
}
