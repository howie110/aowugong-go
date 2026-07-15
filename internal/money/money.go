// Package money 提供项目内统一的十进制金额解析和格式化。
package money

import (
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

var ErrInvalid = errors.New("金额格式无效")

// ParseCents 把十进制金额按 ROUND_HALF_UP 转换为分。
// 输入：value 是十进制金额文本，空值按零处理。
// 输出：返回整数分；格式或范围无效时返回 ErrInvalid。
// 副作用：无。
func ParseCents(value string) (int64, error) {
	// 1. 使用有理数精确解析十进制文本。
	value = strings.TrimSpace(value)
	if value == "" {
		value = "0"
	}
	rational, ok := new(big.Rat).SetString(value)
	if !ok {
		return 0, ErrInvalid
	}
	rational.Mul(rational, big.NewRat(100, 1))

	// 2. 对绝对余数执行四舍五入并保留原符号。
	quotient := new(big.Int)
	remainder := new(big.Int)
	quotient.QuoRem(rational.Num(), rational.Denom(), remainder)
	doubledRemainder := new(big.Int).Lsh(new(big.Int).Abs(remainder), 1)
	if doubledRemainder.Cmp(rational.Denom()) >= 0 {
		if rational.Sign() >= 0 {
			quotient.Add(quotient, big.NewInt(1))
		} else {
			quotient.Sub(quotient, big.NewInt(1))
		}
	}
	if !quotient.IsInt64() {
		return 0, ErrInvalid
	}
	return quotient.Int64(), nil
}

// FormatCents 把整数分格式化为两位小数金额。
// 输入：cents 是金额分，grouped 控制是否添加千分位。
// 输出：返回金额文本。
// 副作用：无。
func FormatCents(cents int64, grouped bool) string {
	// 1. 分离符号、整数和小数部分。
	negative := cents < 0
	if negative {
		cents = -cents
	}
	integerPart := strconv.FormatInt(cents/100, 10)
	if grouped {
		for index := len(integerPart) - 3; index > 0; index -= 3 {
			integerPart = integerPart[:index] + "," + integerPart[index:]
		}
	}
	result := fmt.Sprintf("%s.%02d", integerPart, cents%100)
	if negative {
		return "-" + result
	}
	return result
}
