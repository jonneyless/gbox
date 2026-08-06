package gbox

import (
	"cmp"
	"slices"
)

func Union[T comparable](slice1, slice2 []T) []T {
	m := make(map[T]bool)
	result := make([]T, 0, len(slice1)+len(slice2))

	for _, v := range slice1 {
		if !m[v] {
			m[v] = true
			result = append(result, v)
		}
	}

	for _, v := range slice2 {
		if !m[v] {
			m[v] = true
			result = append(result, v)
		}
	}

	return result
}

func UnionAll[T comparable](slice1, slice2 []T) []T {
	result := make([]T, 0, len(slice1)+len(slice2))
	result = append(result, slice1...)
	result = append(result, slice2...)
	return result
}

func Intersect[T comparable](slice1, slice2 []T) []T {
	m := make(map[T]bool)
	result := make([]T, 0)

	for _, v := range slice1 {
		m[v] = true
	}

	for _, v := range slice2 {
		if m[v] {
			result = append(result, v)
			m[v] = false
		}
	}

	return result
}

func Difference[T comparable](slice1, slice2 []T) []T {
	m := make(map[T]bool)
	result := make([]T, 0)

	// 记录第二个切片的所有元素
	for _, v := range slice2 {
		m[v] = true
	}

	// 检查第一个切片的元素是否在第二个切片中
	for _, v := range slice1 {
		if !m[v] {
			result = append(result, v)
		}
	}

	return result
}

func SymmetricDifference[T comparable](slice1, slice2 []T) []T {
	m := make(map[T]int)
	result := make([]T, 0)

	// 统计每个元素出现的次数
	for _, v := range slice1 {
		m[v]++
	}
	for _, v := range slice2 {
		m[v]++
	}

	// 只出现一次的元素
	for k, v := range m {
		if v == 1 {
			result = append(result, k)
		}
	}

	return result
}

func InArray(item any, array []any) bool {
	return slices.Contains(array, item)
}

func Chunk[T any](items []T, chunkSize int) [][]T {
	if chunkSize <= 0 {
		return [][]T{}
	}

	if len(items) == 0 {
		return [][]T{}
	}

	chunks := make([][]T, 0, (len(items)+chunkSize-1)/chunkSize)

	for i := 0; i < len(items); i += chunkSize {
		end := min(i+chunkSize, len(items))
		chunks = append(chunks, items[i:end])
	}

	return chunks
}

func SameArray[T cmp.Ordered](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}

	aCopy := slices.Clone(a)
	bCopy := slices.Clone(b)

	slices.Sort(aCopy)
	slices.Sort(bCopy)

	return slices.Equal(aCopy, bCopy)
}
