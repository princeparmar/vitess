/*
Copyright 2019 The Vitess Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package vtgate

import (
	"context"
	"fmt"
	"testing"

	"vitess.io/vitess/go/test/utils"

	"github.com/google/go-cmp/cmp"

	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"

	"vitess.io/vitess/go/mysql"
	"vitess.io/vitess/go/sqltypes"
	"vitess.io/vitess/go/test/endtoend/cluster"
)

func TestDMLScatter(t *testing.T) {
	ctx := context.Background()
	conn, err := mysql.Connect(ctx, &vtParams)
	require.NoError(t, err)
	defer conn.Close()

	/* Simple insert. after this dml, the tables will contain the following:
	t3 (id5, id6, id7):
	1 2 3
	2 2 3
	3 4 3
	4 5 4

	t3_id7_idx (id7, keyspace_id:id6):
	3 2
	3 2
	3 4
	4 5
	*/
	exec(t, conn, "begin")
	exec(t, conn, "insert into t3(id5, id6, id7) values(1, 2, 3), (2, 2, 3), (3, 4, 3), (4, 5, 4)")
	exec(t, conn, "commit")
	qr := exec(t, conn, "select id5, id6, id7 from t3 order by id5")
	if got, want := fmt.Sprintf("%v", qr.Rows), "[[INT64(1) INT64(2) INT64(3)] [INT64(2) INT64(2) INT64(3)] [INT64(3) INT64(4) INT64(3)] [INT64(4) INT64(5) INT64(4)]]"; got != want {
		t.Errorf("select:\n%v want\n%v", got, want)
	}

	/* Updating a non lookup column. after this dml, the tables will contain the following:
	t3 (id5, id6, id7):
	42 2 3
	2 2 3
	3 4 3
	4 5 4

	t3_id7_idx (id7, keyspace_id:id6):
	3 2
	3 2
	3 4
	4 5
	*/
	exec(t, conn, "update t3 set id5 = 42 where id5 = 1")
	qr = exec(t, conn, "select id5, id6, id7 from t3 order by id5")
	if got, want := fmt.Sprintf("%v", qr.Rows), "[[INT64(2) INT64(2) INT64(3)] [INT64(3) INT64(4) INT64(3)] [INT64(4) INT64(5) INT64(4)] [INT64(42) INT64(2) INT64(3)]]"; got != want {
		t.Errorf("select:\n%v want\n%v", got, want)
	}

	/* Updating a lookup column. after this dml, the tables will contain the following:
	t3 (id5, id6, id7):
	42 2 42
	2 2 42
	3 4 3
	4 5 4

	t3_id7_idx (id7, keyspace_id:id6):
	42 2
	42 2
	3 4
	4 5
	*/
	exec(t, conn, "begin")
	exec(t, conn, "update t3 set id7 = 42 where id6 = 2")
	exec(t, conn, "commit")
	qr = exec(t, conn, "select id5, id6, id7 from t3 order by id5")
	if got, want := fmt.Sprintf("%v", qr.Rows), "[[INT64(2) INT64(2) INT64(42)] [INT64(3) INT64(4) INT64(3)] [INT64(4) INT64(5) INT64(4)] [INT64(42) INT64(2) INT64(42)]]"; got != want {
		t.Errorf("select:\n%v want\n%v", got, want)
	}

	/* delete one specific keyspace id. after this dml, the tables will contain the following:
	t3 (id5, id6, id7):
	3 4 3
	4 5 4

	t3_id7_idx (id7, keyspace_id:id6):
	3 4
	4 5
	*/
	exec(t, conn, "delete from t3 where id6 = 2")
	qr = exec(t, conn, "select * from t3 where id6 = 2")
	require.Empty(t, qr.Rows)
	qr = exec(t, conn, "select * from t3_id7_idx where id6 = 2")
	require.Empty(t, qr.Rows)

	// delete all the rows.
	exec(t, conn, "delete from t3")
	qr = exec(t, conn, "select * from t3")
	require.Empty(t, qr.Rows)
	qr = exec(t, conn, "select * from t3_id7_idx")
	require.Empty(t, qr.Rows)
}

func TestDMLIn(t *testing.T) {
	ctx := context.Background()
	conn, err := mysql.Connect(ctx, &vtParams)
	require.NoError(t, err)
	defer conn.Close()

	/* Simple insert. after this dml, the tables will contain the following:
	t3 (id5, id6, id7):
	1 2 3
	2 2 3
	3 4 3
	4 5 4

	t3_id7_idx (id7, keyspace_id:id6):
	3 2
	3 2
	3 4
	4 5
	*/
	exec(t, conn, "begin")
	exec(t, conn, "insert into t3(id5, id6, id7) values(1, 2, 3), (2, 2, 3), (3, 4, 3), (4, 5, 4)")
	exec(t, conn, "commit")
	qr := exec(t, conn, "select id5, id6, id7 from t3 order by id5, id6")
	if got, want := fmt.Sprintf("%v", qr.Rows), "[[INT64(1) INT64(2) INT64(3)] [INT64(2) INT64(2) INT64(3)] [INT64(3) INT64(4) INT64(3)] [INT64(4) INT64(5) INT64(4)]]"; got != want {
		t.Errorf("select:\n%v want\n%v", got, want)
	}

	/* Updating a non lookup column. after this dml, the tables will contain the following:
	t3 (id5, id6, id7):
	1 2 3
	2 2 3
	42 4 3
	42 5 4

	t3_id7_idx (id7, keyspace_id:id6):
	3 2
	3 2
	3 4
	4 5
	*/
	exec(t, conn, "update t3 set id5 = 42 where id6 in (4, 5)")
	qr = exec(t, conn, "select id5, id6, id7 from t3 order by id5, id6")
	if got, want := fmt.Sprintf("%v", qr.Rows), "[[INT64(1) INT64(2) INT64(3)] [INT64(2) INT64(2) INT64(3)] [INT64(42) INT64(4) INT64(3)] [INT64(42) INT64(5) INT64(4)]]"; got != want {
		t.Errorf("select:\n%v want\n%v", got, want)
	}

	/* Updating a non lookup column. after this dml, the tables will contain the following:
	t3 (id5, id6, id7):
	1 2 42
	2 2 42
	42 4 3
	42 5 4

	t3_id7_idx (id7, keyspace_id:id6):
	42 2
	42 2
	3 4
	42 5
	*/
	exec(t, conn, "begin")
	exec(t, conn, "update t3 set id7 = 42 where id6 in (2, 5)")
	exec(t, conn, "commit")
	qr = exec(t, conn, "select id5, id6, id7 from t3 order by id5, id6")
	if got, want := fmt.Sprintf("%v", qr.Rows), "[[INT64(1) INT64(2) INT64(42)] [INT64(2) INT64(2) INT64(42)] [INT64(42) INT64(4) INT64(3)] [INT64(42) INT64(5) INT64(42)]]"; got != want {
		t.Errorf("select:\n%v want\n%v", got, want)
	}

	/* Updating a non lookup column. after this dml, the tables will contain the following:
	t3 (id5, id6, id7):
	42 4 3
	42 5 4

	t3_id7_idx (id7, keyspace_id:id6):
	3 4
	42 5
	*/
	exec(t, conn, "delete from t3 where id6 in (2)")
	qr = exec(t, conn, "select * from t3 where id6 = 2")
	require.Empty(t, qr.Rows)
	qr = exec(t, conn, "select * from t3_id7_idx where id6 = 2")
	require.Empty(t, qr.Rows)

	// delete all the rows.
	exec(t, conn, "delete from t3 where id6 in (4, 5)")
	qr = exec(t, conn, "select * from t3")
	require.Empty(t, qr.Rows)
	qr = exec(t, conn, "select * from t3_id7_idx")
	require.Empty(t, qr.Rows)
}

func TestSelectNull(t *testing.T) {
	ctx := context.Background()
	conn, err := mysql.Connect(ctx, &vtParams)
	require.NoError(t, err)
	defer conn.Close()

	exec(t, conn, "begin")
	exec(t, conn, "insert into t5_null_vindex(id, idx) values(1, 'a'), (2, 'b'), (3, null)")
	exec(t, conn, "commit")

	assertMatches(t, conn, "select id, idx from t5_null_vindex order by id", "[[INT64(1) VARCHAR(\"a\")] [INT64(2) VARCHAR(\"b\")] [INT64(3) NULL]]")
	assertIsEmpty(t, conn, "select id, idx from t5_null_vindex where idx = null")
	assertMatches(t, conn, "select id, idx from t5_null_vindex where idx is null", "[[INT64(3) NULL]]")
	assertMatches(t, conn, "select id, idx from t5_null_vindex where idx is not null order by id", "[[INT64(1) VARCHAR(\"a\")] [INT64(2) VARCHAR(\"b\")]]")
	assertIsEmpty(t, conn, "select id, idx from t5_null_vindex where id IN (null)")
	assertMatches(t, conn, "select id, idx from t5_null_vindex where id IN (1,2,null) order by id", "[[INT64(1) VARCHAR(\"a\")] [INT64(2) VARCHAR(\"b\")]]")
	assertIsEmpty(t, conn, "select id, idx from t5_null_vindex where id NOT IN (1,null) order by id")
	assertMatches(t, conn, "select id, idx from t5_null_vindex where id NOT IN (1,3)", "[[INT64(2) VARCHAR(\"b\")]]")

	exec(t, conn, "delete from t5_null_vindex")
}

func TestDoStatement(t *testing.T) {
	ctx := context.Background()
	conn, err := mysql.Connect(ctx, &vtParams)
	require.NoError(t, err)
	defer conn.Close()

	exec(t, conn, "do 1")
	exec(t, conn, "do 'a', 1+2,database()")
}

func TestShowColumns(t *testing.T) {
	conn, err := mysql.Connect(context.Background(), &vtParams)
	require.NoError(t, err)
	defer conn.Close()

	expected := `[[VARCHAR("id") TEXT("bigint(20)") VARCHAR("NO") VARCHAR("PRI") NULL VARCHAR("")] [VARCHAR("idx") TEXT("varchar(50)") VARCHAR("YES") VARCHAR("") NULL VARCHAR("")]]`
	assertMatches(t, conn, "show columns from `t5_null_vindex` in `ks`", expected)
	assertMatches(t, conn, "SHOW COLUMNS from `t5_null_vindex` in `ks`", expected)
	assertMatches(t, conn, "SHOW columns FROM `t5_null_vindex` in `ks`", expected)
}

func TestCastConvert(t *testing.T) {
	conn, err := mysql.Connect(context.Background(), &vtParams)
	require.NoError(t, err)
	defer conn.Close()

	assertMatches(t, conn, `SELECT CAST("test" AS CHAR(60))`, `[[VARCHAR("test")]]`)
}

func TestUnionAll(t *testing.T) {
	conn, err := mysql.Connect(context.Background(), &vtParams)
	require.NoError(t, err)
	defer conn.Close()

	exec(t, conn, "delete from t1")
	exec(t, conn, "delete from t2")

	exec(t, conn, "insert into t1(id1, id2) values(1, 1), (2, 2)")
	exec(t, conn, "insert into t2(id3, id4) values(3, 3), (4, 4)")

	// union all between two selectuniqueequal
	assertMatches(t, conn, "select id1 from t1 where id1 = 1 union all select id1 from t1 where id1 = 4", "[[INT64(1)]]")

	// union all between two different tables
	assertMatches(t, conn, "(select id1,id2 from t1 order by id1) union all (select id3,id4 from t2 order by id3)",
		"[[INT64(1) INT64(1)] [INT64(2) INT64(2)] [INT64(3) INT64(3)] [INT64(4) INT64(4)]]")

	// union all between two different tables
	assertMatches(t, conn, "select tbl2.id1 FROM  ((select id1 from t1 order by id1 limit 5) union all (select id1 from t1 order by id1 desc limit 5)) as tbl1 INNER JOIN t1 as tbl2  ON tbl1.id1 = tbl2.id1",
		"[[INT64(1)] [INT64(2)] [INT64(2)] [INT64(1)]]")

	exec(t, conn, "insert into t1(id1, id2) values(3, 3), (4, 4), (5, 5), (6, 6), (7, 7), (8, 8)")

	// union all between two selectuniquein tables
	qr := exec(t, conn, "select id1 from t1 where id1 in (1, 2, 3, 4, 5, 6, 7, 8) union all select id1 from t1 where id1 in (1, 2, 3, 4, 5, 6, 7, 8)")
	expected := utils.SortString("[[INT64(1)] [INT64(2)] [INT64(3)] [INT64(5)] [INT64(4)] [INT64(6)] [INT64(7)] [INT64(8)] [INT64(1)] [INT64(2)] [INT64(3)] [INT64(5)] [INT64(4)] [INT64(6)] [INT64(7)] [INT64(8)]]")
	assert.Equal(t, expected, utils.SortString(fmt.Sprintf("%v", qr.Rows)))

	// clean up
	exec(t, conn, "delete from t1")
	exec(t, conn, "delete from t2")
}

func TestUnion(t *testing.T) {
	conn, err := mysql.Connect(context.Background(), &vtParams)
	require.NoError(t, err)
	defer conn.Close()

	assertMatches(t, conn, `SELECT 1 UNION SELECT 1 UNION SELECT 1`, `[[INT64(1)]]`)
	assertMatches(t, conn, `SELECT 1,'a' UNION SELECT 1,'a' UNION SELECT 1,'a' ORDER BY 1`, `[[INT64(1) VARCHAR("a")]]`)
	assertMatches(t, conn, `SELECT 1,'z' UNION SELECT 2,'q' UNION SELECT 3,'b' ORDER BY 2`, `[[INT64(3) VARCHAR("b")] [INT64(2) VARCHAR("q")] [INT64(1) VARCHAR("z")]]`)
	assertMatches(t, conn, `SELECT 1,'a' UNION ALL SELECT 1,'a' UNION ALL SELECT 1,'a' ORDER BY 1`, `[[INT64(1) VARCHAR("a")] [INT64(1) VARCHAR("a")] [INT64(1) VARCHAR("a")]]`)
	assertMatches(t, conn, `(SELECT 1,'a') UNION ALL (SELECT 1,'a') UNION ALL (SELECT 1,'a') ORDER BY 1`, `[[INT64(1) VARCHAR("a")] [INT64(1) VARCHAR("a")] [INT64(1) VARCHAR("a")]]`)
	assertMatches(t, conn, `(SELECT 1,'a') ORDER BY 1`, `[[INT64(1) VARCHAR("a")]]`)
	assertMatches(t, conn, `(SELECT 1,'a' order by 1) union (SELECT 1,'a' ORDER BY 1)`, `[[INT64(1) VARCHAR("a")]]`)
}

func TestSavepointInTx(t *testing.T) {
	defer cluster.PanicHandler(t)
	ctx := context.Background()
	conn, err := mysql.Connect(ctx, &vtParams)
	require.Nil(t, err)
	defer conn.Close()

	exec(t, conn, "savepoint a")
	exec(t, conn, "start transaction")
	exec(t, conn, "savepoint b")
	exec(t, conn, "rollback to b")
	exec(t, conn, "release savepoint b")
	exec(t, conn, "savepoint b")
	exec(t, conn, "insert into t1(id1, id2) values(1,1)") // -80
	exec(t, conn, "savepoint c")
	exec(t, conn, "insert into t1(id1, id2) values(4,4)") // 80-
	exec(t, conn, "savepoint d")
	exec(t, conn, "insert into t1(id1, id2) values(2,2)") // -80
	exec(t, conn, "savepoint e")

	// Validate all the data.
	exec(t, conn, "use `ks:-80`")
	assertMatches(t, conn, "select id1 from t1 order by id1", `[[INT64(1)] [INT64(2)]]`)
	exec(t, conn, "use `ks:80-`")
	assertMatches(t, conn, "select id1 from t1 order by id1", `[[INT64(4)]]`)
	exec(t, conn, "use")
	assertMatches(t, conn, "select id1 from t1 order by id1", `[[INT64(1)] [INT64(2)] [INT64(4)]]`)

	_, err = conn.ExecuteFetch("rollback work to savepoint a", 1000, true)
	require.Error(t, err)

	exec(t, conn, "release savepoint d")

	_, err = conn.ExecuteFetch("rollback to d", 1000, true)
	require.Error(t, err)
	assertMatches(t, conn, "select id1 from t1 order by id1", `[[INT64(1)] [INT64(2)] [INT64(4)]]`)

	exec(t, conn, "rollback to c")
	assertMatches(t, conn, "select id1 from t1 order by id1", `[[INT64(1)]]`)

	exec(t, conn, "insert into t1(id1, id2) values(2,2),(3,3),(4,4)")
	assertMatches(t, conn, "select id1 from t1 order by id1", `[[INT64(1)] [INT64(2)] [INT64(3)] [INT64(4)]]`)

	exec(t, conn, "rollback to b")
	assertMatches(t, conn, "select id1 from t1 order by id1", `[]`)

	exec(t, conn, "commit")
	assertMatches(t, conn, "select id1 from t1 order by id1", `[]`)

	exec(t, conn, "start transaction")

	exec(t, conn, "insert into t1(id1, id2) values(2,2),(3,3),(4,4)")
	assertMatches(t, conn, "select id1 from t1 order by id1", `[[INT64(2)] [INT64(3)] [INT64(4)]]`)

	// After previous commit all the savepoints are cleared.
	_, err = conn.ExecuteFetch("rollback to b", 1000, true)
	require.Error(t, err)

	exec(t, conn, "rollback")
	assertMatches(t, conn, "select id1 from t1 order by id1", `[]`)
}

func TestSavepointOutsideTx(t *testing.T) {
	defer cluster.PanicHandler(t)
	ctx := context.Background()
	conn, err := mysql.Connect(ctx, &vtParams)
	require.Nil(t, err)
	defer conn.Close()

	exec(t, conn, "savepoint a")
	exec(t, conn, "savepoint b")

	_, err = conn.ExecuteFetch("rollback to b", 1, true)
	require.Error(t, err)
	_, err = conn.ExecuteFetch("release savepoint a", 1, true)
	require.Error(t, err)
}

func TestSavepointAdditionalCase(t *testing.T) {
	defer cluster.PanicHandler(t)
	ctx := context.Background()
	conn, err := mysql.Connect(ctx, &vtParams)
	require.Nil(t, err)
	defer conn.Close()

	exec(t, conn, "start transaction")
	exec(t, conn, "savepoint a")
	exec(t, conn, "insert into t1(id1, id2) values(1,1)")             // -80
	exec(t, conn, "insert into t1(id1, id2) values(2,2),(3,3),(4,4)") // -80 & 80-
	assertMatches(t, conn, "select id1 from t1 order by id1", `[[INT64(1)] [INT64(2)] [INT64(3)] [INT64(4)]]`)

	exec(t, conn, "rollback to a")
	assertMatches(t, conn, "select id1 from t1 order by id1", `[]`)

	exec(t, conn, "commit")
	assertMatches(t, conn, "select id1 from t1 order by id1", `[]`)

	exec(t, conn, "start transaction")
	exec(t, conn, "insert into t1(id1, id2) values(1,1)") // -80
	exec(t, conn, "savepoint a")
	exec(t, conn, "insert into t1(id1, id2) values(2,2),(3,3)") // -80
	exec(t, conn, "insert into t1(id1, id2) values(4,4)")       // 80-
	assertMatches(t, conn, "select id1 from t1 order by id1", `[[INT64(1)] [INT64(2)] [INT64(3)] [INT64(4)]]`)

	exec(t, conn, "rollback to a")
	assertMatches(t, conn, "select id1 from t1 order by id1", `[[INT64(1)]]`)

	exec(t, conn, "rollback")
	assertMatches(t, conn, "select id1 from t1 order by id1", `[]`)
}

func assertMatches(t *testing.T, conn *mysql.Conn, query, expected string) {
	t.Helper()
	qr := exec(t, conn, query)
	got := fmt.Sprintf("%v", qr.Rows)
	diff := cmp.Diff(expected, got)
	if diff != "" {
		t.Errorf("Query: %s (-want +got):\n%s", query, diff)
	}
}

func assertIsEmpty(t *testing.T, conn *mysql.Conn, query string) {
	t.Helper()
	qr := exec(t, conn, query)
	assert.Empty(t, qr.Rows)
}

func exec(t *testing.T, conn *mysql.Conn, query string) *sqltypes.Result {
	t.Helper()
	qr, err := conn.ExecuteFetch(query, 1000, true)
	require.NoError(t, err)
	return qr
}
