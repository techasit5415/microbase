package services

import (
	"fmt"
	"strconv"
	"strings"
	"time"
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
		if i+2 < n {
			three := input[i : i+3]
			switch three {
			case "?!= ", "?!~", "?!= "[0:3], "?>=", "?<=":
				// handled cleanly below
			}
			if three == "?!=" || three == "?!~" || three == "?>=" || three == "?<=" {
				toks = append(toks, fToken{fTokOp, three})
				i += 3
				continue
			}
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
			case "==":
				toks = append(toks, fToken{fTokOp, "="})
				i += 2
				continue
			case "!=", ">=", "<=", "!~", "?=", "?~", "?>", "?<":
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
			for i < n && (unicode.IsLetter(rune(input[i])) || unicode.IsDigit(rune(input[i])) || input[i] == '_' || input[i] == '.' || input[i] == ':') {
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
	toks       []fToken
	pos        int
	tableName  string
	allowed    map[string]models.FieldDef
	createMode bool
	claims     *AuthClaims
	body       map[string]any
	args       []any
	nextArg    int
}

// ParseFilterToSQL compiles a PocketBase filter string into a safe parameterized PostgreSQL WHERE snippet.
func ParseFilterToSQL(expr string, allowed map[string]models.FieldDef, claims *AuthClaims, startArgIdx int) (string, []any, int, error) {
	return ParseFilterToSQLForTable("", expr, allowed, claims, nil, startArgIdx)
}

// ParseFilterToSQLWithBody compiles a PocketBase filter or API rule expression into parameterized PostgreSQL SQL.
// Supports column identifiers, relation traversal (rel.field, rel1.rel2.id), @request.auth.*, @request.body.* (:isset/:changed),
// @now, &&, ||, (), and =, !=, >, >=, <, <=, ~, !~, ?=, ?!=, ?~, ?!~, ?>, ?>=, ?<, ?<=.
func ParseFilterToSQLWithBody(expr string, allowed map[string]models.FieldDef, claims *AuthClaims, body map[string]any, startArgIdx int) (string, []any, int, error) {
	return ParseFilterToSQLForTable("", expr, allowed, claims, body, startArgIdx)
}

// ParseFilterToSQLForTable compiles a PocketBase filter expression with an explicit outer table name
// so relation subqueries (EXISTS) qualify outer columns (e.g. "instances"."email") and never collide
// with identically named columns on the target table (e.g. "users"."email").
func ParseFilterToSQLForTable(tableName, expr string, allowed map[string]models.FieldDef, claims *AuthClaims, body map[string]any, startArgIdx int) (string, []any, int, error) {
	toks, err := tokenizeFilter(expr)
	if err != nil {
		return "", nil, startArgIdx, err
	}
	p := &filterParser{
		toks:      toks,
		tableName: tableName,
		allowed:   allowed,
		claims:    claims,
		body:      body,
		nextArg:   startArgIdx,
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

// ParseCreateFilterToSQL compiles a PocketBase create-rule expression into a standalone boolean SQL expression
// where bare field names resolve from the incoming payload map rather than table columns.
func ParseCreateFilterToSQL(expr string, claims *AuthClaims, body map[string]any, startArgIdx int) (string, []any, int, error) {
	toks, err := tokenizeFilter(expr)
	if err != nil {
		return "", nil, startArgIdx, err
	}
	p := &filterParser{
		toks:       toks,
		allowed:    nil,
		createMode: true,
		claims:     claims,
		body:       body,
		nextArg:    startArgIdx,
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

func (p *filterParser) addArg(v any) int {
	p.args = append(p.args, v)
	idx := p.nextArg
	p.nextArg++
	return idx
}

func authUserIDStr(claims *AuthClaims) string {
	if claims == nil {
		return ""
	}
	if claims.RecordID != "" {
		return claims.RecordID
	}
	return claims.UserID
}

func isDynamicAuthPath(tok fToken) bool {
	if tok.kind != fTokIdent {
		return false
	}
	lower := strings.ToLower(tok.val)
	if !strings.HasPrefix(lower, "@request.auth.") {
		return false
	}
	rest := lower[len("@request.auth."):]
	return rest != "id" && rest != "email" && rest != "role" && rest != "collectionname"
}

func isBodyChangedToken(tok fToken) (string, bool) {
	if tok.kind != fTokIdent {
		return "", false
	}
	lower := strings.ToLower(tok.val)
	if strings.HasPrefix(lower, "@request.body.") && strings.HasSuffix(lower, ":changed") {
		field := tok.val[len("@request.body.") : len(tok.val)-len(":changed")]
		return field, true
	}
	return "", false
}

func (p *filterParser) isColumnIdent(tok fToken) bool {
	if tok.kind != fTokIdent || p.createMode {
		return false
	}
	lower := strings.ToLower(tok.val)
	if lower == "true" || lower == "false" || lower == "null" || lower == "@now" || strings.HasPrefix(lower, "@request.") {
		return false
	}
	if tok.val == "id" || tok.val == "created_at" || tok.val == "updated_at" {
		return true
	}
	if p.allowed == nil {
		return false
	}
	_, ok := p.allowed[tok.val]
	return ok
}

func (p *filterParser) isRelationPath(tok fToken) bool {
	if tok.kind != fTokIdent || p.allowed == nil || !strings.Contains(tok.val, ".") {
		return false
	}
	lower := strings.ToLower(tok.val)
	if strings.HasPrefix(lower, "@") {
		return false
	}
	parts := strings.Split(tok.val, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return false
	}
	for _, seg := range parts {
		if !models.IsValidIdentifier(seg) {
			return false
		}
	}
	_, ok := p.allowed[parts[0]]
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
	case "?>":
		return "?<"
	case "?>=":
		return "?<="
	case "?<":
		return "?>"
	case "?<=":
		return "?>="
	default:
		return op
	}
}

func stripAnyOp(op string) (string, bool) {
	if strings.HasPrefix(op, "?") {
		return op[1:], true
	}
	return op, false
}

func isNegatedOp(op string) bool {
	base, _ := stripAnyOp(op)
	return base == "!=" || base == "!~"
}

// buildJSONBMatchSQL matches a column (either JSONB array or scalar) against a text parameter $argIdx.
func buildJSONBMatchSQL(colExpr string, baseOp string, argIdx int) string {
	switch baseOp {
	case "=":
		return fmt.Sprintf("((jsonb_typeof(to_jsonb(%s)) = 'array' AND to_jsonb(%s) ? $%d) OR CAST(%s AS TEXT) = $%d)", colExpr, colExpr, argIdx, colExpr, argIdx)
	case "!=":
		return fmt.Sprintf("NOT ((jsonb_typeof(to_jsonb(%s)) = 'array' AND to_jsonb(%s) ? $%d) OR CAST(%s AS TEXT) = $%d)", colExpr, colExpr, argIdx, colExpr, argIdx)
	case "~":
		return fmt.Sprintf("((jsonb_typeof(to_jsonb(%s)) = 'array' AND EXISTS (SELECT 1 FROM jsonb_array_elements_text(to_jsonb(%s)) _el WHERE _el ILIKE $%d)) OR CAST(%s AS TEXT) ILIKE $%d)", colExpr, colExpr, argIdx, colExpr, argIdx)
	case "!~":
		return fmt.Sprintf("NOT ((jsonb_typeof(to_jsonb(%s)) = 'array' AND EXISTS (SELECT 1 FROM jsonb_array_elements_text(to_jsonb(%s)) _el WHERE _el ILIKE $%d)) OR CAST(%s AS TEXT) ILIKE $%d)", colExpr, colExpr, argIdx, colExpr, argIdx)
	default:
		return fmt.Sprintf("CAST(%s AS TEXT) %s $%d", colExpr, baseOp, argIdx)
	}
}

func buildScalarCompSQL(colExpr string, baseOp string, argIdx int) string {
	switch baseOp {
	case "~":
		return fmt.Sprintf("CAST(%s AS TEXT) ILIKE $%d", colExpr, argIdx)
	case "!~":
		return fmt.Sprintf("CAST(%s AS TEXT) NOT ILIKE $%d", colExpr, argIdx)
	default:
		return fmt.Sprintf("CAST(%s AS TEXT) %s $%d", colExpr, baseOp, argIdx)
	}
}

func isSensitiveFilterColumn(col string) bool {
	switch strings.ToLower(strings.TrimSpace(col)) {
	case "password_hash", "token_key", "password", "secret", "client_secret", "profile_api_key":
		return true
	default:
		return false
	}
}

func isInternalSystemTable(name string) bool {
	return strings.HasPrefix(strings.TrimSpace(name), "_")
}

func (p *filterParser) compileDynamicAuthSQL(authTok fToken, op string, valTok fToken) (string, error) {
	rest := authTok.val[len("@request.auth."):]
	parts := strings.Split(rest, ".")
	if len(parts) < 1 || len(parts) > 2 {
		return "", fmt.Errorf("unsupported auth relation path %q", authTok.val)
	}
	for _, seg := range parts {
		if !models.IsValidIdentifier(seg) {
			return "", fmt.Errorf("invalid identifier %q in %q", seg, authTok.val)
		}
		if isSensitiveFilterColumn(seg) {
			return "", fmt.Errorf("access to sensitive field %q is not allowed in filters", seg)
		}
	}
	if len(parts) == 2 && isInternalSystemTable(parts[0]) {
		return "", fmt.Errorf("access to system table %q is not allowed in filters", parts[0])
	}

	val, isNull, err := p.resolveValue(valTok)
	if err != nil {
		return "", err
	}
	authID := authUserIDStr(p.claims)
	if p.claims != nil && p.claims.Collection != "" && p.claims.Collection != "users" {
		return "FALSE", nil
	}
	if authID == "" {
		if isNegatedOp(op) {
			return "TRUE", nil
		}
		return "FALSE", nil
	}
	if isNull {
		if isNegatedOp(op) {
			return "TRUE", nil
		}
		return "FALSE", nil
	}

	baseOp, _ := stripAnyOp(op)
	strVal := fmt.Sprint(val)
	if baseOp == "~" || baseOp == "!~" {
		strVal = "%" + strVal + "%"
	}

	authIdx := p.addArg(authID)
	valIdx := p.addArg(strVal)

	// 1-part (@request.auth.user_type) or 2-part ending in .id (@request.auth.user_type.id)
	if len(parts) == 1 || strings.EqualFold(parts[1], "id") {
		colExpr := fmt.Sprintf(`_au."%s"`, parts[0])
		matchOp := baseOp
		neg := false
		if baseOp == "!=" {
			matchOp = "="
			neg = true
		} else if baseOp == "!~" {
			matchOp = "~"
			neg = true
		}
		cond := buildJSONBMatchSQL(colExpr, matchOp, valIdx)
		expr := fmt.Sprintf(`EXISTS (SELECT 1 FROM "users" _au WHERE CAST(_au."id" AS TEXT) = $%d AND %s)`, authIdx, cond)
		if neg {
			return "NOT " + expr, nil
		}
		return expr, nil
	}

	// 2-part (@request.auth.<rel>.<subField>, e.g. @request.auth.user_type.type ?= "admin")
	rel := parts[0]
	subField := parts[1]
	matchOp := baseOp
	neg := false
	if baseOp == "!=" {
		matchOp = "="
		neg = true
	} else if baseOp == "!~" {
		matchOp = "~"
		neg = true
	}
	subCond := buildScalarCompSQL(fmt.Sprintf(`_ar."%s"`, subField), matchOp, valIdx)
	expr := fmt.Sprintf(
		`EXISTS (SELECT 1 FROM "users" _au JOIN "%s" _ar ON ((jsonb_typeof(to_jsonb(_au."%s")) = 'array' AND to_jsonb(_au."%s") ? CAST(_ar."id" AS TEXT)) OR CAST(_au."%s" AS TEXT) = CAST(_ar."id" AS TEXT)) WHERE CAST(_au."id" AS TEXT) = $%d AND %s)`,
		rel, rel, rel, rel, authIdx, subCond,
	)
	if neg {
		return "NOT " + expr, nil
	}
	return expr, nil
}

func (p *filterParser) compileRelationPathSQL(relTok fToken, op string, valTok fToken) (string, error) {
	parts := strings.Split(relTok.val, ".")
	for _, seg := range parts[1:] {
		if isSensitiveFilterColumn(seg) {
			return "", fmt.Errorf("access to sensitive field %q is not allowed in filters", seg)
		}
	}
	relField := parts[0]
	fDef := p.allowed[relField]
	target1 := fDef.CollectionName
	if !models.IsValidIdentifier(target1) {
		target1 = relField
	}
	if isInternalSystemTable(target1) {
		return "", fmt.Errorf("access to system table %q is not allowed in filters", target1)
	}
	if len(parts) == 3 && isInternalSystemTable(parts[1]) {
		return "", fmt.Errorf("access to system table %q is not allowed in filters", parts[1])
	}

	val, isNull, err := p.resolveValue(valTok)
	if err != nil {
		return "", err
	}
	if isNull {
		if isNegatedOp(op) {
			return "TRUE", nil
		}
		return "FALSE", nil
	}

	baseOp, _ := stripAnyOp(op)
	strVal := fmt.Sprint(val)
	if valTok.kind == fTokIdent && strings.EqualFold(valTok.val, "@request.auth.id") && strVal == "" {
		if isNegatedOp(op) {
			return "TRUE", nil
		}
		return "FALSE", nil
	}
	if baseOp == "~" || baseOp == "!~" {
		strVal = "%" + strVal + "%"
	}
	valIdx := p.addArg(strVal)

	// Shortcut: rel.id ?= val -> match directly on current table's "rel" column
	if len(parts) == 2 && strings.EqualFold(parts[1], "id") {
		return buildJSONBMatchSQL(fmt.Sprintf(`"%s"`, relField), baseOp, valIdx), nil
	}

	matchOp := baseOp
	neg := false
	if baseOp == "!=" {
		matchOp = "="
		neg = true
	} else if baseOp == "!~" {
		matchOp = "~"
		neg = true
	}

	// Qualify outer relation column with outer table name when available so identically named columns
	// on the target table (e.g. instances.email -> users.email) never shadow the outer column.
	outerCol := fmt.Sprintf(`"%s"`, relField)
	if models.IsValidIdentifier(p.tableName) {
		outerCol = fmt.Sprintf(`"%s"."%s"`, p.tableName, relField)
	}

	// 1-level: rel.subField (e.g. user_type.type ?= "teachers", email.email = "alice@example.com")
	if len(parts) == 2 {
		subCond := buildScalarCompSQL(fmt.Sprintf(`_r1."%s"`, parts[1]), matchOp, valIdx)
		expr := fmt.Sprintf(
			`EXISTS (SELECT 1 FROM "%s" _r1 WHERE ((jsonb_typeof(to_jsonb(%s)) = 'array' AND to_jsonb(%s) ? CAST(_r1."id" AS TEXT)) OR CAST(%s AS TEXT) = CAST(_r1."id" AS TEXT)) AND %s)`,
			target1, outerCol, outerCol, outerCol, subCond,
		)
		if neg {
			return "NOT " + expr, nil
		}
		return expr, nil
	}

	// 2-level: rel1.rel2.id (e.g. instance.owners.id ?= @request.auth.id)
	if len(parts) == 3 && strings.EqualFold(parts[2], "id") {
		subCond := buildJSONBMatchSQL(fmt.Sprintf(`_r1."%s"`, parts[1]), matchOp, valIdx)
		expr := fmt.Sprintf(
			`EXISTS (SELECT 1 FROM "%s" _r1 WHERE ((jsonb_typeof(to_jsonb(%s)) = 'array' AND to_jsonb(%s) ? CAST(_r1."id" AS TEXT)) OR CAST(%s AS TEXT) = CAST(_r1."id" AS TEXT)) AND %s)`,
			target1, outerCol, outerCol, outerCol, subCond,
		)
		if neg {
			return "NOT " + expr, nil
		}
		return expr, nil
	}

	// 2-level: rel1.rel2.subField
	target2 := parts[1]
	subCond := buildScalarCompSQL(fmt.Sprintf(`_r2."%s"`, parts[2]), matchOp, valIdx)
	expr := fmt.Sprintf(
		`EXISTS (SELECT 1 FROM "%s" _r1 JOIN "%s" _r2 ON ((jsonb_typeof(to_jsonb(_r1."%s")) = 'array' AND to_jsonb(_r1."%s") ? CAST(_r2."id" AS TEXT)) OR CAST(_r1."%s" AS TEXT) = CAST(_r2."id" AS TEXT)) WHERE ((jsonb_typeof(to_jsonb("%s")) = 'array' AND to_jsonb("%s") ? CAST(_r1."id" AS TEXT)) OR CAST("%s" AS TEXT) = CAST(_r1."id" AS TEXT)) AND %s)`,
		target1, target2, parts[1], parts[1], parts[1], relField, relField, relField, subCond,
	)
	if neg {
		return "NOT " + expr, nil
	}
	return expr, nil
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

	// Special Case A: @request.body.<field>:changed = false/true
	if field, ok := isBodyChangedToken(lhs); ok {
		return p.compileBodyChangedSQL(field, op.val, rhs)
	}
	if field, ok := isBodyChangedToken(rhs); ok {
		return p.compileBodyChangedSQL(field, flipOperator(op.val), lhs)
	}

	// Special Case B: @request.auth.<customField> or @request.auth.<rel>.<subField>
	if isDynamicAuthPath(lhs) {
		return p.compileDynamicAuthSQL(lhs, op.val, rhs)
	}
	if isDynamicAuthPath(rhs) {
		return p.compileDynamicAuthSQL(rhs, flipOperator(op.val), lhs)
	}

	// Special Case C: Record relation path (e.g. owners.id ?= @request.auth.id, user_type.type ?= "teachers")
	if p.isRelationPath(lhs) {
		return p.compileRelationPathSQL(lhs, op.val, rhs)
	}
	if p.isRelationPath(rhs) {
		return p.compileRelationPathSQL(rhs, flipOperator(op.val), lhs)
	}

	// Special Case D: Column modifier :lower or :length (e.g. email:lower = 'a@b.com', tags:length > 0)
	if colName, mod, ok := p.parseColModifier(lhs); ok {
		return p.compileColModifierSQL(colName, mod, op.val, rhs)
	}
	if colName, mod, ok := p.parseColModifier(rhs); ok {
		return p.compileColModifierSQL(colName, mod, flipOperator(op.val), lhs)
	}

	lhsIsCol := p.isColumnIdent(lhs)
	rhsIsCol := p.isColumnIdent(rhs)

	// Case 1: Column vs Column (e.g., col1 != col2)
	if lhsIsCol && rhsIsCol {
		lSQL := fmt.Sprintf(`"%s"`, lhs.val)
		rSQL := fmt.Sprintf(`"%s"`, rhs.val)
		baseOp, _ := stripAnyOp(op.val)
		switch baseOp {
		case "~":
			return fmt.Sprintf("CAST(%s AS TEXT) ILIKE ('%%' || CAST(%s AS TEXT) || '%%')", lSQL, rSQL), nil
		case "!~":
			return fmt.Sprintf("CAST(%s AS TEXT) NOT ILIKE ('%%' || CAST(%s AS TEXT) || '%%')", lSQL, rSQL), nil
		default:
			return fmt.Sprintf("%s %s %s", lSQL, baseOp, rSQL), nil
		}
	}

	// Case 2: Both sides are static literals / @request.* values -> evaluate immediately
	if !lhsIsCol && !rhsIsCol {
		if lhs.kind == fTokIdent && !isLiteralOrRequestToken(lhs.val) && !p.createMode {
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

	// Case 3: One side is a column, the other is a value/literal/@request.*/@now
	colTok := lhs
	valTok := rhs
	effOp := op.val
	if !lhsIsCol && rhsIsCol {
		colTok = rhs
		valTok = lhs
		effOp = flipOperator(op.val)
	}

	colSQL := fmt.Sprintf(`"%s"`, colTok.val)
	baseOp, isAny := stripAnyOp(effOp)

	// Handle @now directly as SQL NOW() so TIMESTAMPTZ comparisons work cleanly.
	if valTok.kind == fTokIdent && strings.EqualFold(valTok.val, "@now") {
		return fmt.Sprintf("%s %s NOW()", colSQL, baseOp), nil
	}

	val, isNull, err := p.resolveValue(valTok)
	if err != nil {
		return "", err
	}

	if isNull {
		if baseOp == "=" {
			return colSQL + " IS NULL", nil
		}
		if baseOp == "!=" {
			return colSQL + " IS NOT NULL", nil
		}
		return "FALSE", nil
	}

	// PocketBase stores empty dates as ""; in PostgreSQL TIMESTAMPTZ columns, empty string causes 22007.
	if colDef, ok := p.allowed[colTok.val]; ok && (colDef.Type == "date" || colDef.Type == "autodate") {
		if s, isStr := val.(string); isStr && s == "" {
			if baseOp == "=" {
				return fmt.Sprintf("(%s IS NULL OR CAST(%s AS TEXT) = '')", colSQL, colSQL), nil
			}
			if baseOp == "!=" {
				return fmt.Sprintf("(%s IS NOT NULL AND CAST(%s AS TEXT) != '')", colSQL, colSQL), nil
			}
		}
	}

	// Any-of operator (?=, ?!=, ?~, ?!~) on a direct column (e.g. multi-relation JSONB or scalar)
	if isAny {
		strVal := fmt.Sprint(val)
		if valTok.kind == fTokIdent && strings.EqualFold(valTok.val, "@request.auth.id") && strVal == "" {
			if isNegatedOp(effOp) {
				return "TRUE", nil
			}
			return "FALSE", nil
		}
		if baseOp == "~" || baseOp == "!~" {
			strVal = "%" + strVal + "%"
		}
		idx := p.addArg(strVal)
		return buildJSONBMatchSQL(colSQL, baseOp, idx), nil
	}

	// When comparing against @request.auth.id, safely compare as TEXT so BIGINT vs TEXT never fails.
	if valTok.kind == fTokIdent && strings.EqualFold(valTok.val, "@request.auth.id") && (baseOp == "=" || baseOp == "!=") {
		strVal := fmt.Sprint(val)
		if strVal == "" {
			if baseOp == "=" {
				return "FALSE", nil
			}
			return "TRUE", nil
		}
		idx := p.addArg(strVal)
		return fmt.Sprintf("CAST(%s AS TEXT) %s $%d", colSQL, baseOp, idx), nil
	}

	switch baseOp {
	case "~":
		idx := p.addArg("%" + fmt.Sprint(val) + "%")
		return fmt.Sprintf("CAST(%s AS TEXT) ILIKE $%d", colSQL, idx), nil
	case "!~":
		idx := p.addArg("%" + fmt.Sprint(val) + "%")
		return fmt.Sprintf("CAST(%s AS TEXT) NOT ILIKE $%d", colSQL, idx), nil
	default:
		idx := p.addArg(val)
		return fmt.Sprintf("%s %s $%d", colSQL, baseOp, idx), nil
	}
}

func (p *filterParser) parseColModifier(tok fToken) (string, string, bool) {
	if tok.kind != fTokIdent || p.createMode {
		return "", "", false
	}
	idx := strings.LastIndexByte(tok.val, ':')
	if idx <= 0 {
		return "", "", false
	}
	col := tok.val[:idx]
	mod := strings.ToLower(tok.val[idx+1:])
	if mod != "lower" && mod != "length" {
		return "", "", false
	}
	if !p.isColumnIdent(fToken{kind: fTokIdent, val: col}) {
		return "", "", false
	}
	return col, mod, true
}

func (p *filterParser) compileColModifierSQL(col, mod, op string, valTok fToken) (string, error) {
	val, isNull, err := p.resolveValue(valTok)
	if err != nil || isNull {
		return "FALSE", err
	}
	baseOp, _ := stripAnyOp(op)
	if mod == "lower" {
		s := strings.ToLower(fmt.Sprint(val))
		if baseOp == "~" || baseOp == "!~" {
			s = "%" + s + "%"
			idx := p.addArg(s)
			if baseOp == "!~" {
				return fmt.Sprintf(`LOWER(CAST("%s" AS TEXT)) NOT LIKE $%d`, col, idx), nil
			}
			return fmt.Sprintf(`LOWER(CAST("%s" AS TEXT)) LIKE $%d`, col, idx), nil
		}
		idx := p.addArg(s)
		return fmt.Sprintf(`LOWER(CAST("%s" AS TEXT)) %s $%d`, col, baseOp, idx), nil
	}
	// mod == "length"
	idx := p.addArg(val)
	lenExpr := fmt.Sprintf(`(CASE WHEN "%s" IS NULL THEN 0 WHEN jsonb_typeof(to_jsonb("%s")) = 'array' THEN jsonb_array_length(to_jsonb("%s")) ELSE char_length(CAST("%s" AS TEXT)) END)`, col, col, col, col)
	return fmt.Sprintf("%s %s $%d", lenExpr, baseOp, idx), nil
}

func (p *filterParser) compileBodyChangedSQL(field string, op string, valTok fToken) (string, error) {
	rhsVal, _, err := p.resolveValue(valTok)
	if err != nil {
		return "", err
	}
	wantChanged := strings.EqualFold(fmt.Sprint(rhsVal), "true")
	if isNegatedOp(op) {
		wantChanged = !wantChanged
	}
	// If field is not present in incoming body, it is definitely NOT changed.
	if p.body == nil {
		if wantChanged {
			return "FALSE", nil
		}
		return "TRUE", nil
	}
	incoming, present := p.body[field]
	if !present {
		if wantChanged {
			return "FALSE", nil
		}
		return "TRUE", nil
	}
	if p.allowed == nil || !models.IsValidIdentifier(field) {
		if wantChanged {
			return "TRUE", nil
		}
		return "FALSE", nil
	}
	idx := p.addArg(fmt.Sprint(incoming))
	if wantChanged {
		return fmt.Sprintf(`CAST("%s" AS TEXT) IS DISTINCT FROM $%d`, field, idx), nil
	}
	return fmt.Sprintf(`CAST("%s" AS TEXT) IS NOT DISTINCT FROM $%d`, field, idx), nil
}

func isLiteralOrRequestToken(val string) bool {
	lower := strings.ToLower(val)
	return lower == "true" || lower == "false" || lower == "null" || lower == "@now" || strings.HasPrefix(lower, "@request.")
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
		case lower == "@now":
			return time.Now().UTC().Format(time.RFC3339), false, nil
		case lower == "@request.auth.id":
			return authUserIDStr(p.claims), false, nil
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
		case lower == "@request.auth.collectionname":
			if p.claims == nil {
				return "", false, nil
			}
			if p.claims.Role == "superuser" {
				return "_superusers", false, nil
			}
			if p.claims.Collection != "" {
				return p.claims.Collection, false, nil
			}
			return "users", false, nil
		case strings.HasPrefix(lower, "@request.body.") && strings.HasSuffix(lower, ":isset"):
			field := tok.val[len("@request.body.") : len(tok.val)-len(":isset")]
			if p.body == nil {
				return false, false, nil
			}
			_, ok := p.body[field]
			return ok, false, nil
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
			if p.createMode && p.body != nil {
				if v, ok := p.body[tok.val]; ok {
					if v == nil {
						return nil, true, nil
					}
					return v, false, nil
				}
				return "", false, nil
			}
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
		case lower == "@now":
			return time.Now().UTC().Format(time.RFC3339), false
		case lower == "@request.auth.id":
			return authUserIDStr(e.claims), false
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
		case lower == "@request.auth.collectionname":
			if e.claims == nil {
				return "", false
			}
			if e.claims.Role == "superuser" {
				return "_superusers", false
			}
			if e.claims.Collection != "" {
				return e.claims.Collection, false
			}
			return "users", false
		case strings.HasPrefix(lower, "@request.body.") && strings.HasSuffix(lower, ":isset"):
			field := tok.val[len("@request.body.") : len(tok.val)-len(":isset")]
			if e.data == nil {
				return false, false
			}
			_, ok := e.data[field]
			return ok, false
		case strings.HasPrefix(lower, "@request.body.") && strings.HasSuffix(lower, ":changed"):
			field := tok.val[len("@request.body.") : len(tok.val)-len(":changed")]
			if e.data == nil {
				return false, false
			}
			_, ok := e.data[field]
			return ok, false
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
	baseOp, isAny := stripAnyOp(op)
	if isAny {
		if arr, ok := lhs.([]any); ok {
			for _, item := range arr {
				if evalComparison(item, item == nil, baseOp, rhs, rNull) {
					return true
				}
			}
			return false
		}
		if arr, ok := lhs.([]string); ok {
			for _, item := range arr {
				if evalComparison(item, false, baseOp, rhs, rNull) {
					return true
				}
			}
			return false
		}
	}
	if lNull || rNull {
		switch baseOp {
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
	switch baseOp {
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
				switch baseOp {
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
		switch baseOp {
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
