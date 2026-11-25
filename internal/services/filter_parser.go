package services

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/techasit5415/microbase/internal/models"
)

type fTokKind int

const (
	fTokEOF fTokKind = iota
	fTokIdent
	fTokString
	fTokNumber
	fTokOp
	fTokAnd
	fTokOr
	fTokLParen
	fTokRParen
)

type fToken struct {
	kind fTokKind
	val  string
}

func tokenizeFilter(input string) ([]fToken, error) {
	var toks []fToken
	i := 0
	n := len(input)
	for i < n {
		c := input[i]
		if unicode.IsSpace(rune(c)) {
			i++
			continue
		}
		if c == '(' {
			toks = append(toks, fToken{fTokLParen, "("})
			i++
			continue
		}
		if c == ')' {
			toks = append(toks, fToken{fTokRParen, ")"})
			i++
			continue
		}
		if i+1 < n {
			two := input[i : i+2]
			switch two {
			case "&&":
				toks = append(toks, fToken{fTokAnd, "&&"})
				i += 2
				continue
			case "||":
				toks = append(toks, fToken{fTokOr, "||"})
				i += 2
				continue
			case "!=", ">=", "<=", "!~":
				toks = append(toks, fToken{fTokOp, two})
				i += 2
				continue
			}
		}
		if c == '=' || c == '>' || c == '<' || c == '~' {
			toks = append(toks, fToken{fTokOp, string(c)})
			i++
			continue
		}
		if c == '\'' || c == '"' {
			quote := c
			i++
			var sb strings.Builder
			for i < n && input[i] != quote {
				if input[i] == '\\' && i+1 < n {
					i++
				}
				sb.WriteByte(input[i])
				i++
			}
			if i >= n {
				return nil, fmt.Errorf("unterminated string literal in filter")
			}
			i++ // consume closing quote
			toks = append(toks, fToken{fTokString, sb.String()})
			continue
		}
		if unicode.IsDigit(rune(c)) || (c == '-' && i+1 < n && unicode.IsDigit(rune(input[i+1]))) {
			start := i
			i++
			for i < n && (unicode.IsDigit(rune(input[i])) || input[i] == '.') {
				i++
			}
			toks = append(toks, fToken{fTokNumber, input[start:i]})
			continue
		}
		if unicode.IsLetter(rune(c)) || c == '_' || c == '@' {
			start := i
			i++
			for i < n && (unicode.IsLetter(rune(input[i])) || unicode.IsDigit(rune(input[i])) || input[i] == '_' || input[i] == '.') {
				i++
			}
			toks = append(toks, fToken{fTokIdent, input[start:i]})
			continue
		}
		return nil, fmt.Errorf("unexpected character %q in filter", c)
	}
	toks = append(toks, fToken{fTokEOF, ""})
	return toks, nil
}

type filterParser struct {
	toks    []fToken
	pos     int
	allowed map[string]models.FieldDef
	claims  *AuthClaims
	args    []any
	nextArg int
}

// ParseFilterToSQL compiles a PocketBase filter string into a safe parameterized PostgreSQL WHERE snippet.
func ParseFilterToSQL(expr string, allowed map[string]models.FieldDef, claims *AuthClaims, startArgIdx int) (string, []any, int, error) {
	toks, err := tokenizeFilter(expr)
	if err != nil {
		return "", nil, startArgIdx, err
	}
	p := &filterParser{
		toks:    toks,
		allowed: allowed,
		claims:  claims,
		nextArg: startArgIdx,
	}
	sql, err := p.parseOr()
	if err != nil {
		return "", nil, startArgIdx, err
	}
	if p.peek().kind != fTokEOF {
		return "", nil, startArgIdx, fmt.Errorf("unexpected token %q in filter", p.peek().val)
	}
	return sql, p.args, p.nextArg, nil
}

func (p *filterParser) peek() fToken { return p.toks[p.pos] }
func (p *filterParser) next() fToken { t := p.toks[p.pos]; p.pos++; return t }

func (p *filterParser) parseOr() (string, error) {
	left, err := p.parseAnd()
	if err != nil {
		return "", err
	}
	for p.peek().kind == fTokOr {
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return "", err
		}
		left = fmt.Sprintf("(%s OR %s)", left, right)
	}
	return left, nil
}

func (p *filterParser) parseAnd() (string, error) {
	left, err := p.parseAtom()
	if err != nil {
		return "", err
	}
	for p.peek().kind == fTokAnd {
		p.next()
		right, err := p.parseAtom()
		if err != nil {
			return "", err
		}
		left = fmt.Sprintf("(%s AND %s)", left, right)
	}
	return left, nil
}

func (p *filterParser) parseAtom() (string, error) {
	if p.peek().kind == fTokLParen {
		p.next()
		inner, err := p.parseOr()
		if err != nil {
			return "", err
		}
		if p.peek().kind != fTokRParen {
			return "", fmt.Errorf("missing closing ')' in filter")
		}
		p.next()
		return "(" + inner + ")", nil
	}

	lhs := p.next()
	if lhs.kind != fTokIdent {
		return "", fmt.Errorf("expected field name, got %q", lhs.val)
	}
	op := p.next()
	if op.kind != fTokOp {
		return "", fmt.Errorf("expected operator after %q, got %q", lhs.val, op.val)
	}
	rhs := p.next()

	val, isNull, err := p.resolveValue(rhs)
	if err != nil {
		return "", err
	}

	// Allow @request.auth.* on LHS for API rules evaluated statically or inline
	if strings.HasPrefix(lhs.val, "@request.auth.") {
		lhsVal, _, _ := p.resolveValue(lhs)
		ok := fmt.Sprint(lhsVal) == fmt.Sprint(val)
		if op.val == "!=" {
			ok = !ok
		}
		if ok {
			return "TRUE", nil
		}
		return "FALSE", nil
	}

	if _, ok := p.allowed[lhs.val]; !ok && lhs.val != "id" && lhs.val != "created_at" && lhs.val != "updated_at" {
		return "", fmt.Errorf("unknown filter field %q", lhs.val)
	}

	colSQL := fmt.Sprintf(`"%s"`, lhs.val)
	if isNull {
		if op.val == "=" {
			return colSQL + " IS NULL", nil
		}
		return colSQL + " IS NOT NULL", nil
	}

	switch op.val {
	case "~":
		p.args = append(p.args, "%"+fmt.Sprint(val)+"%")
		idx := p.nextArg
		p.nextArg++
		return fmt.Sprintf("CAST(%s AS TEXT) ILIKE $%d", colSQL, idx), nil
	case "!~":
		p.args = append(p.args, "%"+fmt.Sprint(val)+"%")
		idx := p.nextArg
		p.nextArg++
		return fmt.Sprintf("CAST(%s AS TEXT) NOT ILIKE $%d", colSQL, idx), nil
	default:
		p.args = append(p.args, val)
		idx := p.nextArg
		p.nextArg++
		return fmt.Sprintf("%s %s $%d", colSQL, op.val, idx), nil
	}
}

func (p *filterParser) resolveValue(tok fToken) (any, bool, error) {
	switch tok.kind {
	case fTokString:
		return tok.val, false, nil
	case fTokNumber:
		if strings.Contains(tok.val, ".") {
			f, err := strconv.ParseFloat(tok.val, 64)
			return f, false, err
		}
		n, err := strconv.ParseInt(tok.val, 10, 64)
		return n, false, err
	case fTokIdent:
		lower := strings.ToLower(tok.val)
		switch lower {
		case "true":
			return true, false, nil
		case "false":
			return false, false, nil
		case "null":
			return nil, true, nil
		case "@request.auth.id":
			if p.claims == nil {
				return "", false, nil
			}
			return p.claims.UserID, false, nil
		case "@request.auth.email":
			if p.claims == nil {
				return "", false, nil
			}
			return p.claims.Email, false, nil
		case "@request.auth.role":
			if p.claims == nil {
				return "", false, nil
			}
			return p.claims.Role, false, nil
		default:
			return tok.val, false, nil
		}
	default:
		return nil, false, fmt.Errorf("unexpected value token %q", tok.val)
	}
}
