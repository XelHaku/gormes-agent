package hermes

import (
	"fmt"
	"unicode/utf8"
)

func compressionContentLength(content any) int {
	switch v := content.(type) {
	case nil:
		return 0
	case string:
		return utf8.RuneCountInString(v)
	case map[string]any:
		return compressionContentBlockLength(v)
	case []any:
		total := 0
		for _, item := range v {
			total += compressionContentListItemLength(item)
		}
		return total
	default:
		return utf8.RuneCountInString(fmt.Sprint(v))
	}
}

func compressionContentListItemLength(item any) int {
	switch v := item.(type) {
	case nil:
		return 0
	case string:
		return utf8.RuneCountInString(v)
	case map[string]any:
		return compressionContentBlockLength(v)
	default:
		return utf8.RuneCountInString(fmt.Sprint(v))
	}
}

func compressionContentBlockLength(block map[string]any) int {
	text, ok := block["text"].(string)
	if !ok {
		return 0
	}
	return utf8.RuneCountInString(text)
}
