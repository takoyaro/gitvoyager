package tui

import (
	"fmt"
	"strconv"
	"strings"
)

// GradientText renders a string with a linear RGB gradient between two hex colors.
// Each rune gets an interpolated color via ANSI true-color escape codes.
func GradientText(s, fromHex, toHex string) string {
	runes := []rune(s)
	n := len(runes)
	if n == 0 {
		return s
	}

	fr, fg, fb := hexToRGB(fromHex)
	tr, tg, tb := hexToRGB(toHex)

	var b strings.Builder
	b.Grow(n * 20) // rough estimate for ANSI overhead

	for i, ch := range runes {
		if ch == ' ' || ch == '\n' {
			b.WriteRune(ch)
			continue
		}
		var t float64
		if n > 1 {
			t = float64(i) / float64(n-1)
		}
		r := int(fr + t*(tr-fr))
		g := int(fg + t*(tg-fg))
		bl := int(fb + t*(tb-fb))
		fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm%c", r, g, bl, ch)
	}
	b.WriteString("\x1b[0m")
	return b.String()
}

func hexToRGB(hex string) (r, g, b float64) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return 128, 128, 128
	}
	ri, _ := strconv.ParseInt(hex[0:2], 16, 32)
	gi, _ := strconv.ParseInt(hex[2:4], 16, 32)
	bi, _ := strconv.ParseInt(hex[4:6], 16, 32)
	return float64(ri), float64(gi), float64(bi)
}
