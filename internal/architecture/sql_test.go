package architecture

import (
	"go/ast"
	"go/token"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// sqlStatement matches a string literal that is a SQL statement.
var sqlStatement = regexp.MustCompile(`(?i)^\s*(SELECT|INSERT(?:\s+OR\s+\w+)?\s+INTO|UPDATE|DELETE|REPLACE|WITH)\s`)

// stringLiterals lists the decoded string literals of a production file.
func stringLiterals(file goFile) []string {
	var literals []string
	ast.Inspect(file.syntax, func(node ast.Node) bool {
		if literal, ok := node.(*ast.BasicLit); ok && literal.Kind == token.STRING {
			if value, err := strconv.Unquote(literal.Value); err == nil {
				literals = append(literals, value)
			}
		}
		return true
	})
	return literals
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	return line
}

type sqlMutation struct {
	operation, table, query string
	columns                 []string
	rewritesExisting        bool
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
			if operation == "replace" && i > 0 && sqlKeyword(tokens, i-1, "or") {
				continue
			}
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
		if operation == "insert" {
			mutation.rewritesExisting = insertRewritesExisting(tokens, i)
		}
		if operation == "update" {
			mutation.columns = sqlUpdatedColumns(tokens, j+1)
		}
		result = append(result, mutation)
	}
	return result
}

func insertRewritesExisting(tokens []sqlLexeme, index int) bool {
	if sqlKeyword(tokens, index, "replace") || sqlKeyword(tokens, index+1, "or") && sqlKeyword(tokens, index+2, "replace") {
		return true
	}
	for i := index + 1; i+1 < len(tokens) && tokens[i].text != ";"; i++ {
		if sqlKeyword(tokens, i, "do") && sqlKeyword(tokens, i+1, "update") {
			return true
		}
	}
	return false
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
		if expectColumn && token.text == "(" {
			// Tuple assignments are deliberately unsupported: reject the whole
			// handoff mutation instead of silently missing its payload columns.
			return nil
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
			t.Parallel()

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

func TestInsertConflictClassifierDistinguishesIgnoreFromRewrite(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		query    string
		rewrites bool
	}{
		{`INSERT INTO commands(command_id) VALUES (?)`, false},
		{`INSERT OR IGNORE INTO commands(command_id) VALUES (?)`, false},
		{`INSERT INTO commands(command_id) VALUES ('DO UPDATE SET') ON CONFLICT DO NOTHING`, false},
		{`INSERT INTO commands(command_id) VALUES (?) ON CONFLICT DO UPDATE SET command_json=?`, true},
		{`INSERT OR REPLACE INTO main.commands(command_id) VALUES (?)`, true},
		{`REPLACE INTO "commands"(command_id) VALUES (?)`, true},
	} {
		mutations := sqlMutations(tc.query)
		if len(mutations) != 1 || mutations[0].rewritesExisting != tc.rewrites {
			t.Fatalf("conflict rewrite classification: %s: %+v", tc.query, mutations)
		}
		if allowed := ownsMutation("internal/policy/internal/store", mutations[0]); allowed == tc.rewrites {
			t.Fatalf("prepared command creation permission: %s: allowed=%v", tc.query, allowed)
		}
	}
}

// TestSQLGateDistinguishesInsertStatementsFromErrorMessages preserves error text.
func TestSQLGateDistinguishesInsertStatementsFromErrorMessages(t *testing.T) {
	t.Parallel()

	for _, query := range []string{"INSERT INTO items(id) VALUES (?)", "INSERT OR IGNORE INTO items(id) VALUES (?)", "insert or replace into items(id) values (?)"} {
		if !sqlStatement.MatchString(query) {
			t.Errorf("SQL missed: %s", query)
		}
	}
	if sqlStatement.MatchString("insert item: %w") || sqlStatement.MatchString("insert reconsideration item: %w") {
		t.Fatal("error prefix classified as executable SQL")
	}
}
