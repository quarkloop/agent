package docsvc

import (
	"regexp"
	"strings"
)

var wideSpaceRE = regexp.MustCompile(`\s{2,}`)

func tablesFromPages(pages []textPage) []table {
	var out []table
	for _, page := range pages {
		out = append(out, markdownTables(page)...)
		out = append(out, delimitedTables(page)...)
	}
	return out
}

func markdownTables(page textPage) []table {
	lines := strings.Split(page.Text, "\n")
	var out []table
	for i := 0; i < len(lines); i++ {
		if !looksLikeMarkdownTableLine(lines[i]) {
			continue
		}
		start := i
		var group []string
		for i < len(lines) && looksLikeMarkdownTableLine(lines[i]) {
			group = append(group, lines[i])
			i++
		}
		i--
		if len(group) < 2 {
			continue
		}
		headers := splitMarkdownCells(group[0])
		rowStart := 1
		if isMarkdownSeparator(group[1]) {
			rowStart = 2
		}
		rows := make([]tableRow, 0, len(group)-rowStart)
		for _, line := range group[rowStart:] {
			cells := splitMarkdownCells(line)
			if len(cells) > 0 {
				rows = append(rows, tableRow{Cells: cells})
			}
		}
		if len(headers) > 0 && len(rows) > 0 {
			out = append(out, table{
				PageNumber: page.PageNumber,
				Headers:    headers,
				Rows:       rows,
				Box:        box{X: 36, Y: 36 + float32(start*16), Width: 540, Height: float32(len(group) * 16)},
			})
		}
	}
	return out
}

func delimitedTables(page textPage) []table {
	lines := strings.Split(page.Text, "\n")
	var out []table
	for i := 0; i < len(lines); i++ {
		fields := splitDelimitedLine(lines[i])
		if len(fields) < 2 {
			continue
		}
		start := i
		group := [][]string{fields}
		for i+1 < len(lines) {
			next := splitDelimitedLine(lines[i+1])
			if len(next) != len(fields) {
				break
			}
			group = append(group, next)
			i++
		}
		if len(group) < 2 {
			continue
		}
		rows := make([]tableRow, 0, len(group)-1)
		for _, row := range group[1:] {
			rows = append(rows, tableRow{Cells: row})
		}
		out = append(out, table{
			PageNumber: page.PageNumber,
			Headers:    group[0],
			Rows:       rows,
			Box:        box{X: 36, Y: 36 + float32(start*16), Width: 540, Height: float32(len(group) * 16)},
		})
	}
	return out
}

func looksLikeMarkdownTableLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.Count(trimmed, "|") >= 2
}

func splitMarkdownCells(line string) []string {
	trimmed := strings.Trim(strings.TrimSpace(line), "|")
	raw := strings.Split(trimmed, "|")
	out := make([]string, 0, len(raw))
	for _, cell := range raw {
		cell = strings.TrimSpace(cell)
		if cell != "" {
			out = append(out, cell)
		}
	}
	return out
}

func isMarkdownSeparator(line string) bool {
	for _, cell := range splitMarkdownCells(line) {
		cell = strings.Trim(cell, ":- ")
		if cell != "" {
			return false
		}
	}
	return true
}

func splitDelimitedLine(line string) []string {
	trimmed := strings.TrimSpace(line)
	if strings.Contains(trimmed, "\t") {
		return cleanCells(strings.Split(trimmed, "\t"))
	}
	if wideSpaceRE.MatchString(trimmed) {
		return cleanCells(wideSpaceRE.Split(trimmed, -1))
	}
	return nil
}

func cleanCells(cells []string) []string {
	out := make([]string, 0, len(cells))
	for _, cell := range cells {
		cell = strings.TrimSpace(cell)
		if cell != "" {
			out = append(out, cell)
		}
	}
	return out
}
