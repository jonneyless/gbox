package gbox

import "fmt"

func GetPagination(prefix string, page, pageSize, pageMax int) []map[string]any {
	key := fmt.Sprintf("%s:%%d:%%d:%%d", prefix)

	pageButtons := make([]map[string]any, 0)

	if pageMax == 1 {
		return pageButtons
	}

	// 总页数不超过8页，显示所有页码
	if pageMax <= 8 {
		for i := 1; i <= pageMax; i++ {
			text := fmt.Sprintf("%d", i)
			if i == page {
				text = fmt.Sprintf("✅%d", i)
			}
			pageButtons = append(pageButtons, map[string]any{
				"text":          text,
				"callback_data": fmt.Sprintf(key, i, pageSize, pageMax),
			})
		}
		return pageButtons
	}

	// 总页数大于8页
	// 前3页：1,2,3,4,5,...,尾页
	if page <= 3 {
		var i int
		for i = 1; i <= 5; i++ {
			text := fmt.Sprintf("%d", i)
			if i == page {
				text = fmt.Sprintf("✅%d", i)
			}
			pageButtons = append(pageButtons, map[string]any{
				"text":          text,
				"callback_data": fmt.Sprintf(key, i, pageSize, pageMax),
			})
		}
		pageButtons = append(pageButtons, map[string]any{
			"text":          "...",
			"callback_data": fmt.Sprintf(key, i+1, pageSize, pageMax),
		})
		pageButtons = append(pageButtons, map[string]any{
			"text":          "尾页",
			"callback_data": fmt.Sprintf(key, pageMax, pageSize, pageMax),
		})
		return pageButtons
	}

	// 第4页：首页,2,3,4,5,6,...,尾页
	if page == 4 {
		pageButtons = append(pageButtons, map[string]any{
			"text":          "首页",
			"callback_data": fmt.Sprintf(key, 1, pageSize, pageMax),
		})
		var i int
		for i = 2; i <= 6; i++ {
			text := fmt.Sprintf("%d", i)
			if i == page {
				text = fmt.Sprintf("✅%d", i)
			}
			pageButtons = append(pageButtons, map[string]any{
				"text":          text,
				"callback_data": fmt.Sprintf(key, i, pageSize, pageMax),
			})
		}
		pageButtons = append(pageButtons, map[string]any{
			"text":          "...",
			"callback_data": fmt.Sprintf(key, i+1, pageSize, pageMax),
		})
		pageButtons = append(pageButtons, map[string]any{
			"text":          "尾页",
			"callback_data": fmt.Sprintf(key, pageMax, pageSize, pageMax),
		})
		return pageButtons
	}

	// 第pageMax-3页：首页,...,pageMax-4,pageMax-3,pageMax-2,pageMax-1,尾页
	if page == pageMax-3 {
		pageButtons = append(pageButtons, map[string]any{
			"text":          "首页",
			"callback_data": fmt.Sprintf(key, 1, pageSize, pageMax),
		})
		pageButtons = append(pageButtons, map[string]any{
			"text":          "...",
			"callback_data": fmt.Sprintf(key, pageMax-5, pageSize, pageMax),
		})
		for i := pageMax - 4; i <= pageMax-1; i++ {
			text := fmt.Sprintf("%d", i)
			if i == page {
				text = fmt.Sprintf("✅%d", i)
			}
			pageButtons = append(pageButtons, map[string]any{
				"text":          text,
				"callback_data": fmt.Sprintf(key, i, pageSize, pageMax),
			})
		}
		pageButtons = append(pageButtons, map[string]any{
			"text":          "尾页",
			"callback_data": fmt.Sprintf(key, pageMax, pageSize, pageMax),
		})
		return pageButtons
	}

	// 后3页（pageMax-2, pageMax-1, pageMax）：首页,...,pageMax-3,pageMax-2,pageMax-1,pageMax
	if page >= pageMax-2 {
		pageButtons = append(pageButtons, map[string]any{
			"text":          "首页",
			"callback_data": fmt.Sprintf(key, 1, pageSize, pageMax),
		})
		pageButtons = append(pageButtons, map[string]any{
			"text":          "...",
			"callback_data": fmt.Sprintf(key, pageMax-4, pageSize, pageMax),
		})
		for i := pageMax - 3; i <= pageMax; i++ {
			text := fmt.Sprintf("%d", i)
			if i == page {
				text = fmt.Sprintf("✅%d", i)
			}
			pageButtons = append(pageButtons, map[string]any{
				"text":          text,
				"callback_data": fmt.Sprintf(key, i, pageSize, pageMax),
			})
		}
		return pageButtons
	}

	// 中间页（5到pageMax-4）：首页,...,page-1,page,page+1,...,尾页
	pageButtons = append(pageButtons, map[string]any{
		"text":          "首页",
		"callback_data": fmt.Sprintf(key, 1, pageSize, pageMax),
	})
	pageButtons = append(pageButtons, map[string]any{
		"text":          "...",
		"callback_data": fmt.Sprintf(key, page-2, pageSize, pageMax),
	})
	var i int
	for i = page - 1; i <= page+1; i++ {
		text := fmt.Sprintf("%d", i)
		if i == page {
			text = fmt.Sprintf("✅%d", i)
		}
		pageButtons = append(pageButtons, map[string]any{
			"text":          text,
			"callback_data": fmt.Sprintf(key, i, pageSize, pageMax),
		})
	}
	pageButtons = append(pageButtons, map[string]any{
		"text":          "...",
		"callback_data": fmt.Sprintf(key, i+1, pageSize, pageMax),
	})
	pageButtons = append(pageButtons, map[string]any{
		"text":          "尾页",
		"callback_data": fmt.Sprintf(key, pageMax, pageSize, pageMax),
	})

	return pageButtons
}
