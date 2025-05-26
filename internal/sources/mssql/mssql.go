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

package mssql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/genai-toolbox/internal/sources"
	_ "github.com/microsoft/go-mssqldb"
	"go.opentelemetry.io/otel/trace"
)

const Kind string = "mssql"

// validate interface
var _ sources.SourceConfig = Config{}

func init() {
	if !sources.Register(Kind, newConfig) {
		panic(fmt.Sprintf("source kind %q already registered", Kind))
	}
}

func newConfig(ctx context.Context, name string, decoder *yaml.Decoder) (sources.SourceConfig, error) {
	actual := Config{Name: name}
	if err := decoder.DecodeContext(ctx, &actual); err != nil {
		return nil, fmt.Errorf("unable to parse %q config: %w", Kind, err)
	}
	return actual, nil
}

type Config struct {
	// Cloud SQL MSSQL configs
	Name     string `yaml:"name" validate:"required"`
	Kind     string `yaml:"kind" validate:"required"`
	Host     string `yaml:"host" validate:"required"`
	Port     string `yaml:"port" validate:"required"`
	User     string `yaml:"user" validate:"required"`
	Password string `yaml:"password" validate:"required"`
	Database string `yaml:"database" validate:"required"`
}

func (r Config) SourceConfigKind() string {
	// Returns Cloud SQL MSSQL source kind
	return Kind
}

func (r Config) Initialize(ctx context.Context, tracer trace.Tracer) (sources.Source, error) {
	// Initializes a MSSQL source
	db, err := initMssqlConnection(ctx, tracer, r.Name, r.Host, r.Port, r.User, r.Password, r.Database)
	if err != nil {
		return nil, fmt.Errorf("unable to create db connection: %w", err)
	}

	// Verify db connection
	err = db.PingContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to connect successfully: %w", err)
	}

	s := &Source{
		Name: r.Name,
		Kind: Kind,
		Db:   db,
	}
	return s, nil
}

var _ sources.Source = &Source{}

type Source struct {
	// Cloud SQL MSSQL struct with connection pool
	Name string `yaml:"name"`
	Kind string `yaml:"kind"`
	Db   *sql.DB
}

func (s *Source) SourceKind() string {
	// Returns Cloud SQL MSSQL source kind
	return Kind
}

func (s *Source) MSSQLDB() *sql.DB {
	// Returns a Cloud SQL MSSQL database connection pool
	return s.Db
}

func initMssqlConnection(ctx context.Context, tracer trace.Tracer, name, host, port, user, pass, dbname string) (*sql.DB, error) {
	//nolint:all // Reassigned ctx
	ctx, span := sources.InitConnectionSpan(ctx, tracer, Kind, name)
	defer span.End()

	// Create dsn
	dsn := fmt.Sprintf("sqlserver://%s:%s@%s:%s?database=%s", user, pass, host, port, dbname)

	// Open database connection
	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	return db, nil
}
