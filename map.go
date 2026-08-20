package gbox

import "sort"

func InsertToMap[T any](m map[int]T, pos int, val T) map[int]T {
	if pos < 1 {
		pos = 1
	}
	if _, exists := m[pos]; exists {
		keys := make([]int, 0, len(m)-pos+1)
		for k := range m {
			if k >= pos {
				keys = append(keys, k)
			}
		}
		sort.Sort(sort.Reverse(sort.IntSlice(keys)))
		for _, k := range keys {
			m[k+1] = m[k]
			delete(m, k)
		}
	}
	m[pos] = val
	return m
}

func InsertToMapBefore[T any](m map[int]T, beforeKey int, val T) map[int]T {
	if beforeKey < 1 {
		return InsertToMap(m, 1, val)
	}
	return InsertToMap(m, beforeKey, val)
}

// InsertToMapAfter 在指定 key 后面插入
func InsertToMapAfter[T any](m map[int]T, afterKey int, val T) map[int]T {
	if afterKey < 1 {
		return InsertToMap(m, 1, val)
	}
	return InsertToMap(m, afterKey+1, val)
}
