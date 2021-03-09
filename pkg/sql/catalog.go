/*
Copyright 2021 CodeNotary, Inc. All rights reserved.

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
package sql

type Catalog struct {
	databases map[string]*Database
}

type Database struct {
	name   string
	tables map[string]*Table
}

type Table struct {
	db      *Database
	name    string
	cols    map[string]*Column
	pk      string
	indexes map[string]struct{}
}

type Column struct {
	table   *Table
	colName string
	colType SQLValueType
}

func (c *Catalog) Databases() []*Database {
	dbs := make([]*Database, len(c.databases))

	i := 0
	for _, db := range c.databases {
		dbs[i] = db
		i++
	}

	return dbs
}

func (c *Catalog) ExistDatabase(db string) bool {
	_, exists := c.databases[db]
	return exists
}

func (db *Database) ExistTable(table string) bool {
	_, exists := db.tables[table]
	return exists
}