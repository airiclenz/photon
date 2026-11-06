package render

import (
	"cmp"
	"image"
	"math"
	"slices"
)

// channelCount is the number of channels a colour is cut along: red, green, blue and alpha.
const channelCount = 4

// colourCount is one distinct colour of an image and the number of pixels holding it.
type colourCount struct {
	channels [channelCount]uint8
	pixels   int
}

// key is the colour as one number, red in the high byte: the total order the median cut sorts
// on, so the palette never depends on map iteration.
func (c colourCount) key() uint32 {
	return packColour(c.channels)
}

// Quantize returns an image of img's bounds holding at most maxColors distinct colours, as the
// tune preview's stand-in for the shipped GIF's palette. The palette is a median cut over img's
// colours, each entry the pixel-weighted mean of the colours its box holds, and every pixel is
// mapped to its nearest entry without dithering: the palette pass's ditherMode in encode.go, so a
// flat colour stays flat. The result is deterministic: one image gives equal pixels on every
// call.
//
// An image already holding no more than maxColors colours is returned as it is, img itself;
// otherwise the result is a new image and img is left untouched. A maxColors below 1 is taken
// as 1.
func Quantize(img *image.RGBA, maxColors int) *image.RGBA {
	maxColors = max(maxColors, 1)
	colours := distinctColours(img)
	if len(colours) <= maxColors {
		return img
	}

	palette := medianCut(colours, maxColors)
	lookup := make(map[uint32][channelCount]uint8, len(colours))
	for _, colour := range colours {
		lookup[colour.key()] = nearest(colour.channels, palette)
	}
	return remap(img, lookup)
}

// distinctColours counts img's colours inside its bounds, sorted by key.
func distinctColours(img *image.RGBA) []colourCount {
	counts := make(map[uint32]int)
	forEachPixel(img, func(pixel []uint8) {
		counts[packColour([channelCount]uint8(pixel))]++
	})

	colours := make([]colourCount, 0, len(counts))
	for key, pixels := range counts {
		colours = append(colours, colourCount{channels: unpackColour(key), pixels: pixels})
	}
	slices.SortFunc(colours, func(a, b colourCount) int { return cmp.Compare(a.key(), b.key()) })
	return colours
}

// medianCut splits colours, which hold more than boxes distinct colours, into boxes boxes and
// returns each box's pixel-weighted mean. Each round splits the box with the widest channel
// range, the earliest on a tie, at the pixel median along that channel.
func medianCut(colours []colourCount, boxes int) [][channelCount]uint8 {
	cut := [][]colourCount{colours}
	for len(cut) < boxes {
		widest, channel := widestBox(cut)
		if widest < 0 {
			break
		}
		low, high := splitBox(cut[widest], channel)
		cut[widest] = low
		cut = slices.Insert(cut, widest+1, high)
	}

	palette := make([][channelCount]uint8, 0, len(cut))
	for _, box := range cut {
		palette = append(palette, meanColour(box))
	}
	return palette
}

// widestBox returns the index of the box with the widest range along one channel, and that
// channel; the earliest box and the lowest channel win a tie. It returns -1 when every box holds
// a single colour.
func widestBox(cut [][]colourCount) (int, int) {
	widest, widestChannel, widestRange := -1, 0, 0
	for index, box := range cut {
		for channel := range channelCount {
			low, high := uint8(math.MaxUint8), uint8(0)
			for _, colour := range box {
				low = min(low, colour.channels[channel])
				high = max(high, colour.channels[channel])
			}
			if spread := int(high) - int(low); spread > widestRange {
				widest, widestChannel, widestRange = index, channel, spread
			}
		}
	}
	return widest, widestChannel
}

// splitBox sorts box along channel, the key breaking ties, and splits it where half its pixels
// have been passed; both halves hold at least one colour.
func splitBox(box []colourCount, channel int) ([]colourCount, []colourCount) {
	slices.SortFunc(box, func(a, b colourCount) int {
		return cmp.Or(
			cmp.Compare(a.channels[channel], b.channels[channel]),
			cmp.Compare(a.key(), b.key()),
		)
	})

	total := 0
	for _, colour := range box {
		total += colour.pixels
	}
	split, passed := 1, 0
	for index, colour := range box {
		passed += colour.pixels
		if passed*2 >= total {
			split = index + 1
			break
		}
	}
	split = min(max(split, 1), len(box)-1)
	return box[:split], box[split:]
}

// meanColour is the pixel-weighted mean of box's colours, rounded to the nearest channel value.
func meanColour(box []colourCount) [channelCount]uint8 {
	var sums [channelCount]int
	total := 0
	for _, colour := range box {
		for channel := range channelCount {
			sums[channel] += int(colour.channels[channel]) * colour.pixels
		}
		total += colour.pixels
	}

	var mean [channelCount]uint8
	for channel := range channelCount {
		mean[channel] = uint8((sums[channel] + total/2) / total)
	}
	return mean
}

// nearest returns the palette entry closest to colour by squared distance over all channels, the
// earliest entry on a tie.
func nearest(colour [channelCount]uint8, palette [][channelCount]uint8) [channelCount]uint8 {
	best, bestDistance := palette[0], -1
	for _, entry := range palette {
		distance := 0
		for channel := range channelCount {
			delta := int(colour[channel]) - int(entry[channel])
			distance += delta * delta
		}
		if bestDistance < 0 || distance < bestDistance {
			best, bestDistance = entry, distance
		}
	}
	return best
}

// remap returns a new image of img's bounds with every pixel replaced by its lookup entry.
func remap(img *image.RGBA, lookup map[uint32][channelCount]uint8) *image.RGBA {
	out := image.NewRGBA(img.Bounds())
	offset := 0
	forEachPixel(img, func(pixel []uint8) {
		entry := lookup[packColour([channelCount]uint8(pixel))]
		copy(out.Pix[offset:offset+channelCount], entry[:])
		offset += channelCount
	})
	return out
}

// forEachPixel calls visit with each pixel's four channel bytes inside img's bounds, row by row.
func forEachPixel(img *image.RGBA, visit func(pixel []uint8)) {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		row := img.PixOffset(bounds.Min.X, y)
		for x := range bounds.Dx() {
			at := row + x*channelCount
			visit(img.Pix[at : at+channelCount])
		}
	}
}

// packColour folds four channel values into one key, red in the high byte.
func packColour(channels [channelCount]uint8) uint32 {
	return uint32(channels[0])<<24 | uint32(channels[1])<<16 |
		uint32(channels[2])<<8 | uint32(channels[3])
}

// unpackColour is packColour's inverse.
func unpackColour(key uint32) [channelCount]uint8 {
	return [channelCount]uint8{uint8(key >> 24), uint8(key >> 16), uint8(key >> 8), uint8(key)}
}
