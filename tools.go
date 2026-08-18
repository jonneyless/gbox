package gbox

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"unicode"
	"unsafe"

	"github.com/forPelevin/gomoji"
	"github.com/spf13/cast"
)

func Md5Unsafe(s string) string {
	h := md5.New()
	h.Write(unsafe.Slice(unsafe.StringData(s), len(s)))
	return hex.EncodeToString(h.Sum(nil))
}

func EmojiCount(s string) int {
	if s == "" {
		return 0
	}

	// 获取字符串中的所有emoji
	emojis := gomoji.CollectAll(s)
	emojiCount := 0

	// 计算emoji字符的总数
	for _, emoji := range emojis {
		emojiCount += len([]rune(emoji.Character))
	}

	return emojiCount
}

func IsPureEmoji(s string) bool {
	if s == "" {
		return false
	}

	emojiCount := EmojiCount(s)

	// 计算字符串中非空格字符的总数
	totalNonSpace := 0
	for _, r := range s {
		if !unicode.IsSpace(r) {
			totalNonSpace++
		}
	}

	// 如果所有非空格字符都是emoji，则认为是纯emoji
	return emojiCount == totalNonSpace && emojiCount > 0
}

func IsEmoji(r rune) bool {
	return (r >= 0x1F600 && r <= 0x1F64F) ||
		(r >= 0x1F300 && r <= 0x1F5FF) ||
		(r >= 0x1F680 && r <= 0x1F6FF) ||
		(r >= 0x1F900 && r <= 0x1F9FF) ||
		(r >= 0x2600 && r <= 0x26FF) ||
		(r >= 0x2700 && r <= 0x27BF)
}

func IsOtherLanguageChar(r rune) bool {
	// 英文或数字
	if unicode.IsLetter(r) && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
		return false
	}
	// 中文（基本块）
	if r >= 0x4e00 && r <= 0x9fff {
		return false
	}
	// Emoji
	if IsEmoji(r) {
		return false
	}
	// 其他语言（如日文平假名、韩文、阿拉伯文等）
	return true
}

// ContainsOtherLanguage 字符串是否包含其他语言字符
func ContainsOtherLanguage(s string) bool {
	for _, r := range s {
		if IsOtherLanguageChar(r) {
			return true
		}
	}
	return false
}

func FormatTimeByHour(duration int) string {
	timeStr := fmt.Sprintf("%d小时", duration)

	if duration > 24 {
		days := duration / 24
		hours := duration % 24
		timeStr = fmt.Sprintf("%d天", days)
		if hours > 0 {
			timeStr += fmt.Sprintf("%d小时", hours)
		}
	}

	return timeStr
}

func FormatNumberByChinese(amount int64) string {
	if amount >= 100000000 {
		value := amount / 100000000
		remainder := amount % 100000000
		if remainder == 0 {
			return fmt.Sprintf("%d亿", value)
		}

		return cast.ToString(amount)
	}

	if amount >= 10000 {
		value := amount / 10000
		remainder := amount % 10000
		if remainder == 0 {
			return fmt.Sprintf("%d万", value)
		}

		return cast.ToString(amount)
	}

	return cast.ToString(amount)
}

// ButtonSlice 按钮切分，可以被3整除就按三个一组切分，否则按两个一组切分
func ButtonSlice(data []map[string]any) [][]map[string]any {
	if len(data) == 0 {
		return [][]map[string]any{}
	}

	groupSize := 2
	if len(data)%3 == 0 {
		groupSize = 3
	}

	var result [][]map[string]any
	for i := 0; i < len(data); i += groupSize {
		end := min(i+groupSize, len(data))

		group := make([]map[string]any, end-i)
		copy(group, data[i:end])
		result = append(result, group)
	}
	return result
}

func DetectContentType(file multipart.File) string {
	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return "application/octet-stream"
	}
	return http.DetectContentType(buffer[:n])
}

func MultipartFileToBytesReader(file multipart.File) (*bytes.Reader, error) {
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(data), nil
}

func IsValidChar(r rune) bool {
	if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
		return true
	}

	punctuation := `，。！？；：、‘’“”（）【】《》—…,.!?;:()[]{}<>/'"@#$%^&*+=_~`
	for _, p := range punctuation {
		if r == p {
			return true
		}
	}

	if (r >= 0x1F600 && r <= 0x1F64F) || // 表情
		(r >= 0x1F300 && r <= 0x1F5FF) || // 符号和象形文字
		(r >= 0x1F680 && r <= 0x1F6FF) || // 交通和地图
		(r >= 0x1F700 && r <= 0x1F77F) || // 炼金术符号
		(r >= 0x1F900 && r <= 0x1F9FF) || // 补充符号
		(r >= 0x2600 && r <= 0x27BF) || // 杂项符号
		(r >= 0xFE00 && r <= 0xFE0F) || // 变体选择器
		(r >= 0x1F1E6 && r <= 0x1F1FF) { // 国旗（地区指示符）
		return true
	}

	return false
}

func ValidateAdsContent(s string) bool {
	if strings.Count(s, "\n") > 8 {
		return false
	}

	// 2. 检查是否包含不允许的字符
	for _, r := range s {
		if !IsValidChar(r) {
			return false
		}
	}

	return true
}

func SafeString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
