package storyboard

import (
	"errors"
	"fmt"
	"strings"
)

// Run is a stretch of a caption's text drawn in one colour and one weight: a span's text, or the
// unstyled text between spans, which takes the caption's own colour and weight.
type Run struct {
	Text   string
	Color  RGB
	Weight Weight
}

// The span markup a caption's text may hold: [ opens a tag, ] ends it, [[ is a literal [ and
// [/] closes the open span. A tag holds space-separated tokens, each bold or a #rrggbb colour.
const (
	tagOpen      = '['
	tagClose     = ']'
	spanClose    = "/"
	literalOpen  = "[["
	boldToken    = "bold"
	escapeAdvice = "write [[ for a literal ["
)

// errSpanMarkup marks every markup problem, so each one's message ends naming the [[ escape: a
// caption with a literal [ written before spans existed fails with a hint at the fix.
var errSpanMarkup = errors.New(escapeAdvice)

// spanStyle is what a tag sets on the span it opens: a colour, a bold weight, or both.
type spanStyle struct {
	color    RGB
	hasColor bool
	isBold   bool
}

// markupParser turns a caption's text into its runs, one run per span and per stretch of
// unstyled text, styling them on the caption's own color and weight.
type markupParser struct {
	color  RGB
	weight Weight
	runs   []Run
	text   strings.Builder
	// openTag is the tag of the span the parser is inside, and the empty string outside one.
	openTag string
	style   spanStyle
}

// parseRuns splits text into its runs: unstyled text carries color and weight, a span the
// colour and weight its tag sets, falling back to color and weight for what the tag leaves out.
// Text without tags yields exactly one run of the whole text. A span may cross a newline; spans
// do not nest. It refuses an unknown token, two colours in one tag, a tag with no token or no
// closing ], a span never closed, a [/] outside a span, a tag inside a span and an empty span.
func parseRuns(text string, color RGB, weight Weight) ([]Run, error) {
	parser := markupParser{color: color, weight: weight}
	for rest := text; rest != ""; {
		var err error
		if rest, err = parser.consume(rest); err != nil {
			return nil, fmt.Errorf("%w (%w)", err, errSpanMarkup)
		}
	}
	if parser.openTag != "" {
		return nil, fmt.Errorf("span %s is never closed by [/] (%w)", parser.openTag, errSpanMarkup)
	}
	parser.flush()
	return parser.runs, nil
}

// consume reads the next character, escape or tag from rest and returns what follows it.
func (p *markupParser) consume(rest string) (string, error) {
	if strings.HasPrefix(rest, literalOpen) {
		p.text.WriteByte(tagOpen)
		return rest[len(literalOpen):], nil
	}
	if rest[0] != tagOpen {
		next := strings.IndexByte(rest, tagOpen)
		if next < 0 {
			next = len(rest)
		}
		p.text.WriteString(rest[:next])
		return rest[next:], nil
	}
	end := strings.IndexByte(rest, tagClose)
	if end < 0 {
		return "", fmt.Errorf("tag %q has no closing ]", rest)
	}
	return rest[end+1:], p.applyTag(rest[:end+1])
}

// applyTag closes the open span on [/], or opens a span styled by tag.
func (p *markupParser) applyTag(tag string) error {
	content := tag[1 : len(tag)-1]
	if strings.TrimSpace(content) == spanClose {
		return p.closeSpan()
	}
	if p.openTag != "" {
		return fmt.Errorf("tag %s inside span %s: spans do not nest", tag, p.openTag)
	}
	style, err := parseSpanStyle(tag, content)
	if err != nil {
		return err
	}
	p.flush()
	p.openTag, p.style = tag, style
	return nil
}

// closeSpan ends the open span as one run in its style.
func (p *markupParser) closeSpan() error {
	if p.openTag == "" {
		return errors.New("[/] closes no span")
	}
	if p.text.Len() == 0 {
		return fmt.Errorf("span %s holds no text", p.openTag)
	}
	p.flush()
	p.openTag, p.style = "", spanStyle{}
	return nil
}

// flush ends the text read so far as a run, in the open span's style or the caption's own; it
// adds nothing when no text was read.
func (p *markupParser) flush() {
	if p.text.Len() == 0 {
		return
	}
	run := Run{Text: p.text.String(), Color: p.color, Weight: p.weight}
	if p.style.hasColor {
		run.Color = p.style.color
	}
	if p.style.isBold {
		run.Weight = WeightBold
	}
	p.runs = append(p.runs, run)
	p.text.Reset()
}

// parseSpanStyle reads the tokens of a tag's content: each bold or a #rrggbb colour, at least
// one, at most one colour.
func parseSpanStyle(tag, content string) (spanStyle, error) {
	tokens := strings.Fields(content)
	if len(tokens) == 0 {
		return spanStyle{}, fmt.Errorf("tag %s: want %s or #rrggbb, got no token", tag, boldToken)
	}
	var style spanStyle
	for _, token := range tokens {
		if token == boldToken {
			style.isBold = true
			continue
		}
		color, isColor := parseColor(token)
		if !isColor {
			return spanStyle{}, fmt.Errorf("tag %s: want %s or #rrggbb, got %q",
				tag, boldToken, token)
		}
		if style.hasColor {
			return spanStyle{}, fmt.Errorf("tag %s: want at most one colour", tag)
		}
		style.color, style.hasColor = color, true
	}
	return style, nil
}
