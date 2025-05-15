// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package tests contains end to end tests meant to verify the Toolbox Server
// works as expected when executed as a binary.

package tests

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/googleapis/genai-toolbox/internal/tools"
	"github.com/jackc/pgx/v5/pgxpool"
)

// GetToolsConfig returns a mock tools config file
func GetToolsConfig(sourceConfig map[string]any, toolKind, param_tool_statement, auth_tool_statement string) map[string]any {
	// Write config into a file and pass it to command
	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-instance": sourceConfig,
		},
		"authServices": map[string]any{
			"my-google-auth": map[string]any{
				"kind":     "google",
				"clientId": ClientId,
			},
		},
		"tools": map[string]any{
			"my-simple-tool": map[string]any{
				"kind":        toolKind,
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
				"statement":   "SELECT 1;",
			},
			"my-param-tool": map[string]any{
				"kind":        toolKind,
				"source":      "my-instance",
				"description": "Tool to test invocation with params.",
				"statement":   param_tool_statement,
				"parameters": []any{
					map[string]any{
						"name":        "id",
						"type":        "integer",
						"description": "user ID",
					},
					map[string]any{
						"name":        "name",
						"type":        "string",
						"description": "user name",
					},
				},
			},
			"my-auth-tool": map[string]any{
				"kind":        toolKind,
				"source":      "my-instance",
				"description": "Tool to test authenticated parameters.",
				// statement to auto-fill authenticated parameter
				"statement": auth_tool_statement,
				"parameters": []map[string]any{
					{
						"name":        "email",
						"type":        "string",
						"description": "user email",
						"authServices": []map[string]string{
							{
								"name":  "my-google-auth",
								"field": "email",
							},
						},
					},
				},
			},
			"my-auth-required-tool": map[string]any{
				"kind":        toolKind,
				"source":      "my-instance",
				"description": "Tool to test auth required invocation.",
				"statement":   "SELECT 1;",
				"authRequired": []string{
					"my-google-auth",
				},
			},
			"my-fail-tool": map[string]any{
				"kind":        toolKind,
				"source":      "my-instance",
				"description": "Tool to test statement with incorrect syntax.",
				"statement":   "SELEC 1;",
			},
		},
	}

	return toolsFile
}

// AddPgExecuteSqlConfig gets the tools config for `postgres-execute-sql`
func AddPgExecuteSqlConfig(t *testing.T, config map[string]any) map[string]any {
	tools, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}
	tools["my-exec-sql-tool"] = map[string]any{
		"kind":        "postgres-execute-sql",
		"source":      "my-instance",
		"description": "Tool to execute sql",
	}
	tools["my-auth-exec-sql-tool"] = map[string]any{
		"kind":        "postgres-execute-sql",
		"source":      "my-instance",
		"description": "Tool to execute sql",
		"authRequired": []string{
			"my-google-auth",
		},
	}
	config["tools"] = tools
	return config
}

// GetHTTPToolsConfig returns a mock HTTP tool's config file
func GetHTTPToolsConfig(sourceConfig map[string]any, toolKind string) map[string]any {
	// Write config into a file and pass it to command
	otherSourceConfig := make(map[string]any)
	for k, v := range sourceConfig {
		otherSourceConfig[k] = v
	}
	otherSourceConfig["headers"] = map[string]string{"X-Custom-Header": "unexpected", "Content-Type": "application/json"}
	otherSourceConfig["queryParams"] = map[string]any{"id": 1, "name": "Sid"}

	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-instance":    sourceConfig,
			"other-instance": otherSourceConfig,
		},
		"authServices": map[string]any{
			"my-google-auth": map[string]any{
				"kind":     "google",
				"clientId": ClientId,
			},
		},
		"tools": map[string]any{
			"my-simple-tool": map[string]any{
				"kind":        toolKind,
				"path":        "/tool0",
				"method":      "POST",
				"source":      "my-instance",
				"requestBody": "{}",
				"description": "Simple tool to test end to end functionality.",
			},
			"my-param-tool": map[string]any{
				"kind":        toolKind,
				"source":      "my-instance",
				"method":      "GET",
				"path":        "/tool1",
				"description": "some description",
				"queryParams": []tools.Parameter{
					tools.NewIntParameter("id", "user ID")},
				"requestBody": `{
"age": 36,
"name": "{{.name}}"
}
`,
				"bodyParams": []tools.Parameter{tools.NewStringParameter("name", "user name")},
				"headers":    map[string]string{"Content-Type": "application/json"},
			},
			"my-auth-tool": map[string]any{
				"kind":        toolKind,
				"source":      "my-instance",
				"method":      "GET",
				"path":        "/tool2",
				"description": "some description",
				"requestBody": "{}",
				"queryParams": []tools.Parameter{
					tools.NewStringParameterWithAuth("email", "some description",
						[]tools.ParamAuthService{{Name: "my-google-auth", Field: "email"}}),
				},
			},
			"my-auth-required-tool": map[string]any{
				"kind":         toolKind,
				"source":       "my-instance",
				"method":       "POST",
				"path":         "/tool0",
				"description":  "some description",
				"requestBody":  "{}",
				"authRequired": []string{"my-google-auth"},
			},
			"my-advanced-tool": map[string]any{
				"kind":        toolKind,
				"source":      "other-instance",
				"method":      "get",
				"path":        "/tool3?id=2",
				"description": "some description",
				"headers": map[string]string{
					"X-Custom-Header": "example",
				},
				"queryParams": []tools.Parameter{
					tools.NewIntParameter("id", "user ID"), tools.NewStringParameter("country", "country")},
				"requestBody": `{
"place": "zoo",
"animals": {{json .animalArray }}
}
`,
				"bodyParams":   []tools.Parameter{tools.NewArrayParameter("animalArray", "animals in the zoo", tools.NewStringParameter("animals", "desc"))},
				"headerParams": []tools.Parameter{tools.NewStringParameter("X-Other-Header", "custom header")},
			},
		},
	}
	return toolsFile
}

// GetPostgresSQLParamToolInfo returns statements and param for my-param-tool postgres-sql kind
func GetPostgresSQLParamToolInfo(tableName string) (string, string, string, []any) {
	create_statement := fmt.Sprintf("CREATE TABLE %s (id SERIAL PRIMARY KEY, name TEXT);", tableName)
	insert_statement := fmt.Sprintf("INSERT INTO %s (name) VALUES ($1), ($2), ($3);", tableName)
	tool_statement := fmt.Sprintf("SELECT * FROM %s WHERE id = $1 OR name = $2;", tableName)
	params := []any{"Alice", "Jane", "Sid"}
	return create_statement, insert_statement, tool_statement, params
}

// GetPostgresSQLAuthToolInfo returns statements and param of my-auth-tool for postgres-sql kind
func GetPostgresSQLAuthToolInfo(tableName string) (string, string, string, []any) {
	create_statement := fmt.Sprintf("CREATE TABLE %s (id SERIAL PRIMARY KEY, name TEXT, email TEXT);", tableName)
	insert_statement := fmt.Sprintf("INSERT INTO %s (name, email) VALUES ($1, $2), ($3, $4)", tableName)
	tool_statement := fmt.Sprintf("SELECT name FROM %s WHERE email = $1;", tableName)
	params := []any{"Alice", SERVICE_ACCOUNT_EMAIL, "Jane", "janedoe@gmail.com"}
	return create_statement, insert_statement, tool_statement, params
}

// GetMssqlParamToolInfo returns statements and param for my-param-tool mssql-sql kind
func GetMssqlParamToolInfo(tableName string) (string, string, string, []any) {
	create_statement := fmt.Sprintf("CREATE TABLE %s (id INT IDENTITY(1,1) PRIMARY KEY, name VARCHAR(255));", tableName)
	insert_statement := fmt.Sprintf("INSERT INTO %s (name) VALUES (@alice), (@jane), (@sid);", tableName)
	tool_statement := fmt.Sprintf("SELECT * FROM %s WHERE id = @id OR name = @p2;", tableName)
	params := []any{sql.Named("alice", "Alice"), sql.Named("jane", "Jane"), sql.Named("sid", "Sid")}
	return create_statement, insert_statement, tool_statement, params
}

// GetMssqlLAuthToolInfo returns statements and param of my-auth-tool for mssql-sql kind
func GetMssqlLAuthToolInfo(tableName string) (string, string, string, []any) {
	create_statement := fmt.Sprintf("CREATE TABLE %s (id INT IDENTITY(1,1) PRIMARY KEY, name VARCHAR(255), email VARCHAR(255));", tableName)
	insert_statement := fmt.Sprintf("INSERT INTO %s (name, email) VALUES (@alice, @aliceemail), (@jane, @janeemail);", tableName)
	tool_statement := fmt.Sprintf("SELECT name FROM %s WHERE email = @email;", tableName)
	params := []any{sql.Named("alice", "Alice"), sql.Named("aliceemail", SERVICE_ACCOUNT_EMAIL), sql.Named("jane", "Jane"), sql.Named("janeemail", "janedoe@gmail.com")}
	return create_statement, insert_statement, tool_statement, params
}

// GetMysqlParamToolInfo returns statements and param for my-param-tool mssql-sql kind
func GetMysqlParamToolInfo(tableName string) (string, string, string, []any) {
	create_statement := fmt.Sprintf("CREATE TABLE %s (id INT NOT NULL AUTO_INCREMENT PRIMARY KEY, name VARCHAR(255));", tableName)
	insert_statement := fmt.Sprintf("INSERT INTO %s (name) VALUES (?), (?), (?);", tableName)
	tool_statement := fmt.Sprintf("SELECT * FROM %s WHERE id = ? OR name = ?;", tableName)
	params := []any{"Alice", "Jane", "Sid"}
	return create_statement, insert_statement, tool_statement, params
}

// GetMysqlLAuthToolInfo returns statements and param of my-auth-tool for mssql-sql kind
func GetMysqlLAuthToolInfo(tableName string) (string, string, string, []any) {
	create_statement := fmt.Sprintf("CREATE TABLE %s (id INT NOT NULL AUTO_INCREMENT PRIMARY KEY, name VARCHAR(255), email VARCHAR(255));", tableName)
	insert_statement := fmt.Sprintf("INSERT INTO %s (name, email) VALUES (?, ?), (?, ?)", tableName)
	tool_statement := fmt.Sprintf("SELECT name FROM %s WHERE email = ?;", tableName)
	params := []any{"Alice", SERVICE_ACCOUNT_EMAIL, "Jane", "janedoe@gmail.com"}
	return create_statement, insert_statement, tool_statement, params
}

func GetNonSpannerInvokeParamWant() (string, string, string) {
	invokeParamWant := "[{\"id\":1,\"name\":\"Alice\"},{\"id\":3,\"name\":\"Sid\"}]"
	mcpInvokeParamWant := `{"jsonrpc":"2.0","id":"my-param-tool","result":{"content":[{"type":"text","text":"{\"id\":1,\"name\":\"Alice\"}"},{"type":"text","text":"{\"id\":3,\"name\":\"Sid\"}"}]}}`
	invokeAuthWant := `[{\"name\":\"Alice\"}]`
	return invokeParamWant, invokeAuthWant, mcpInvokeParamWant
}

// GetPostgresWants return the expected wants for postgres
func GetPostgresWants() (string, string) {
	select1Want := "[{\"?column?\":1}]"
	failInvocationWant := `{"jsonrpc":"2.0","id":"invoke-fail-tool","result":{"content":[{"type":"text","text":"unable to execute query: ERROR: syntax error at or near \"SELEC\" (SQLSTATE 42601)"}],"isError":true}}`
	return select1Want, failInvocationWant
}

// GetMssqlWants return the expected wants for mssql
func GetMssqlWants() (string, string) {
	select1Want := "[{\"\":1}]"
	failInvocationWant := `{"jsonrpc":"2.0","id":"invoke-fail-tool","result":{"content":[{"type":"text","text":"unable to execute query: mssql: Could not find stored procedure 'SELEC'."}],"isError":true}}`
	return select1Want, failInvocationWant
}

// GetMysqlWants return the expected wants for mysql
func GetMysqlWants() (string, string) {
	select1Want := "[{\"1\":1}]"
	failInvocationWant := `{"jsonrpc":"2.0","id":"invoke-fail-tool","result":{"content":[{"type":"text","text":"unable to execute query: Error 1064 (42000): You have an error in your SQL syntax; check the manual that corresponds to your MySQL server version for the right syntax to use near 'SELEC 1' at line 1"}],"isError":true}}`
	return select1Want, failInvocationWant
}

// SetupPostgresSQLTable creates and inserts data into a table of tool
// compatible with postgres-sql tool
func SetupPostgresSQLTable(t *testing.T, ctx context.Context, pool *pgxpool.Pool, create_statement, insert_statement, tableName string, params []any) func(*testing.T) {
	err := pool.Ping(ctx)
	if err != nil {
		t.Fatalf("unable to connect to test database: %s", err)
	}

	// Create table
	_, err = pool.Query(ctx, create_statement)
	if err != nil {
		t.Fatalf("unable to create test table %s: %s", tableName, err)
	}

	// Insert test data
	_, err = pool.Query(ctx, insert_statement, params...)
	if err != nil {
		t.Fatalf("unable to insert test data: %s", err)
	}

	return func(t *testing.T) {
		// tear down test
		_, err = pool.Exec(ctx, fmt.Sprintf("DROP TABLE %s;", tableName))
		if err != nil {
			t.Errorf("Teardown failed: %s", err)
		}
	}
}

// SetupMsSQLTable creates and inserts data into a table of tool
// compatible with mssql-sql tool
func SetupMsSQLTable(t *testing.T, ctx context.Context, pool *sql.DB, create_statement, insert_statement, tableName string, params []any) func(*testing.T) {
	err := pool.PingContext(ctx)
	if err != nil {
		t.Fatalf("unable to connect to test database: %s", err)
	}

	// Create table
	_, err = pool.QueryContext(ctx, create_statement)
	if err != nil {
		t.Fatalf("unable to create test table %s: %s", tableName, err)
	}

	// Insert test data
	_, err = pool.QueryContext(ctx, insert_statement, params...)
	if err != nil {
		t.Fatalf("unable to insert test data: %s", err)
	}

	return func(t *testing.T) {
		// tear down test
		_, err = pool.ExecContext(ctx, fmt.Sprintf("DROP TABLE %s;", tableName))
		if err != nil {
			t.Errorf("Teardown failed: %s", err)
		}
	}
}

// SetupMySQLTable creates and inserts data into a table of tool
// compatible with mssql-sql tool
func SetupMySQLTable(t *testing.T, ctx context.Context, pool *sql.DB, create_statement, insert_statement, tableName string, params []any) func(*testing.T) {
	err := pool.PingContext(ctx)
	if err != nil {
		t.Fatalf("unable to connect to test database: %s", err)
	}

	// Create table
	_, err = pool.QueryContext(ctx, create_statement)
	if err != nil {
		t.Fatalf("unable to create test table %s: %s", tableName, err)
	}

	// Insert test data
	_, err = pool.QueryContext(ctx, insert_statement, params...)
	if err != nil {
		t.Fatalf("unable to insert test data: %s", err)
	}

	return func(t *testing.T) {
		// tear down test
		_, err = pool.ExecContext(ctx, fmt.Sprintf("DROP TABLE %s;", tableName))
		if err != nil {
			t.Errorf("Teardown failed: %s", err)
		}
	}
}

// GetRedisWants return the expected wants for redis
func GetRedisValkeyWants() (string, string, string, string, string) {
	select1Want := "[\"PONG\"]"
	failInvocationWant := `{"jsonrpc":"2.0","id":"invoke-fail-tool","result":{"content":[{"type":"text","text":"\"error from executing command at index 0: ERR unknown command 'SELEC 1;', with args beginning with:  command length: 1\""}]}}`
	invokeParamWant := "[\"Alice\",\"Sid\"]"
	invokeAuthWant := `["{\"name\":\"Alice\"}"]`
	mcpInvokeParamWant := `{"jsonrpc":"2.0","id":"my-param-tool","result":{"content":[{"type":"text","text":"\"Alice\""},{"type":"text","text":"\"Sid\""}]}}`
	return select1Want, failInvocationWant, invokeParamWant, invokeAuthWant, mcpInvokeParamWant
}

// GetPostgresSQLParamToolInfo returns statements and param for my-param-tool postgres-sql kind
func GetRedisValkeyToolCmds() ([][]string, [][]string) {
	paramCmd := [][]string{{"HGET", "row1", "name"}, {"HGET", "row3", "name"}}
	authCmd := [][]string{{"HGET", "$email", "name"}}
	return paramCmd, authCmd
}

func GetRedisValkeyToolsConfig(sourceConfig map[string]any, toolKind string, param_cmds, auth_cmds [][]string) map[string]any {
	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-instance": sourceConfig,
		},
		"authServices": map[string]any{
			"my-google-auth": map[string]any{
				"kind":     "google",
				"clientId": ClientId,
			},
		},
		"tools": map[string]any{
			"my-simple-tool": map[string]any{
				"kind":        toolKind,
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
				"commands":    [][]string{{"PING"}},
			},
			"my-param-tool": map[string]any{
				"kind":        toolKind,
				"source":      "my-instance",
				"description": "Tool to test invocation with params.",
				"commands":    param_cmds,
				"parameters": []any{
					map[string]any{
						"name":        "id",
						"type":        "integer",
						"description": "user ID",
					},
					map[string]any{
						"name":        "name",
						"type":        "string",
						"description": "user name",
					},
				},
			},
			"my-auth-tool": map[string]any{
				"kind":        toolKind,
				"source":      "my-instance",
				"description": "Tool to test authenticated parameters.",
				// statement to auto-fill authenticated parameter
				"commands": auth_cmds,
				"parameters": []map[string]any{
					{
						"name":        "email",
						"type":        "string",
						"description": "user email",
						"authServices": []map[string]string{
							{
								"name":  "my-google-auth",
								"field": "email",
							},
						},
					},
				},
			},
			"my-auth-required-tool": map[string]any{
				"kind":        toolKind,
				"source":      "my-instance",
				"description": "Tool to test auth required invocation.",
				"commands":    [][]string{{"PING"}},
				"authRequired": []string{
					"my-google-auth",
				},
			},
			"my-fail-tool": map[string]any{
				"kind":        toolKind,
				"source":      "my-instance",
				"description": "Tool to test statement with incorrect syntax.",
				"commands":    [][]string{{"SELEC 1;"}},
			},
		},
	}

	return toolsFile

}
