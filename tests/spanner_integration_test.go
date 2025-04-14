//go:build integration && spanner

// Copyright 2024 Google LLC
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

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	database "cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/google/uuid"
)

var (
	SPANNER_SOURCE_KIND = "spanner"
	SPANNER_TOOL_KIND   = "spanner-sql"
	SPANNER_PROJECT     = os.Getenv("SPANNER_PROJECT")
	SPANNER_DATABASE    = os.Getenv("SPANNER_DATABASE")
	SPANNER_INSTANCE    = os.Getenv("SPANNER_INSTANCE")
)

func getSpannerVars(t *testing.T) map[string]any {
	switch "" {
	case SPANNER_PROJECT:
		t.Fatal("'SPANNER_PROJECT' not set")
	case SPANNER_DATABASE:
		t.Fatal("'SPANNER_DATABASE' not set")
	case SPANNER_INSTANCE:
		t.Fatal("'SPANNER_INSTANCE' not set")
	}

	return map[string]any{
		"kind":     SPANNER_SOURCE_KIND,
		"project":  SPANNER_PROJECT,
		"instance": SPANNER_INSTANCE,
		"database": SPANNER_DATABASE,
	}
}

func initSpannerClients(ctx context.Context, project, instance, dbname string) (*spanner.Client, *database.DatabaseAdminClient, error) {
	// Configure the connection to the database
	db := fmt.Sprintf("projects/%s/instances/%s/databases/%s", project, instance, dbname)

	// Configure session pool to automatically clean inactive transactions
	sessionPoolConfig := spanner.SessionPoolConfig{
		TrackSessionHandles: true,
		InactiveTransactionRemovalOptions: spanner.InactiveTransactionRemovalOptions{
			ActionOnInactiveTransaction: spanner.WarnAndClose,
		},
	}

	// Create Spanner client (for queries)
	dataClient, err := spanner.NewClientWithConfig(context.Background(), db, spanner.ClientConfig{SessionPoolConfig: sessionPoolConfig})
	if err != nil {
		return nil, nil, fmt.Errorf("unable to create new Spanner client: %w", err)
	}

	// Create Spanner admin client (for creating databases)
	adminClient, err := database.NewDatabaseAdminClient(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to create new Spanner admin client: %w", err)
	}

	return dataClient, adminClient, nil
}

func TestSpannerToolEndpoints(t *testing.T) {
	sourceConfig := getSpannerVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	var args []string

	// Create Spanner client
	dataClient, adminClient, err := initSpannerClients(ctx, SPANNER_PROJECT, SPANNER_INSTANCE, SPANNER_DATABASE)
	if err != nil {
		t.Fatalf("unable to create Spanner client: %s", err)
	}

	// create table name with UUID
	tableNameParam := "param_table_" + strings.Replace(uuid.New().String(), "-", "", -1)
	tableNameAuth := "auth_table_" + strings.Replace(uuid.New().String(), "-", "", -1)

	// set up data for param tool
	create_statement1, insert_statement1, tool_statement1, params1 := GetSpannerParamToolInfo(tableNameParam)
	dbString := fmt.Sprintf(
		"projects/%s/instances/%s/databases/%s",
		SPANNER_PROJECT,
		SPANNER_INSTANCE,
		SPANNER_DATABASE,
	)
	teardownTable1 := SetupSpannerTable(t, ctx, adminClient, dataClient, create_statement1, insert_statement1, tableNameParam, dbString, params1)
	defer teardownTable1(t)

	// set up data for auth tool
	create_statement2, insert_statement2, tool_statement2, params2 := GetSpannerAuthToolInfo(tableNameAuth)
	teardownTable2 := SetupSpannerTable(t, ctx, adminClient, dataClient, create_statement2, insert_statement2, tableNameAuth, dbString, params2)
	defer teardownTable2(t)

	// Write config into a file and pass it to command
	toolsFile := GetToolsConfig(sourceConfig, SPANNER_TOOL_KIND, tool_statement1, tool_statement2)

	cmd, cleanup, err := StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := cmd.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`))
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	RunToolGetTest(t)

	select_1_want := "[{\"\":\"1\"}]"
	RunToolInvokeTest(t, select_1_want)
}

// TestSpannerWriteTransaction test Spanner Tool invocation with write statement
func TestSpannerWriteStatement(t *testing.T) {
	// Spanner needs an extra test for Tool invocation with write transactions
	// Because it calls separate APIs with read/write Tool statements
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	var args []string

	_, adminClient, err := initSpannerClients(ctx, SPANNER_PROJECT, SPANNER_INSTANCE, SPANNER_DATABASE)
	if err != nil {
		t.Fatalf("unable to create Spanner client: %s", err)
	}

	sourceConfig := getSpannerVars(t)
	tableName := "test_write_table" + strings.Replace(uuid.New().String(), "-", "", -1)

	// Write config into a file and pass it to command
	toolStmt := fmt.Sprintf("INSERT INTO %s (Id, Name) VALUES (1, 'Alice');", tableName)
	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-instance": sourceConfig,
		},
		"tools": map[string]any{
			"write-tool": map[string]any{
				"kind":        SPANNER_TOOL_KIND,
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
				"statement":   toolStmt,
			},
		},
	}

	cmd, cleanup, err := StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := cmd.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`))
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	// Create test table
	dbString := fmt.Sprintf(
		"projects/%s/instances/%s/databases/%s",
		SPANNER_PROJECT,
		SPANNER_INSTANCE,
		SPANNER_DATABASE,
	)

	stmt := fmt.Sprintf("CREATE TABLE %s (Id INT64 NOT NULL, Name STRING(255) NOT NULL) PRIMARY KEY (Id)", tableName)

	op, err := adminClient.UpdateDatabaseDdl(ctx, &databasepb.UpdateDatabaseDdlRequest{
		Database:   dbString,
		Statements: []string{stmt},
	})
	if err != nil {
		t.Fatalf("unable to start create table operation %s: %s", tableName, err)
	}
	err = op.Wait(ctx)
	if err != nil {
		t.Fatalf("unable to create test table %s: %s", tableName, err)
	}

	// Tear down table after test complete
	teardownTable := func(t *testing.T) {
		// tear down test
		op, err = adminClient.UpdateDatabaseDdl(ctx, &databasepb.UpdateDatabaseDdlRequest{
			Database:   dbString,
			Statements: []string{fmt.Sprintf("DROP TABLE %s", tableName)},
		})
		if err != nil {
			t.Errorf("unable to start drop table operation: %s", err)
			return
		}

		err = op.Wait(ctx)
		if err != nil {
			t.Errorf("Teardown failed: %s", err)
		}
	}

	defer teardownTable(t)

	invokeTcs := []struct {
		name          string
		api           string
		requestHeader map[string]string
		requestBody   io.Reader
		want          string
		isErr         bool
	}{
		{
			name:          "invoke my-simple-tool",
			api:           "http://127.0.0.1:5000/api/tool/write-tool/invoke",
			requestHeader: map[string]string{},
			requestBody:   bytes.NewBuffer([]byte(`{}`)),
			want:          "[1 row(s) updated]",
			isErr:         false,
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {

			// Send Tool invocation request
			req, err := http.NewRequest(http.MethodPost, tc.api, tc.requestBody)
			if err != nil {
				t.Fatalf("unable to create request: %s", err)
			}
			req.Header.Add("Content-type", "application/json")
			for k, v := range tc.requestHeader {
				req.Header.Add(k, v)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("unable to send request: %s", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				if tc.isErr == true {
					return
				}
				bodyBytes, _ := io.ReadAll(resp.Body)
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(bodyBytes))
			}

			// Check response body
			var body map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&body)
			if err != nil {
				t.Fatalf("error parsing response body")
			}

			got, ok := body["result"].(string)
			if !ok {
				t.Fatalf("unable to find result in response body")
			}
			// Remove `\` and `"` for string comparison
			got = strings.ReplaceAll(got, "\\", "")
			want := strings.ReplaceAll(tc.want, "\\", "")
			got = strings.ReplaceAll(got, "\"", "")
			want = strings.ReplaceAll(want, "\"", "")
			if got != want {
				t.Fatalf("unexpected value: got %q, want %q", got, tc.want)
			}
		})
	}
}
