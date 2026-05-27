package markdown

import (
	"strings"
	"unicode"
)

func PlainText(value string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
}

func InlineCode(value string) string {
	text := PlainText(value)
	longest := 0
	current := 0
	for _, r := range text {
		if r == '`' {
			current++
			if current > longest {
				longest = current
			}
		} else {
			current = 0
		}
	}
	fence := strings.Repeat("`", longest+1)
	if strings.HasPrefix(text, "`") || strings.HasSuffix(text, "`") {
		text = " " + text + " "
	}
	return fence + text + fence
}

func FencedCodeBlock(language, content string) string {
	longest := 0
	current := 0
	for _, r := range content {
		if r == '`' {
			current++
			if current > longest {
				longest = current
			}
		} else {
			current = 0
		}
	}
	size := 3
	if longest >= size {
		size = longest + 1
	}
	fence := strings.Repeat("`", size)
	return fence + language + "\n" + content + "\n" + fence
}
