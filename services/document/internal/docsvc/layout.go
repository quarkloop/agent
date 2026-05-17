package docsvc

import "strings"

func layoutsFromPages(pages []textPage) []layoutPage {
	out := make([]layoutPage, 0, len(pages))
	for _, page := range pages {
		lines := nonEmptyLines(page.Text)
		blocks := make([]layoutBlock, 0, len(lines))
		for i, line := range lines {
			width := float32(len([]rune(line))) * 7
			if width < 24 {
				width = 24
			}
			if width > 560 {
				width = 560
			}
			blocks = append(blocks, layoutBlock{
				Kind: "text",
				Text: line,
				Box: box{
					X:      36,
					Y:      36 + float32(i*16),
					Width:  width,
					Height: 14,
				},
			})
		}
		height := float32(72 + len(lines)*16)
		if height < 792 {
			height = 792
		}
		out = append(out, layoutPage{
			PageNumber: page.PageNumber,
			Width:      612,
			Height:     height,
			Blocks:     blocks,
		})
	}
	return out
}

func nonEmptyLines(text string) []string {
	raw := strings.Split(text, "\n")
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func imagesForSource(source sourceDocument, detected detection) []image {
	if detected.Family != "image" {
		return nil
	}
	ref := "sha256:" + sourceHash(source.Content)
	return []image{{
		PageNumber: 1,
		ImageRef:   ref,
		MIMEType:   detected.MIMEType,
		Box:        box{X: 0, Y: 0, Width: 1, Height: 1},
		Metadata:   cloneMap(detected.Metadata),
	}}
}
