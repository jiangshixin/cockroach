// Copyright 2016 The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.
//
// Author: Matt Jibson (mjibson@gmail.com)

package sql_test

import (
	"strings"
	"testing"

	"github.com/cockroachdb/cockroach/pkg/keys"
	"github.com/cockroachdb/cockroach/pkg/roachpb"
	"github.com/cockroachdb/cockroach/pkg/server"
	"github.com/cockroachdb/cockroach/pkg/testutils/serverutils"
	"github.com/cockroachdb/cockroach/pkg/testutils/sqlutils"
	"github.com/cockroachdb/cockroach/pkg/util/leaktest"
)

func TestSplitAt(t *testing.T) {
	defer leaktest.AfterTest(t)()

	params, _ := createTestServerParams()
	s, db, _ := serverutils.StartServer(t, params)
	defer s.Stopper().Stop()

	r := sqlutils.MakeSQLRunner(t, db)

	r.Exec("CREATE DATABASE d")
	r.Exec(`CREATE TABLE d.t (
		i INT,
		s STRING,
		PRIMARY KEY (i, s),
		INDEX s_idx (s)
	)`)
	r.Exec(`CREATE TABLE d.i (k INT PRIMARY KEY)`)

	tests := []struct {
		in    string
		error string
		args  []interface{}
	}{
		{
			in: "ALTER TABLE d.t SPLIT AT VALUES (2, 'b')",
		},
		{
			// Splitting at an existing split is a silent no-op.
			in: "ALTER TABLE d.t SPLIT AT VALUES (2, 'b')",
		},
		{
			in: "ALTER TABLE d.t SPLIT AT VALUES (3, 'c'), (4, 'd')",
		},
		{
			in: "ALTER TABLE d.t SPLIT AT SELECT 5, 'd'",
		},
		{
			in: "ALTER TABLE d.t SPLIT AT SELECT * FROM (VALUES (6, 'e'), (7, 'f')) AS a",
		},
		{
			in: "ALTER TABLE d.t SPLIT AT VALUES (10)",
		},
		{
			in:    "ALTER TABLE d.t SPLIT AT VALUES ('c', 3)",
			error: "SPLIT AT data column 1 must be of type int, not type string",
		},
		{
			in:    "ALTER TABLE d.t SPLIT AT VALUES (i, s)",
			error: `name "i" is not defined`,
		},
		{
			in: "ALTER INDEX d.t@s_idx SPLIT AT VALUES ('f')",
		},
		{
			in:    "ALTER INDEX d.t@not_present SPLIT AT VALUES ('g')",
			error: `index "not_present" does not exist`,
		},
		{
			in:    "ALTER TABLE d.i SPLIT AT VALUES (avg(1))",
			error: "aggregate functions are not allowed in VALUES",
		},
		{
			in:   "ALTER TABLE d.i SPLIT AT VALUES ($1)",
			args: []interface{}{8},
		},
		{
			in:    "ALTER TABLE d.i SPLIT AT VALUES ($1)",
			error: "no value provided for placeholder: $1",
		},
		{
			in:    "ALTER TABLE d.i SPLIT AT VALUES ($1)",
			args:  []interface{}{"blah"},
			error: "error in argument for $1: strconv.ParseInt",
		},
		{
			in:    "ALTER TABLE d.i SPLIT AT VALUES ($1::string)",
			args:  []interface{}{"1"},
			error: "SPLIT AT data column 1 must be of type int, not type string",
		},
		{
			in: "ALTER TABLE d.i SPLIT AT VALUES ((SELECT 1))",
		},
	}

	for _, tt := range tests {
		var key roachpb.Key
		var pretty string
		err := db.QueryRow(tt.in, tt.args...).Scan(&key, &pretty)
		if err != nil && tt.error == "" {
			t.Fatalf("%s: unexpected error: %s", tt.in, err)
		} else if tt.error != "" && err == nil {
			t.Fatalf("%s: expected error: %s", tt.in, tt.error)
		} else if err != nil && tt.error != "" {
			if !strings.Contains(err.Error(), tt.error) {
				t.Fatalf("%s: unexpected error: %s", tt.in, err)
			}
		} else {
			// Successful split, verify it happened.
			rng, err := s.(*server.TestServer).LookupRange(key)
			if err != nil {
				t.Fatal(err)
			}
			expect := roachpb.Key(keys.MakeRowSentinelKey(rng.StartKey))
			if !expect.Equal(key) {
				t.Fatalf("%s: expected range start %s, got %s", tt.in, pretty, expect)
			}
		}
	}
}
