package main

func rotateRunes(s string, shift int) string {
	runes := []rune(s)
	n := len(runes)
	if n == 0 {
		return ""
	}

	k := shift % n
	if k < 0 {
		k += n
	}

	result := []rune{}
	result = append(result, runes[k:]...)
	result = append(result, runes[:k]...)
	return string(result)
}
