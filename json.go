package gbox

import (
	"github.com/bytedance/sonic"
)

func PrettyJSON(compactJSON string) (string, error) {
	var data any
	if err := sonic.Unmarshal([]byte(compactJSON), &data); err != nil {
		return "", err
	}

	prettyBytes, err := sonic.MarshalIndent(&data, "", "  ")
	if err != nil {
		return "", err
	}
	return string(prettyBytes), nil
}

func CompactJSON(prettyJSON string) (string, error) {
	var data any
	if err := sonic.Unmarshal([]byte(prettyJSON), &data); err != nil {
		return "", err
	}

	// 直接使用 Marshal 会输出紧凑 JSON（默认行为）
	compactBytes, err := sonic.Marshal(&data)
	if err != nil {
		return "", err
	}
	return string(compactBytes), nil
}

func ReformatJSON(jsonStr string, pretty bool) (string, error) {
	var data any
	if err := sonic.Unmarshal([]byte(jsonStr), &data); err != nil {
		return "", err
	}

	if pretty {
		prettyBytes, err := sonic.MarshalIndent(&data, "", "  ")
		if err != nil {
			return "", err
		}
		return string(prettyBytes), nil
	}

	compactBytes, err := sonic.Marshal(&data)
	if err != nil {
		return "", err
	}
	return string(compactBytes), nil
}
