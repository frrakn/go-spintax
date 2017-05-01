package spintax

import (
	"bytes"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"time"

	"github.com/pkg/errors"
)

var (
	slashes = regexp.MustCompile("\\\\(.)")
)

type Spintax struct {
	symbolTable map[string]*expression
	*expression
}

const (
	PICKER = iota
	VARIABLE
)

type spintax struct {
	spintaxType int32
	*picker
	*variable
}

type picker struct {
	numToPick   int32
	expressions []*expression
}

type variable struct {
	lookup *map[string]*expression
	symbol string
}

type expression struct {
	elements []*expressionElement
}

const (
	STRING_EXPRESSION = iota
	SPINTAX_EXPRESSION
)

type expressionElement struct {
	expressionType int32
	str            string
	spintax        *spintax
}

func New(expr string) (*Spintax, error) {
	s := &Spintax{
		symbolTable: map[string]*expression{},
	}

	tokens, err := tokenize(expr)
	if err != nil {
		return nil, errors.Wrap(err, "unable to tokenize expression")
	}

	err = s.parse(tokens)
	if err != nil {
		return nil, errors.Wrap(err, "unable to parse tokens")
	}

	return s, nil
}

func (s *Spintax) Spin() string {
	return s.expression.spin()
}

func (s *Spintax) Define(symbol string, variable string) error {
	tokens, err := tokenize(variable)
	if err != nil {
		return errors.Wrap(err, "unable to tokenize expression")
	}
	expr, ts, err := s.parseExpression(tokens)
	if err != nil {
		return errors.Wrap(err, "error parsing expression")
	}
	if len(ts) > 0 {
		return errors.WithMessage(err, "Leftover tokens after expression parse")
	}
	s.symbolTable[symbol] = expr
	return nil
}

func (e *expression) spin() string {
	var b bytes.Buffer
	for _, ele := range e.elements {
		b.WriteString(ele.spin())
	}
	return b.String()
}

func (e *expressionElement) spin() string {
	switch e.expressionType {
	case STRING_EXPRESSION:
		return e.str
		break
	case SPINTAX_EXPRESSION:
		return e.spintax.spin()
		break
	default:
		panic("Invalid spintax parse tree")
	}
	return ""
}

func (s *spintax) spin() string {
	switch s.spintaxType {
	case PICKER:
		rand.Seed(time.Now().UnixNano())
		randomIndicies := rand.Perm(len(s.picker.expressions))
		selectedIndicies := randomIndicies[:s.picker.numToPick]
		var b bytes.Buffer
		for _, i := range selectedIndicies {
			b.WriteString(s.picker.expressions[i].spin())
		}
		return b.String()
		break
	case VARIABLE:
		lookup := *s.lookup
		return lookup[s.symbol].spin()
		break
	default:
		panic("Invalid spintax parse tree")
	}
	return ""
}

func (s *Spintax) parse(tokens []string) error {
	expr, ts, err := s.parseExpression(tokens)
	if err != nil {
		return errors.Wrap(err, "error parsing expression")
	}
	if len(ts) > 0 {
		return errors.New("Leftover tokens after expression parse")
	}

	s.expression = expr
	return nil
}

func (s *Spintax) parseExpression(tokens []string) (expr *expression, ts []string, err error) {
	expr = &expression{
		elements: []*expressionElement{},
	}
	ts = tokens

	for len(ts) > 0 {
		next := ts[0]
		switch next {
		case "{":
			fallthrough
		case "{:":
			fallthrough
		case "[":
			var (
				sp  *spintax
				err error
			)
			sp, ts, err = s.parseSpintax(ts)
			if err != nil {
				return nil, nil, errors.Wrap(err, "error parsing spintax")
			}
			expr.elements = append(expr.elements, &expressionElement{
				expressionType: SPINTAX_EXPRESSION,
				spintax:        sp,
			})
			break
		case "|":
			fallthrough
		case "}":
			return expr, ts, nil
			break
		default:
			expr.elements = append(expr.elements, &expressionElement{
				expressionType: STRING_EXPRESSION,
				str:            slashes.ReplaceAllString(next, "$1"),
			})
			ts = ts[1:]
		}
	}

	return expr, ts, nil
}

func (s *Spintax) parseSpintax(tokens []string) (sptx *spintax, ts []string, err error) {
	ts = tokens
	next := ts[0]
	switch next {
	case "{:":
		if len(ts) < 3 || ts[2] != ":" {
			return nil, nil, errors.WithMessage(err, "Unrecognized numbered picker syntax")
		}
		var (
			parsedPickerValue int64
			err               error
		)
		if parsedPickerValue, err = strconv.ParseInt(ts[1], 10, 32); err != nil {
			return nil, nil, errors.WithMessage(err, fmt.Sprintf("Invalid picker value %s", ts[1]))
		}

		sptx = &spintax{
			spintaxType: PICKER,
			picker: &picker{
				numToPick:   int32(parsedPickerValue),
				expressions: []*expression{},
			},
		}
		ts = ts[3:]
		break
	case "{":
		sptx = &spintax{
			spintaxType: PICKER,
			picker: &picker{
				numToPick:   1,
				expressions: []*expression{},
			},
		}
		ts = ts[1:]
		break
	case "[":
		sptx = &spintax{
			spintaxType: VARIABLE,
		}
		ts = ts[1:]
		break
	default:
		return nil, nil, errors.New("Unrecognized spintax beginning token identifier")
	}

	for len(ts) > 0 {
		switch sptx.spintaxType {
		case PICKER:
			picker := sptx.picker
			var (
				expr *expression
				err  error
			)
			expr, ts, err = s.parseExpression(ts)
			if err != nil {
				return nil, nil, errors.Wrap(err, "unable to parse expression")
			}
			picker.expressions = append(picker.expressions, expr)
			sptx.picker = picker

			if len(ts) == 0 {
				return nil, nil, errors.WithMessage(err, "Unexpected end of expression while parsing picker")
			}
			next := ts[0]
			ts = ts[1:]

			switch next {
			case "}":
				if int(picker.numToPick) > len(picker.expressions) {
					return nil, nil, errors.WithMessage(err, "Defined a numbered picker without enough choices")
				}
				return sptx, ts, nil
			case "|":
				break
			default:
				return nil, nil, errors.WithMessage(err, "Unrecognized token in picker")
			}
			break
		case VARIABLE:
			if len(ts) < 2 || ts[1] != "]" {
				return nil, nil, errors.WithMessage(err, "Unrecognized variable syntax")
			}
			sptx.variable = &variable{
				symbol: ts[0],
				lookup: &s.symbolTable,
			}
			ts = ts[2:]
			return sptx, ts, nil
			break
		default:
			return nil, nil, errors.WithMessage(err, "Unrecognized spintax type")
		}
	}

	return nil, nil, errors.New("Ran out of tokens parsing spintax")
}

func tokenize(expr string) ([]string, error) {
	var tokens []string

	state := 0
	const (
		TEXT = iota
		OPEN_BRACE
		OPEN_BRACE_COLON
		VARIABLE
	)

	tmp := ""
	escape := false
	for k, r := range expr {
		char := string(r)

		if !escape && char == "\\" {
			escape = true
			continue
		}

		switch state {
		case TEXT:
			if escape {
				escape = false
				tmp += ("\\" + char)
				break
			}
			switch char {
			case "{":
				state = OPEN_BRACE
				if tmp != "" {
					tokens = append(tokens, tmp)
				}
				tmp = char
				break
			case "[":
				state = VARIABLE
				if tmp != "" {
					tokens = append(tokens, tmp)
				}
				tokens = append(tokens, char)
				tmp = ""
				break
			case "}":
				fallthrough
			case "|":
				if tmp != "" {
					tokens = append(tokens, tmp)
				}
				tokens = append(tokens, char)
				tmp = ""
				break
			default:
				tmp += char
			}
			break
		case OPEN_BRACE:
			if escape {
				escape = false
				state = TEXT
				if tmp != "" {
					tokens = append(tokens, tmp)
				}
				tmp = "\\" + char
				break
			}
			switch char {
			case "{":
				state = OPEN_BRACE
				if tmp != "" {
					tokens = append(tokens, tmp)
				}
				tmp = char
				break
			case ":":
				state = OPEN_BRACE_COLON
				tokens = append(tokens, tmp+char)
				tmp = ""
				break
			case "[":
				state = VARIABLE
				if tmp != "" {
					tokens = append(tokens, tmp)
				}
				tokens = append(tokens, char)
				tmp = ""
				break
			case "}":
				fallthrough
			case "|":
				if tmp != "" {
					tokens = append(tokens, tmp)
				}
				tokens = append(tokens, char)
				tmp = ""
				break
			default:
				state = TEXT
				if tmp != "" {
					tokens = append(tokens, tmp)
				}
				tmp = char
			}
			break
		case OPEN_BRACE_COLON:
			switch char {
			case "0":
				fallthrough
			case "1":
				fallthrough
			case "2":
				fallthrough
			case "3":
				fallthrough
			case "4":
				fallthrough
			case "5":
				fallthrough
			case "6":
				fallthrough
			case "7":
				fallthrough
			case "8":
				fallthrough
			case "9":
				tmp = char
				break
			case ":":
				state = TEXT
				if tmp != "" {
					tokens = append(tokens, tmp)
				}
				tokens = append(tokens, char)
				tmp = ""
				break
			default:
				return nil, errors.Errorf("Unrecognized syntax at rune %d", k)
			}
			break
		case VARIABLE:
			if escape {
				escape = false
				tmp += ("\\" + char)
				break
			}
			switch char {
			case "]":
				state = TEXT
				if tmp != "" {
					tokens = append(tokens, tmp)
				}
				tokens = append(tokens, char)
				tmp = ""
				break
			default:
				tmp += char
			}
			break
		default:
			return nil, errors.Errorf("Unrecognized state at rune %d", k)
		}
	}

	if tmp != "" {
		tokens = append(tokens, tmp)
	}

	return tokens, nil
}
