package raster

import "image"

// The block elements drawBlockElement fills, by the first and last rune of each run.
const (
	blockUpperHalf         = 0x2580 // ▀
	blockLowerEighthsFirst = 0x2581 // ▁, one eighth, up to █ at eight
	blockFull              = 0x2588 // █
	blockLeftEighthsLast   = 0x258F // ▏, one eighth, down from ▉ at seven
	blockRightHalf         = 0x2590 // ▐
	blockShadeFirst        = 0x2591 // ░, then ▒ and ▓
	blockShadeLast         = 0x2593 // ▓
	blockUpperEighth       = 0x2594 // ▔
	blockRightEighth       = 0x2595 // ▕
	blockQuadrantFirst     = 0x2596 // ▖, the first of the quadrants up to ▟
)

// eighthsPerCell is how many eighths a cell's width or height is split into; a half is
// halfEighths of them.
const (
	eighthsPerCell = 8
	halfEighths    = eighthsPerCell / 2
)

// shadeStep is the coverage one shade level adds: ░ ▒ ▓ cover a quarter, a half and three
// quarters of the cell's ink.
const shadeStep = 0x40

// Quadrant bits: upper-left, upper-right, lower-left, lower-right.
const (
	quadrantUpperLeft = 1 << iota
	quadrantUpperRight
	quadrantLowerLeft
	quadrantLowerRight
)

// blockQuadrants maps U+2596–259F to the quadrants each fills.
var blockQuadrants = [...]uint8{
	quadrantLowerLeft,  // ▖
	quadrantLowerRight, // ▗
	quadrantUpperLeft,  // ▘
	quadrantUpperLeft | quadrantLowerLeft | quadrantLowerRight,  // ▙
	quadrantUpperLeft | quadrantLowerRight,                      // ▚
	quadrantUpperLeft | quadrantUpperRight | quadrantLowerLeft,  // ▛
	quadrantUpperLeft | quadrantUpperRight | quadrantLowerRight, // ▜
	quadrantUpperRight,                                          // ▝
	quadrantUpperRight | quadrantLowerLeft,                      // ▞
	quadrantUpperRight | quadrantLowerLeft | quadrantLowerRight, // ▟
}

// drawBlockElement fills a block element (U+2580–259F) into mask. The eighths a partial block or
// bar fills are rounded to whole pixels, and the halves of a cell are complementary: ▀ and ▄, ▌
// and ▐ tile it without a gap or an overlap.
func drawBlockElement(mask *image.Alpha, ru rune) {
	width, height := mask.Rect.Dx(), mask.Rect.Dy()
	columns := func(eighths int) int { return (width*eighths + halfEighths) / eighthsPerCell }
	rows := func(eighths int) int { return (height*eighths + halfEighths) / eighthsPerCell }
	middleX, middleY := columns(halfEighths), height-rows(halfEighths)
	switch {
	case ru == blockUpperHalf:
		fillMask(mask, image.Rect(0, 0, width, middleY), fullCoverage)
	case ru >= blockLowerEighthsFirst && ru <= blockFull:
		fillMask(mask, image.Rect(0, height-rows(int(ru-blockUpperHalf)), width, height), fullCoverage)
	case ru > blockFull && ru <= blockLeftEighthsLast:
		fillMask(mask, image.Rect(0, 0, columns(eighthsPerCell-int(ru-blockFull)), height), fullCoverage)
	case ru == blockRightHalf:
		fillMask(mask, image.Rect(middleX, 0, width, height), fullCoverage)
	case ru >= blockShadeFirst && ru <= blockShadeLast:
		fillMask(mask, mask.Rect, uint8(shadeStep*int(ru-blockRightHalf)))
	case ru == blockUpperEighth:
		fillMask(mask, image.Rect(0, 0, width, rows(1)), fullCoverage)
	case ru == blockRightEighth:
		fillMask(mask, image.Rect(width-columns(1), 0, width, height), fullCoverage)
	case ru >= blockQuadrantFirst && ru <= blockElementsLast:
		drawQuadrants(mask, blockQuadrants[ru-blockQuadrantFirst], middleX, middleY)
	}
}

// drawQuadrants fills the quadrants set in bits, the cell split at (middleX, middleY).
func drawQuadrants(mask *image.Alpha, bits uint8, middleX, middleY int) {
	width, height := mask.Rect.Dx(), mask.Rect.Dy()
	for bit, quadrant := range [...]image.Rectangle{
		image.Rect(0, 0, middleX, middleY), image.Rect(middleX, 0, width, middleY),
		image.Rect(0, middleY, middleX, height), image.Rect(middleX, middleY, width, height),
	} {
		if bits&(1<<bit) != 0 {
			fillMask(mask, quadrant, fullCoverage)
		}
	}
}
