// Package id 为基础设施边界提供随机 UUIDv4 字符串。
package id

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
)

// New 返回随机生成的小写 UUIDv4 字符串。
func New() (string, error) {
	var value [16]byte
	if _, err := io.ReadFull(rand.Reader, value[:]); err != nil {
		return "", fmt.Errorf("read random UUID bytes: %w", err)
	}

	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80

	return format(value), nil
}

func format(value [16]byte) string {
	var result [36]byte
	hex.Encode(result[0:8], value[0:4])
	result[8] = '-'
	hex.Encode(result[9:13], value[4:6])
	result[13] = '-'
	hex.Encode(result[14:18], value[6:8])
	result[18] = '-'
	hex.Encode(result[19:23], value[8:10])
	result[23] = '-'
	hex.Encode(result[24:36], value[10:16])
	return string(result[:])
}
