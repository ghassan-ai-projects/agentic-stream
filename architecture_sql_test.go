package agenticstream

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"strings"
	"testing"
)

type sqlMutation struct {
	operation, table, query string
	columns                 []string
}
type sqlLexeme struct {
	text   string
	quoted bool
}

// sqlLexemes removes SQL values and comments, retaining identifiers and
// punctuation. Quoted identifiers cannot masquerade as SQL verbs.
func sqlLexemes(query string) []sqlLexeme {
	var result []sqlLexeme
	for i := 0; i < len(query); {
		c := query[i]
		switch {
		case c == '\'' || c == '"' || c == '`' || c == '[':
			end := c
			if c == '[' {
				end = ']'
			}
			i++
			var value strings.Builder
			for i < len(query) {
				if query[i] == end {
					i++
					if end != ']' && i < len(query) && query[i] == end {
						value.WriteByte(end)
						i++
						continue
					}
					break
				}
				value.WriteByte(query[i])
				i++
			}
			if c != '\'' {
				result = append(result, sqlLexeme{text: strings.ToLower(value.String()), quoted: true})
			}
		case c == '-' && i+1 < len(query) && query[i+1] == '-':
			i += 2
			for i < len(query) && query[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(query) && query[i+1] == '*':
			i += 2
			for i+1 < len(query) && (query[i] != '*' || query[i+1] != '/') {
				i++
			}
			i = min(i+2, len(query))
		case sqlIdentifierByte(c):
			start := i
			for i < len(query) && sqlIdentifierByte(query[i]) {
				i++
			}
			result = append(result, sqlLexeme{text: strings.ToLower(query[start:i])})
		case c == ' ' || c == '\n' || c == '\r' || c == '\t':
			i++
		default:
			result = append(result, sqlLexeme{text: string(c)})
			i++
		}
	}
	return result
}

func sqlIdentifierByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_'
}

func sqlMutations(query string) []sqlMutation {
	tokens := sqlLexemes(query)
	var result []sqlMutation
	for i, token := range tokens {
		if token.quoted {
			continue
		}
		operation := token.text
		j := i + 1
		switch operation {
		case "insert", "replace":
			if sqlKeyword(tokens, j, "or") {
				j += 2
			}
			if !sqlKeyword(tokens, j, "into") {
				continue
			}
			j++
			operation = "insert"
		case "update":
			if sqlKeyword(tokens, j, "or") {
				j += 2
			}
		case "delete":
			if !sqlKeyword(tokens, j, "from") {
				continue
			}
			j++
		default:
			continue
		}
		if j >= len(tokens) {
			result = append(result, sqlMutation{operation: operation})
			continue
		}
		table := tokens[j].text
		j++
		if j < len(tokens) && tokens[j].text == "." {
			j++
			if j < len(tokens) {
				table = tokens[j].text
				j++
			}
		}
		if operation == "update" {
			// DO UPDATE SET is the insert's conflict clause, not an independent writer.
			if table == "set" {
				continue
			}
			if sqlKeyword(tokens, j, "as") {
				j += 2
			}
			if !sqlKeyword(tokens, j, "set") {
				continue
			}
		}
		mutation := sqlMutation{operation: operation, table: table, query: query}
		if operation == "update" {
			mutation.columns = sqlUpdatedColumns(tokens, j+1)
		}
		result = append(result, mutation)
	}
	return result
}

func sqlKeyword(tokens []sqlLexeme, index int, word string) bool {
	return index < len(tokens) && !tokens[index].quoted && tokens[index].text == word
}

func sqlUpdatedColumns(tokens []sqlLexeme, index int) []string {
	var columns []string
	depth := 0
	expectColumn := true
	for i := index; i < len(tokens); i++ {
		token := tokens[i]
		if depth == 0 && (sqlKeyword(tokens, i, "where") || sqlKeyword(tokens, i, "returning") || token.text == ";") {
			break
		}
		if expectColumn && i+1 < len(tokens) && tokens[i+1].text == "=" {
			columns = append(columns, token.text)
			expectColumn = false
		}
		switch token.text {
		case "(":
			depth++
		case ")":
			depth--
		case ",":
			if depth == 0 {
				expectColumn = true
			}
		}
	}
	return columns
}

func productionSQLMutations(t *testing.T, visit func(string, string, sqlMutation)) {
	t.Helper()
	root := repoRoot(t)
	for _, file := range productionGoFiles(t, root) {
		positions := token.NewFileSet()
		parsed, err := parser.ParseFile(positions, file.abs, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			query, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatal(err)
			}
			for _, mutation := range sqlMutations(query) {
				visit(file.rel, fmt.Sprintf("%s:%d", file.rel, positions.Position(literal.Pos()).Line), mutation)
			}
			return true
		})
	}
}

func TestSQLMutationClassifierRecognizesOwnershipBoundaries(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		query, table, operation string
		columns                 []string
	}{
		{`WITH pending AS (SELECT 1) UPDATE "main"."episodes" SET lifecycle_status = ?, ended_at = COALESCE(?, ended_at) WHERE episode_id = ?`, "episodes", "update", []string{"lifecycle_status", "ended_at"}},
		{`INSERT OR IGNORE INTO [approvals](approval_id) VALUES ('UPDATE episodes SET x=1') ON CONFLICT DO UPDATE SET status='pending'`, "approvals", "insert", nil},
		{"-- UPDATE episodes SET x=1\n/* DELETE FROM commands */ DELETE FROM `shadow_decisions` WHERE episode_id=?", "shadow_decisions", "delete", nil},
		{`REPLACE INTO main.commands(command_id) VALUES (?)`, "commands", "insert", nil},
	} {
		t.Run(tc.table+tc.operation, func(t *testing.T) {
			got := sqlMutations(tc.query)
			if len(got) != 1 || got[0].table != tc.table || got[0].operation != tc.operation || !slices.Equal(got[0].columns, tc.columns) {
				t.Fatalf("classify %s: %+v", tc.query, got)
			}
		})
	}
	for _, query := range []string{`SELECT 'DELETE FROM episodes', 'INSERT INTO commands', 'UPDATE approvals SET x=1'`, `SELECT "update" FROM episodes`, `update episode terminal: %w`} {
		if got := sqlMutations(query); len(got) != 0 {
			t.Fatalf("read/text classified as mutation: %s: %+v", query, got)
		}
	}
}
