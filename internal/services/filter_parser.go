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
	body    map[string]any
	args    []any
	nextArg int
}

// ParseFilterToSQL compiles a PocketBase filter string into a safe parameterized PostgreSQL WHERE snippet.
func ParseFilterToSQL(expr string, allowed map[string]models.FieldDef, claims *AuthClaims, startArgIdx int) (string, []any, int, error) {
	return ParseFilterToSQLWithBody(expr, allowed, claims, nil, startArgIdx)
}

// ParseFilterToSQLWithBody compiles a PocketBase filter or API rule expression into parameterized PostgreSQL SQL.
// Supports column identifiers on either side, @request.auth.*, @request.body.*, &&, ||, (), and =, !=, >, >=, <, <=, ~, !~.
func ParseFilterToSQLWithBody(expr string, allowed map[string]models.FieldDef, claims *AuthClaims, body map[string]any, startArgIdx int) (string, []any, int, error) {
	toks, err := tokenizeFilter(expr)
	if err != nil {
		return "", nil, startArgIdx, err
	}
	p := &filterParser{
		toks:    toks,
		allowed: allowed,
		claims:  claims,
		body:    body,
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
func (p *filterParser) next() fToken {
	t := p.peek()
	if t.kind != fTokEOF {
		p.pos++
	}
	return t
}

func (p *filterParser) isColumnIdent(tok fToken) bool {
	if tok.kind != fTokIdent {
		return false
	}
	lower := strings.ToLower(tok.val)
	if lower == "true" || lower == "false" || lower == "null" || strings.HasPrefix(lower, "@request.") {
		return false
	}
	if tok.val == "id" || tok.val == "created_at" || tok.val == "updated_at" {
		return true
	}
	_, ok := p.allowed[tok.val]
	return ok
}

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

func flipOperator(op string) string {
	switch op {
	case ">":
		return "<"
	case ">=":
		return "<="
	case "<":
		return ">"
	case "<=":
		return ">="
	default:
		return op
	}
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
	op := p.next()
	if op.kind != fTokOp {
		return "", fmt.Errorf("expected operator after %q, got %q", lhs.val, op.val)
	}
	rhs := p.next()

	lhsIsCol := p.isColumnIdent(lhs)
	rhsIsCol := p.isColumnIdent(rhs)

	// Case 1: Column vs Column (e.g., col1 != col2)
	if lhsIsCol && rhsIsCol {
		lSQL := fmt.Sprintf(`"%s"`, lhs.val)
		rSQL := fmt.Sprintf(`"%s"`, rhs.val)
		switch op.val {
		case "~":
			return fmt.Sprintf("CAST(%s AS TEXT) ILIKE ('%%' || CAST(%s AS TEXT) || '%%')", lSQL, rSQL), nil
		case "!~":
			return fmt.Sprintf("CAST(%s AS TEXT) NOT ILIKE ('%%' || CAST(%s AS TEXT) || '%%')", lSQL, rSQL), nil
		default:
			return fmt.Sprintf("%s %s %s", lSQL, op.val, rSQL), nil
		}
	}

	// Case 2: Both sides are static literals / @request.* values -> evaluate immediately
	if !lhsIsCol && !rhsIsCol {
		// Reject unknown bare identifiers that aren't literals or @request.*
		if lhs.kind == fTokIdent && !isLiteralOrRequestToken(lhs.val) {
			return "", fmt.Errorf("unknown filter field %q", lhs.val)
		}
		lhsVal, lhsNull, err := p.resolveValue(lhs)
		if err != nil {
			return "", err
		}
		rhsVal, rhsNull, err := p.resolveValue(rhs)
		if err != nil {
			return "", err
		}
		if evalComparison(lhsVal, lhsNull, op.val, rhsVal, rhsNull) {
			return "TRUE", nil
		}
		return "FALSE", nil
	}

	// Case 3: One side is a column, the other is a value/literal/@request.*
	colTok := lhs
	valTok := rhs
	effOp := op.val
	if !lhsIsCol && rhsIsCol {
		colTok = rhs
		valTok = lhs
		effOp = flipOperator(op.val)
	}

	val, isNull, err := p.resolveValue(valTok)
	if err != nil {
		return "", err
	}

	colSQL := fmt.Sprintf(`"%s"`, colTok.val)
	if isNull {
		if effOp == "=" {
			return colSQL + " IS NULL", nil
		}
		if effOp == "!=" {
			return colSQL + " IS NOT NULL", nil
		}
		return "FALSE", nil
	}

	// When comparing against @request.auth.id (which may be int64 or "" when unauthenticated),
	// safely compare as TEXT so BIGINT vs TEXT columns never throw a PostgreSQL cast error.
	if valTok.kind == fTokIdent && strings.EqualFold(valTok.val, "@request.auth.id") && (effOp == "=" || effOp == "!=") {
		strVal := fmt.Sprint(val)
		if strVal == "" {
			if effOp == "=" {
				return "FALSE", nil
			}
			return "TRUE", nil
		}
		p.args = append(p.args, strVal)
		idx := p.nextArg
		p.nextArg++
		return fmt.Sprintf("CAST(%s AS TEXT) %s $%d", colSQL, effOp, idx), nil
	}

	switch effOp {
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
		return fmt.Sprintf("%s %s $%d", colSQL, effOp, idx), nil
	}
}

func isLiteralOrRequestToken(val string) bool {
	lower := strings.ToLower(val)
	return lower == "true" || lower == "false" || lower == "null" || strings.HasPrefix(lower, "@request.")
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
		switch {
		case lower == "true":
			return true, false, nil
		case lower == "false":
			return false, false, nil
		case lower == "null":
			return nil, true, nil
		case lower == "@request.auth.id":
			if p.claims == nil {
				return "", false, nil
			}
			if p.claims.RecordID != "" {
				return p.claims.RecordID, false, nil
			}
			return p.claims.UserID, false, nil
		case lower == "@request.auth.email":
			if p.claims == nil {
				return "", false, nil
			}
			return p.claims.Email, false, nil
		case lower == "@request.auth.role":
			if p.claims == nil {
				return "", false, nil
			}
			return p.claims.Role, false, nil
		case strings.HasPrefix(lower, "@request.body."):
			field := tok.val[len("@request.body."):]
			if p.body == nil {
				return nil, true, nil
			}
			v, ok := p.body[field]
			if !ok || v == nil {
				return nil, true, nil
			}
			return v, false, nil
		default:
			return tok.val, false, nil
		}
	default:
		return nil, false, fmt.Errorf("unexpected value token %q", tok.val)
	}
}

// EvalFilterOnData evaluates a PocketBase expression in-memory against a record/payload map (used for create rules).
func EvalFilterOnData(expr string, data map[string]any, claims *AuthClaims) (bool, error) {
	toks, err := tokenizeFilter(expr)
	if err != nil {
		return false, err
	}
	ev := &dataRuleEval{
		toks:   toks,
		data:   data,
		claims: claims,
	}
	ok, err := ev.evalOr()
	if err != nil {
		return false, err
	}
	if ev.peek().kind != fTokEOF {
		return false, fmt.Errorf("unexpected token %q in rule", ev.peek().val)
	}
	return ok, nil
}

type dataRuleEval struct {
	toks   []fToken
	pos    int
	data   map[string]any
	claims *AuthClaims
}

func (e *dataRuleEval) peek() fToken { return e.toks[e.pos] }
func (e *dataRuleEval) next() fToken {
	t := e.peek()
	if t.kind != fTokEOF {
		e.pos++
	}
	return t
}

func (e *dataRuleEval) evalOr() (bool, error) {
	left, err := e.evalAnd()
	if err != nil {
		return false, err
	}
	for e.peek().kind == fTokOr {
		e.next()
		right, err := e.evalAnd()
		if err != nil {
			return false, err
		}
		left = left || right
	}
	return left, nil
}

func (e *dataRuleEval) evalAnd() (bool, error) {
	left, err := e.evalAtom()
	if err != nil {
		return false, err
	}
	for e.peek().kind == fTokAnd {
		e.next()
		right, err := e.evalAtom()
		if err != nil {
			return false, err
		}
		left = left && right
	}
	return left, nil
}

func (e *dataRuleEval) evalAtom() (bool, error) {
	if e.peek().kind == fTokLParen {
		e.next()
		inner, err := e.evalOr()
		if err != nil {
			return false, err
		}
		if e.peek().kind != fTokRParen {
			return false, fmt.Errorf("missing closing ')' in rule")
		}
		e.next()
		return inner, nil
	}
	lhs := e.next()
	op := e.next()
	if op.kind != fTokOp {
		return false, fmt.Errorf("expected operator after %q, got %q", lhs.val, op.val)
	}
	rhs := e.next()
	for _, tok := range []fToken{lhs, rhs} {
		if tok.kind != fTokIdent && tok.kind != fTokString && tok.kind != fTokNumber {
			return false, fmt.Errorf("expected rule operand, got %q", tok.val)
		}
		if tok.kind == fTokNumber {
			if _, err := strconv.ParseFloat(tok.val, 64); err != nil {
				return false, err
			}
		}
	}
	lv, lNull := e.resolve(lhs)
	rv, rNull := e.resolve(rhs)
	return evalComparison(lv, lNull, op.val, rv, rNull), nil
}

func (e *dataRuleEval) resolve(tok fToken) (any, bool) {
	switch tok.kind {
	case fTokString:
		return tok.val, false
	case fTokNumber:
		if strings.Contains(tok.val, ".") {
			f, _ := strconv.ParseFloat(tok.val, 64)
			return f, false
		}
		n, _ := strconv.ParseInt(tok.val, 10, 64)
		return n, false
	case fTokIdent:
		lower := strings.ToLower(tok.val)
		switch {
		case lower == "true":
			return true, false
		case lower == "false":
			return false, false
		case lower == "null":
			return nil, true
		case lower == "@request.auth.id":
			if e.claims == nil {
				return "", false
			}
			if e.claims.RecordID != "" {
				return e.claims.RecordID, false
			}
			return e.claims.UserID, false
		case lower == "@request.auth.email":
			if e.claims == nil {
				return "", false
			}
			return e.claims.Email, false
		case lower == "@request.auth.role":
			if e.claims == nil {
				return "", false
			}
			return e.claims.Role, false
		case strings.HasPrefix(lower, "@request.body."):
			field := tok.val[len("@request.body."):]
			if e.data == nil {
				return nil, true
			}
			v, ok := e.data[field]
			if !ok || v == nil {
				return nil, true
			}
			return v, false
		default:
			if e.data != nil {
				if v, ok := e.data[tok.val]; ok {
					if v == nil {
						return nil, true
					}
					return v, false
				}
			}
			return nil, true
		}
	default:
		return nil, true
	}
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func evalComparison(lhs any, lNull bool, op string, rhs any, rNull bool) bool {
	if lNull || rNull {
		switch op {
		case "=":
			return lNull && rNull
		case "!=":
			return lNull != rNull
		default:
			return false
		}
	}
	ls := fmt.Sprint(lhs)
	rs := fmt.Sprint(rhs)
	switch op {
	case "=":
		if lf, lok := toFloat64(lhs); lok {
			if rf, rok := toFloat64(rhs); rok {
				return lf == rf
			}
		}
		return ls == rs
	case "!=":
		if lf, lok := toFloat64(lhs); lok {
			if rf, rok := toFloat64(rhs); rok {
				return lf != rf
			}
		}
		return ls != rs
	case "~":
		return strings.Contains(strings.ToLower(ls), strings.ToLower(rs))
	case "!~":
		return !strings.Contains(strings.ToLower(ls), strings.ToLower(rs))
	case ">", ">=", "<", "<=":
		if lf, lok := toFloat64(lhs); lok {
			if rf, rok := toFloat64(rhs); rok {
				switch op {
				case ">":
					return lf > rf
				case ">=":
					return lf >= rf
				case "<":
					return lf < rf
				case "<=":
					return lf <= rf
				}
			}
		}
		switch op {
		case ">":
			return ls > rs
		case ">=":
			return ls >= rs
		case "<":
			return ls < rs
		case "<=":
			return ls <= rs
		}
	}
	return false
}
