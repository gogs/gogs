package dariomark

import (
	"bytes"
)

// parseDelimiterRow parses a table delimiter row, e.g., "| :-- | --: |".
func parseDelimiterRow(line []byte, aligns []alignment) ([]alignment, bool) {
	line = bytes.Trim(line, " \t")
	line = bytes.TrimPrefix(line, []byte("|"))
	line = bytes.TrimSuffix(line, []byte("|"))
	if len(line) == 0 {
		return nil, false
	}
	for len(line) > 0 {
		cell := line
		if i := bytes.IndexByte(line, '|'); i >= 0 {
			cell, line = line[:i], line[i+1:]
			if len(line) == 0 {
				// An empty cell after the last separator.
				return nil, false
			}
		} else {
			line = nil
		}
		cell = bytes.Trim(cell, " \t")
		if len(cell) == 0 {
			return nil, false
		}
		left := cell[0] == ':'
		right := cell[len(cell)-1] == ':'
		dashes := cell
		if left {
			dashes = dashes[1:]
		}
		if right && len(dashes) > 0 {
			dashes = dashes[:len(dashes)-1]
		}
		if len(dashes) == 0 {
			return nil, false
		}
		for _, c := range dashes {
			if c != '-' {
				return nil, false
			}
		}
		switch {
		case left && right:
			aligns = append(aligns, alignCenter)
		case left:
			aligns = append(aligns, alignLeft)
		case right:
			aligns = append(aligns, alignRight)
		default:
			aligns = append(aligns, alignNone)
		}
	}
	return aligns, true
}

// splitTableRow splits a table row into trimmed cells.
func splitTableRow(line []byte, cells [][]byte) [][]byte {
	line = bytes.Trim(line, " \t")
	line = bytes.TrimPrefix(line, []byte("|"))
	if isBlankBytes(line) {
		// A row of a pipe and whitespace has no cells.
		return cells
	}
	line = bytes.TrimSuffix(line, []byte("|"))
	start := 0
	for i := 0; i < len(line); i++ {
		if line[i] == '|' && (i == 0 || line[i-1] != '\\') {
			cells = append(cells, bytes.Trim(line[start:i], " \t"))
			start = i + 1
		}
	}
	if start == len(line) && start > 0 {
		// Another pipe at the end does not start a cell, e.g., "a||".
		return cells
	}
	return append(cells, bytes.Trim(line[start:], " \t"))
}

// extractTable looks for a table in the lines of a paragraph. When found,
// the table is inserted after para, or before it if before is true, and the
// lines preceding the table are returned.
func (p *parser) extractTable(para *node, segs []seg, before bool) []seg {
	for i := 1; i < len(segs); i++ {
		d := segs[i]
		if (d.indent >= 4 || d.tab) && !before {
			// Before a setext heading underline, an indented delimiter row is
			// allowed.
			continue
		}
		line := p.src[d.start:d.end]
		if len(line) == 0 || (line[0] != '|' && line[0] != '-' && line[0] != ':') {
			continue
		}
		if d.eol == d.end && d.indent == 0 && len(bytes.Trim(line, "-")) == 0 ||
			before && i == len(segs)-1 && len(bytes.Trim(line, "- \t")) == 0 {
			// A row of only dashes is not a delimiter row at the end of the
			// input without a line ending, which a lazy continuation line
			// can contain, or directly before a setext heading underline.
			continue
		}
		aligns, ok := parseDelimiterRow(line, nil)
		if !ok {
			continue
		}
		h := segs[i-1]
		header := splitTableRow(p.src[h.start:h.end], p.inl.cells[:0])
		p.inl.cells = header[:0]
		if len(header) > len(aligns) {
			// Only the first delimiter row is considered.
			return segs
		}

		t := p.arena.new(kindTable)
		t.aligns = aligns
		t.noBlankBefore = i == 1
		t.startLine = para.startLine
		t.lastLine = para.lastLine
		p.addTableRow(t, header, true)
		for _, r := range segs[i+1:] {
			p.addTableRow(t, splitTableRow(p.src[r.start:r.end], p.inl.cells[:0]), false)
		}
		if before {
			para.insertBefore(t)
		} else {
			para.insertAfter(t)
		}
		return segs[:i-1]
	}
	return segs
}

func (p *parser) addTableRow(t *node, cells [][]byte, header bool) {
	row := p.arena.new(kindTableRow)
	row.header = header
	t.appendChild(row)
	// Cells beyond the delimiter row are dropped. Missing cells are added
	// when rendering, see renderer.table.
	for i, a := range t.aligns[:min(len(cells), len(t.aligns))] {
		c := p.arena.new(kindTableCell)
		c.header = header
		c.align = a
		c.text = cells[i]
		row.appendChild(c)
	}
	p.inl.cells = cells[:0]
}
