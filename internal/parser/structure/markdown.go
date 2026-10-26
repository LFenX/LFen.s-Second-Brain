package structure

import (
	"bytes"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	mdast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

type markdownHeading struct {
	start int
	level int
	name  string
}

func (b *fileBuilder) parseMarkdown() error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	offset := 0
	if hasBOM(b.in.Content) {
		offset = 3
	}
	raw := b.in.Content[offset:]
	// Goldmark builds syntax only; rendering and image fetching are never used.
	document := goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser().Parse(text.NewReader(raw))
	if err := b.ctx.Err(); err != nil {
		return err
	}
	headings := []markdownHeading{}
	err := mdast.Walk(document, func(node mdast.Node, entering bool) (mdast.WalkStatus, error) {
		if !entering {
			return mdast.WalkContinue, nil
		}
		if err := b.ctx.Err(); err != nil {
			return mdast.WalkStop, err
		}
		if node.Kind().String() == "List" || node.Kind().String() == "Table" {
			b.diagnostic("block_navigation_not_materialized", "Table and list navigation is not materialized; original source remains complete.", -1, -1)
		}
		heading, ok := node.(*mdast.Heading)
		if !ok || heading.Lines().Len() == 0 {
			return mdast.WalkContinue, nil
		}
		if heading.Parent() != document {
			b.diagnostic("block_navigation_not_materialized", "Container heading navigation is not materialized; original source remains complete.", -1, -1)
			return mdast.WalkContinue, nil
		}
		first := heading.Lines().At(0)
		last := heading.Lines().At(heading.Lines().Len() - 1)
		firstByte, lastByte := first.Start+offset, last.Stop+offset
		if firstByte < offset || lastByte < firstByte || lastByte > len(b.in.Content) {
			return mdast.WalkStop, b.fail("invalid_markdown_range")
		}
		lineIndex := b.lineAt(firstByte) - 1
		start := b.lineStart[lineIndex]
		lineEnd := len(b.in.Content)
		if lineIndex+1 < len(b.lineStart) {
			lineEnd = b.lineStart[lineIndex+1]
		}
		line := b.in.Content[start:lineEnd]
		if start == 0 && hasBOM(line) {
			line = line[3:]
		}
		indent := 0
		for indent < len(line) && line[indent] == ' ' {
			indent++
		}
		marker := bytes.TrimRight(line[indent:], "\r\n")
		atx := indent <= 3 && len(marker) >= heading.Level && bytes.Equal(marker[:heading.Level], bytes.Repeat([]byte{'#'}, heading.Level)) && (len(marker) == heading.Level || marker[heading.Level] == ' ' || marker[heading.Level] == '\t')
		setext := false
		if !atx {
			lastLine := b.lineAt(maxInt(firstByte, lastByte-1)) - 1
			if lastLine+1 < len(b.lineStart) {
				underlineStart := b.lineStart[lastLine+1]
				underlineEnd := len(b.in.Content)
				if lastLine+2 < len(b.lineStart) {
					underlineEnd = b.lineStart[lastLine+2]
				}
				underline := bytes.Trim(b.in.Content[underlineStart:underlineEnd], " \t\r\n")
				if len(underline) > 0 && ((heading.Level == 1 && allByte(underline, '=')) || (heading.Level == 2 && allByte(underline, '-'))) {
					setext = true
				}
			}
		}
		if !atx && !setext {
			// Quoted/container headings have syntax offsets, but their complete
			// marker/container extent is not proven by this first producer.
			b.diagnostic("block_navigation_not_materialized", "Container block navigation is not materialized; original source remains complete.", start, lineEnd)
			return mdast.WalkContinue, nil
		}
		nameParts := make([]string, 0, heading.Lines().Len())
		for i := 0; i < heading.Lines().Len(); i++ {
			segment := heading.Lines().At(i)
			nameParts = append(nameParts, string(segment.Value(raw)))
		}
		headings = append(headings, markdownHeading{start: start, level: heading.Level, name: strings.TrimSpace(strings.Join(nameParts, " "))})
		return mdast.WalkContinue, nil
	})
	if err != nil {
		return err
	}
	sort.Slice(headings, func(i, j int) bool { return headings[i].start < headings[j].start })
	stack := []struct {
		level int
		key   string
	}{{0, "file"}}
	for i, heading := range headings {
		if err := b.ctx.Err(); err != nil {
			return err
		}
		for len(stack) > 1 && stack[len(stack)-1].level >= heading.level {
			stack = stack[:len(stack)-1]
		}
		end := len(b.in.Content)
		for j := i + 1; j < len(headings); j++ {
			if headings[j].level <= heading.level {
				end = headings[j].start
				break
			}
		}
		key := b.addNode(stack[len(stack)-1].key, "section", "section", heading.name, heading.start, end)
		if b.err != nil {
			return b.err
		}
		stack = append(stack, struct {
			level int
			key   string
		}{heading.level, key})
		b.ends = append(b.ends, end)
	}
	return nil
}

func allByte(v []byte, ch byte) bool {
	for _, b := range v {
		if b != ch {
			return false
		}
	}
	return true
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
