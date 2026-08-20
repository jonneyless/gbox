package gbox

import (
	"fmt"
	"strings"
)

var (
	digits = []string{"零", "一", "二", "三", "四", "五", "六", "七", "八", "九"}
	units  = []string{"", "十", "百", "千", "万", "亿"}
)

// NumToChinese 将数字转为中文小写数字（支持 0 ~ 99999999）
func NumToChinese(n int) string {
	if n == 0 {
		return "零"
	}

	// 处理负数
	negative := false
	if n < 0 {
		negative = true
		n = -n
	}

	// 按四位一组分割
	parts := []string{}
	for n > 0 {
		parts = append(parts, formatFourDigits(n%10000))
		n /= 10000
	}

	// 拼接万、亿单位
	result := ""
	for i, part := range parts {
		if part == "" {
			continue
		}

		// 处理中间的零
		if i > 0 && len(part) < 4 && !strings.HasPrefix(part, "零") {
			result = "零" + result
		}

		// 特殊处理：如果当前部分以"一十"开头且是最高位，去掉"一"
		unit := ""
		if i > 0 {
			unit = units[i+1]
		}

		// 修正：最高位的"一十"变成"十"
		if i == len(parts)-1 && strings.HasPrefix(part, "一十") {
			part = strings.TrimPrefix(part, "一")
		}

		result = part + unit + result
	}

	if negative {
		result = "负" + result
	}
	return result
}

// formatFourDigits 处理四位以内的数字
func formatFourDigits(n int) string {
	if n == 0 {
		return ""
	}

	result := ""
	zeroFlag := false

	for i := 3; i >= 0; i-- {
		factor := pow10(i)
		digit := n / factor
		n %= factor

		if digit == 0 {
			if i > 0 && !zeroFlag && result != "" {
				zeroFlag = true
			}
			continue
		}

		if zeroFlag {
			result += "零"
			zeroFlag = false
		}

		result += digits[digit]
		if i > 0 {
			result += units[i]
		}
	}

	return result
}

func pow10(n int) int {
	result := 1
	for i := 0; i < n; i++ {
		result *= 10
	}
	return result
}

func main() {
	testCases := []int{1, 10, 11, 15, 100, 101, 110, 111, 123, 1000, 1001, 1010, 1100, 1234, 10000, 10001, 10010, 11000, 100000000}
	for _, n := range testCases {
		fmt.Printf("%d → %s\n", n, NumToChinese(n))
	}
}
