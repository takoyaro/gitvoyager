package readme

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"

	"github.com/yuin/goldmark/extension"
)

// Signals holds structural metrics extracted from a README's markdown AST.
type Signals struct {
	ByteSize               int
	WordCount              int
	HeadingCount           int
	CodeBlockCount         int
	TableCount             int
	ImageCount             int
	LinkCount              int
	ListCount              int
	BadgeCount             int
	HasInstallSection      bool
	HasUsageSection        bool
	HasComparisonSection   bool
	HasArchitectureSection bool
	HasDemoAssets          bool
}

// Section heading keyword sets for detection.
var (
	installKeywords      = []string{"install", "setup", "getting started", "quick start", "quickstart"}
	usageKeywords        = []string{"usage", "example", "examples", "quick start", "quickstart", "how to use", "tutorial"}
	comparisonKeywords   = []string{"alternative", "alternatives", "comparison", "vs", "vs.", "compared", "unlike", "why this", "why not"}
	architectureKeywords = []string{"architecture", "design", "how it works", "overview", "internals", "under the hood"}
)

// Analyze parses README markdown content and returns structural signals.
func Analyze(content string) Signals {
	src := []byte(content)
	md := goldmark.New(goldmark.WithExtensions(extension.GFM))
	doc := md.Parser().Parse(text.NewReader(src))

	s := Signals{ByteSize: len(src)}

	// Track whether we've seen the first H2+ heading (for badge detection).
	seenFirstSection := false

	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch n.Kind() {
		case ast.KindHeading:
			h := n.(*ast.Heading)
			s.HeadingCount++

			if h.Level >= 2 {
				seenFirstSection = true
			}

			// Extract heading text and check for section keywords.
			headingText := strings.ToLower(extractText(n, src))
			checkSectionKeywords(&s, headingText)

		case ast.KindFencedCodeBlock, ast.KindCodeBlock:
			s.CodeBlockCount++

		case ast.KindList:
			s.ListCount++

		case ast.KindImage:
			img := n.(*ast.Image)
			dest := string(img.Destination)
			s.ImageCount++

			if !seenFirstSection && isBadgeURL(dest) {
				s.BadgeCount++
			} else if isDemoAsset(dest) {
				s.HasDemoAssets = true
			}

		case ast.KindLink:
			s.LinkCount++

		case ast.KindText:
			t := n.(*ast.Text)
			seg := t.Segment
			s.WordCount += len(strings.Fields(string(seg.Value(src))))
		}

		// Check for GFM tables.
		if n.Kind() == east.KindTable {
			s.TableCount++
		}

		return ast.WalkContinue, nil
	})

	return s
}

// Score computes a quality score (0-10) from README signals.
func Score(s Signals) float64 {
	raw := 0.0

	// Size tiers (max 10)
	switch {
	case s.ByteSize > 10240:
		raw += 10.0
	case s.ByteSize > 5120:
		raw += 7.0
	case s.ByteSize > 2048:
		raw += 5.0
	case s.ByteSize > 500:
		raw += 3.0
	}

	if s.CodeBlockCount > 0 {
		raw += 3.0
	}
	if s.TableCount > 0 {
		raw += 3.0
	}
	if s.BadgeCount > 0 {
		raw += 2.0
	}
	if s.HasInstallSection {
		raw += 5.0
	}
	if s.HasComparisonSection {
		raw += 5.0
	}
	if s.HasArchitectureSection {
		raw += 3.0
	}
	if s.HasUsageSection {
		raw += 2.0
	}
	if s.HasDemoAssets {
		raw += 2.0
	}

	// Normalize: max raw is ~35, scale to 0-10.
	const maxRaw = 35.0
	score := raw / maxRaw * 10.0
	if score > 10.0 {
		score = 10.0
	}
	return score
}

// extractText collects all text content from a node and its children.
func extractText(n ast.Node, src []byte) string {
	var sb strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if c.Kind() == ast.KindText {
			t := c.(*ast.Text)
			sb.Write(t.Segment.Value(src))
		}
		// Recurse into inline nodes (e.g. emphasis inside headings).
		for gc := c.FirstChild(); gc != nil; gc = gc.NextSibling() {
			if gc.Kind() == ast.KindText {
				t := gc.(*ast.Text)
				sb.Write(t.Segment.Value(src))
			}
		}
	}
	return sb.String()
}

func checkSectionKeywords(s *Signals, heading string) {
	for _, kw := range installKeywords {
		if strings.Contains(heading, kw) {
			s.HasInstallSection = true
			return
		}
	}
	for _, kw := range usageKeywords {
		if strings.Contains(heading, kw) {
			s.HasUsageSection = true
			return
		}
	}
	for _, kw := range comparisonKeywords {
		if strings.Contains(heading, kw) {
			s.HasComparisonSection = true
			return
		}
	}
	for _, kw := range architectureKeywords {
		if strings.Contains(heading, kw) {
			s.HasArchitectureSection = true
			return
		}
	}
}

func isBadgeURL(url string) bool {
	lower := strings.ToLower(url)
	return strings.Contains(lower, "img.shields.io") ||
		strings.Contains(lower, "badge") ||
		strings.Contains(lower, "badgen.net") ||
		strings.Contains(lower, "github.com/") && strings.Contains(lower, "/workflows/") ||
		strings.Contains(lower, "codecov.io") ||
		strings.Contains(lower, "coveralls.io") ||
		strings.Contains(lower, "pkg.go.dev/badge") ||
		strings.Contains(lower, "goreportcard.com/badge")
}

func isDemoAsset(url string) bool {
	lower := strings.ToLower(url)
	return strings.HasSuffix(lower, ".gif") ||
		strings.HasSuffix(lower, ".mp4") ||
		strings.HasSuffix(lower, ".webm") ||
		strings.HasSuffix(lower, ".mov") ||
		(strings.HasSuffix(lower, ".png") && !isBadgeURL(lower)) ||
		(strings.HasSuffix(lower, ".jpg") && !isBadgeURL(lower)) ||
		(strings.HasSuffix(lower, ".jpeg") && !isBadgeURL(lower)) ||
		strings.Contains(lower, "demo") ||
		strings.Contains(lower, "screenshot") ||
		strings.Contains(lower, "preview")
}
