// SPDX-License-Identifier: Apache-2.0

package state_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"github.com/xataio/pgroll/pkg/migrations"
	"github.com/xataio/pgroll/pkg/schema"
	"github.com/xataio/pgroll/pkg/state"
	"github.com/xataio/pgroll/pkg/testutils"
)

func TestMain(m *testing.M) {
	testutils.SharedTestMain(m)
}

func TestSchemaOptionIsRespected(t *testing.T) {
	t.Parallel()

	testutils.WithStateAndConnectionToContainer(t, func(state *state.State, db *sql.DB) {
		ctx := context.Background()

		// create a table in the public schema
		if _, err := db.ExecContext(ctx, "CREATE TABLE public.table1 (id int)"); err != nil {
			t.Fatal(err)
		}

		// init the state
		if err := state.Init(ctx); err != nil {
			t.Fatal(err)
		}

		// check that starting a new migration returns the already existing table
		currentSchema, err := state.Start(ctx, "public", &migrations.Migration{
			Name: "1_add_column",
			Operations: migrations.Operations{
				&migrations.OpAddColumn{
					Table: "table1",
					Column: migrations.Column{
						Name: "test",
						Type: "text",
					},
				},
			},
		})
		assert.NoError(t, err)

		assert.Equal(t, 1, len(currentSchema.Tables))
		assert.Equal(t, "public", currentSchema.Name)
	})
}

func TestReadSchema(t *testing.T) {
	t.Parallel()

	testutils.WithStateAndConnectionToContainer(t, func(state *state.State, db *sql.DB) {
		ctx := context.Background()

		tests := []struct {
			name       string
			createStmt string
			wantSchema *schema.Schema
		}{
			{
				name:       "one table",
				createStmt: "CREATE TABLE public.table1 (id int)",
				wantSchema: &schema.Schema{
					Name: "public",
					Tables: map[string]schema.Table{
						"table1": {
							Name: "table1",
							Columns: map[string]schema.Column{
								"id": {
									Name:     "id",
									Type:     "integer",
									Nullable: true,
								},
							},
						},
					},
				},
			},
			{
				name:       "unique, not null",
				createStmt: "CREATE TABLE public.table1 (id int NOT NULL, CONSTRAINT id_unique UNIQUE(id))",
				wantSchema: &schema.Schema{
					Name: "public",
					Tables: map[string]schema.Table{
						"table1": {
							Name: "table1",
							Columns: map[string]schema.Column{
								"id": {
									Name:     "id",
									Type:     "integer",
									Nullable: false,
									Unique:   true,
								},
							},
							Indexes: map[string]schema.Index{
								"id_unique": {
									Name: "id_unique",
								},
							},
							UniqueConstraints: map[string]schema.UniqueConstraint{
								"id_unique": {
									Name:    "id_unique",
									Columns: []string{"id"},
								},
							},
						},
					},
				},
			},
			{
				name:       "foreign key",
				createStmt: "CREATE TABLE public.table1 (id int PRIMARY KEY); CREATE TABLE public.table2 (fk int NOT NULL, CONSTRAINT fk_fkey FOREIGN KEY (fk) REFERENCES public.table1 (id))",
				wantSchema: &schema.Schema{
					Name: "public",
					Tables: map[string]schema.Table{
						"table1": {
							Name: "table1",
							Columns: map[string]schema.Column{
								"id": {
									Name:     "id",
									Type:     "integer",
									Nullable: false,
									Unique:   true,
								},
							},
							PrimaryKey: []string{"id"},
							Indexes: map[string]schema.Index{
								"table1_pkey": {
									Name: "table1_pkey",
								},
							},
						},
						"table2": {
							Name: "table2",
							Columns: map[string]schema.Column{
								"fk": {
									Name:     "fk",
									Type:     "integer",
									Nullable: false,
								},
							},
							ForeignKeys: map[string]schema.ForeignKey{
								"fk_fkey": {
									Name:              "fk_fkey",
									Columns:           []string{"fk"},
									ReferencedTable:   "table1",
									ReferencedColumns: []string{"id"},
								},
							},
						},
					},
				},
			},
			{
				name:       "check constraint",
				createStmt: "CREATE TABLE public.table1 (id int PRIMARY KEY, age INTEGER, CONSTRAINT age_check CHECK (age > 18));",
				wantSchema: &schema.Schema{
					Name: "public",
					Tables: map[string]schema.Table{
						"table1": {
							Name: "table1",
							Columns: map[string]schema.Column{
								"id": {
									Name:     "id",
									Type:     "integer",
									Nullable: false,
									Unique:   true,
								},
								"age": {
									Name:     "age",
									Type:     "integer",
									Nullable: true,
								},
							},
							PrimaryKey: []string{"id"},
							Indexes: map[string]schema.Index{
								"table1_pkey": {
									Name: "table1_pkey",
								},
							},
							CheckConstraints: map[string]schema.CheckConstraint{
								"age_check": {
									Name:       "age_check",
									Columns:    []string{"age"},
									Definition: "CHECK ((age > 18))",
								},
							},
						},
					},
				},
			},
			{
				name:       "unique constraint",
				createStmt: "CREATE TABLE public.table1 (id int PRIMARY KEY, name TEXT, CONSTRAINT name_unique UNIQUE(name) );",
				wantSchema: &schema.Schema{
					Name: "public",
					Tables: map[string]schema.Table{
						"table1": {
							Name: "table1",
							Columns: map[string]schema.Column{
								"id": {
									Name:     "id",
									Type:     "integer",
									Nullable: false,
									Unique:   true,
								},
								"name": {
									Name:     "name",
									Type:     "text",
									Unique:   true,
									Nullable: true,
								},
							},
							PrimaryKey: []string{"id"},
							Indexes: map[string]schema.Index{
								"table1_pkey": {
									Name: "table1_pkey",
								},
								"name_unique": {
									Name: "name_unique",
								},
							},
							UniqueConstraints: map[string]schema.UniqueConstraint{
								"name_unique": {
									Name:    "name_unique",
									Columns: []string{"name"},
								},
							},
						},
					},
				},
			},
		}

		// init the state
		if err := state.Init(ctx); err != nil {
			t.Fatal(err)
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if _, err := db.ExecContext(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public"); err != nil {
					t.Fatal(err)
				}

				if _, err := db.ExecContext(ctx, tt.createStmt); err != nil {
					t.Fatal(err)
				}

				gotSchema, err := state.ReadSchema(ctx, "public")
				if err != nil {
					t.Fatal(err)
				}
				if diff := cmp.Diff(tt.wantSchema, gotSchema, cmpopts.IgnoreFields(schema.Table{}, "OID")); diff != "" {
					t.Errorf("expected schema mismatch (-want +got):\n%s", diff)
				}
			})
		}
	})
}
