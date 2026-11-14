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

// splitTableRow splits a table row into trimmed cells. It stops after limit
// cells, if limit is positive.
func splitTableRow(line []byte, cells [][]byte, limit int) [][]byte {
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
			if len(cells) == limit {
				return cells
			}
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
		if d.lazy || d.indent >= 4 {
			continue
		}
		line := p.src[d.start:d.end]
		if len(line) == 0 || (line[0] != '|' && line[0] != '-' && line[0] != ':') {
			continue
		}
		aligns, ok := parseDelimiterRow(line, nil)
		if !ok {
			continue
		}
		h := segs[i-1]
		header := splitTableRow(p.src[h.start:h.end], p.inl.cells[:0], len(aligns)+1)
		p.inl.cells = header[:0]
		if len(header) != len(aligns) {
			// The header row must have as many cells as the delimiter row.
			continue
		}

		t := p.arena.new(kindTable)
		p.arena.newExt(t).aligns = aligns
		t.noBlankBefore = i == 1
		t.startLine = para.startLine
		t.lastLine = para.lastLine
		p.addTableRow(t, p.src[h.start:h.end], true)
		for _, r := range segs[i+1:] {
			p.addTableRow(t, p.src[r.start:r.end], false)
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

// isDelimiterRow reports whether line, the next line of a paragraph whose
// last line is header, is a table delimiter row with as many cells as header.
// The paragraph then holds a table, see extractTable, which stops lazy
// continuation lines.
func (p *parser) isDelimiterRow(header seg, line []byte) bool {
	if p.indent >= 4 || len(line) == 0 || line[0] != '|' && line[0] != '-' && line[0] != ':' {
		return false
	}
	aligns, ok := parseDelimiterRow(line, p.inl.aligns[:0])
	p.inl.aligns = aligns[:0]
	if !ok {
		return false
	}
	cells := splitTableRow(p.src[header.start:header.end], p.inl.cells[:0], len(aligns)+1)
	p.inl.cells = cells[:0]
	return len(cells) == len(aligns)
}

// emptyTableRow reports whether line has no table cells, e.g., "|".
func (p *parser) emptyTableRow(line []byte) bool {
	cells := splitTableRow(line, p.inl.cells[:0], 1)
	p.inl.cells = cells[:0]
	return len(cells) == 0
}

// addTableRow adds a row to the table t. Its cells are only split when it is
// rendered, see rowCells, so that a table costs one node per row.
func (p *parser) addTableRow(t *node, line []byte, header bool) {
	row := p.arena.new(kindTableRow)
	row.header = header
	row.text = line
	t.appendChild(row)
}

// rowCells adds the cells of the row of the table t as its children, with
// their alignments. Cells beyond the delimiter row are dropped, and missing
// cells are added when rendering, see renderer.table.
func (p *parser) rowCells(t, row *node) {
	cells := splitTableRow(row.text, p.inl.rowCells[:0], len(t.ext.aligns))
	for i, text := range cells {
		c := p.arena.new(kindTableCell)
		c.header = row.header
		c.align = t.ext.aligns[i]
		c.text = text
		row.appendChild(c)
	}
	clear(cells)
	p.inl.rowCells = cells[:0]
}
