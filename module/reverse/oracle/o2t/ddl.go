/*
Copyright © 2020 Marvin

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
package o2t

import (
	"encoding/json"
	"fmt"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/wentaojin/transferdb/module/reverse"
	"go.uber.org/zap"
	"strings"
)

type DDL struct {
	SourceSchemaName   string   `json:"source_schema"`
	SourceTableName    string   `json:"source_table_name"`
	SourceTableType    string   `json:"source_table_type"`
	SourceTableDDL     string   `json:"-"` // 忽略
	TargetSchemaName   string   `json:"target_schema"`
	TargetTableName    string   `json:"target_table_name"`
	TargetDBVersion    string   `json:"target_db_version"`
	TableColumns       []string `json:"table_columns"`
	TableKeys          []string `json:"table_keys"`
	TableSuffix        string   `json:"table_suffix"`
	TableComment       string   `json:"table_comment"`
	TableCheckKeys     []string `json:"table_check_keys""`
	TableForeignKeys   []string `json:"table_foreign_keys"`
	TableCompatibleDDL []string `json:"table_compatible_ddl"`
}

func (d *DDL) Write(w *reverse.Write) (string, error) {
	if w.Cfg.ReverseConfig.DirectWrite {
		errSql, err := d.WriteDB(w)
		if err != nil {
			return errSql, err
		}
	} else {
		errSql, err := d.WriteFile(w)
		if err != nil {
			return errSql, err
		}
	}
	return "", nil
}

func (d *DDL) WriteFile(w *reverse.Write) (string, error) {

	revDDLS, compDDLS := d.GenDDLStructure()

	var (
		sqlRev  strings.Builder
		sqlComp strings.Builder
	)

	// 表 with 主键
	sqlRev.WriteString("/*\n")
	sqlRev.WriteString(" oracle table reverse sql \n")

	sw := table.NewWriter()
	sw.SetStyle(table.StyleLight)
	sw.AppendHeader(table.Row{"#", "ORACLE TABLE TYPE", "ORACLE", "TIDB", "SUGGEST"})
	sw.AppendRows([]table.Row{
		{"TABLE", d.SourceTableType, fmt.Sprintf("%s.%s", d.SourceSchemaName, d.SourceTableName), fmt.Sprintf("%s.%s", d.TargetSchemaName, d.TargetTableName), "Create Table"},
	})
	sqlRev.WriteString(fmt.Sprintf("%v\n", sw.Render()))
	sqlRev.WriteString(fmt.Sprintf("ORIGIN DDL:%v\n", d.SourceTableDDL))
	sqlRev.WriteString("*/\n")

	sqlRev.WriteString(strings.Join(revDDLS, "\n"))

	// 兼容项处理
	if len(compDDLS) > 0 {
		sqlComp.WriteString("/*\n")
		sqlComp.WriteString(" oracle table index or consrtaint maybe tidb has compatibility, skip\n")
		tw := table.NewWriter()
		tw.SetStyle(table.StyleLight)
		tw.AppendHeader(table.Row{"#", "ORACLE", "TIDB", "SUGGEST"})
		tw.AppendRows([]table.Row{
			{"TABLE", fmt.Sprintf("%s.%s", d.SourceSchemaName, d.SourceTableName), fmt.Sprintf("%s.%s", d.TargetSchemaName, d.TargetTableName), "Create Index Or Constraints"}})

		sqlComp.WriteString(fmt.Sprintf("%v\n", tw.Render()))
		sqlComp.WriteString("*/\n")

		sqlComp.WriteString(strings.Join(compDDLS, "\n") + "\n")
	}

	// 数据写入
	if sqlRev.String() != "" {
		if _, err := w.RWriteFile(sqlRev.String()); err != nil {
			return sqlRev.String(), err
		}
	}
	if sqlComp.String() != "" {
		if _, err := w.CWriteFile(sqlComp.String()); err != nil {
			return sqlComp.String(), err
		}
	}
	return "", nil
}

func (d *DDL) WriteDB(w *reverse.Write) (string, error) {

	revDDLS, compDDLS := d.GenDDLStructure()

	var (
		sqlRev  strings.Builder
		sqlComp strings.Builder
	)

	// 表 with 主键
	sqlRev.WriteString(strings.Join(revDDLS, "\n"))

	// 兼容项处理
	if len(compDDLS) > 0 {
		sqlComp.WriteString("/*\n")
		sqlComp.WriteString(" oracle table index or consrtaint maybe tidb has compatibility, skip\n")
		tw := table.NewWriter()
		tw.SetStyle(table.StyleLight)
		tw.AppendHeader(table.Row{"#", "ORACLE", "TIDB", "SUGGEST"})
		tw.AppendRows([]table.Row{
			{"TABLE", fmt.Sprintf("%s.%s", d.SourceSchemaName, d.SourceTableName), fmt.Sprintf("%s.%s", d.TargetSchemaName, d.TargetTableName), "Create Index Or Constraints"}})

		sqlComp.WriteString(fmt.Sprintf("%v\n", tw.Render()))
		sqlComp.WriteString("*/\n")

		sqlComp.WriteString(strings.Join(compDDLS, "\n"))
	}

	// 数据写入
	if sqlRev.String() != "" {
		if err := w.RWriteDB(sqlRev.String()); err != nil {
			return sqlRev.String(), err
		}
	}
	if sqlComp.String() != "" {
		if _, err := w.CWriteFile(sqlComp.String()); err != nil {
			return sqlComp.String(), err
		}
	}
	return "", nil
}

func (d *DDL) GenDDLStructure() ([]string, []string) {
	var (
		reverseDDLS   []string
		compDDLS      []string
		tableDDL      string
		checkKeyDDL   []string
		foreignKeyDDL []string
	)

	// 表 with 主键
	var structDDL string
	if len(d.TableKeys) > 0 {
		structDDL = fmt.Sprintf("CREATE TABLE `%s`.`%s` (\n%s,\n%s\n)",
			d.TargetSchemaName,
			d.TargetTableName,
			strings.Join(d.TableColumns, ",\n"),
			strings.Join(d.TableKeys, ",\n"))
	} else {
		structDDL = fmt.Sprintf("CREATE TABLE `%s`.`%s` (\n%s\n)",
			d.TargetSchemaName,
			d.TargetTableName,
			strings.Join(d.TableColumns, ",\n"))
	}

	if strings.EqualFold(d.TableComment, "") {
		tableDDL = fmt.Sprintf("%s %s;", structDDL, d.TableSuffix)
	} else {
		tableDDL = fmt.Sprintf("%s %s %s;", structDDL, d.TableSuffix, d.TableComment)
	}

	zap.L().Info("reverse oracle table structure",
		zap.String("schema", d.TargetSchemaName),
		zap.String("table", d.TargetTableName),
		zap.String("sql", tableDDL))

	reverseDDLS = append(reverseDDLS, tableDDL+"\n")

	// foreign and check key sql ddl
	if len(d.TableForeignKeys) > 0 {
		for _, fk := range d.TableForeignKeys {
			fkSQL := fmt.Sprintf("ALTER TABLE `%s`.`%s` ADD %s;",
				d.TargetSchemaName, d.TargetTableName, fk)
			zap.L().Info("reverse oracle table foreign key",
				zap.String("schema", d.TargetSchemaName),
				zap.String("table", d.TargetTableName),
				zap.String("fk sql", fkSQL))
			foreignKeyDDL = append(foreignKeyDDL, fkSQL)
		}
	}
	if len(d.TableCheckKeys) > 0 {
		for _, ck := range d.TableCheckKeys {
			ckSQL := fmt.Sprintf("ALTER TABLE `%s`.`%s` ADD %s;",
				d.TargetSchemaName, d.TargetTableName, ck)
			zap.L().Info("reverse oracle table check key",
				zap.String("schema", d.TargetSchemaName),
				zap.String("table", d.TargetTableName),
				zap.String("ck sql", ckSQL))
			checkKeyDDL = append(checkKeyDDL, ckSQL)
		}
	}

	// 外键约束、检查约束
	// TiDB 增加不兼容性语句
	if len(foreignKeyDDL) > 0 {
		for _, sql := range foreignKeyDDL {
			compDDLS = append(compDDLS, sql)
		}
	}
	if len(checkKeyDDL) > 0 {
		for _, sql := range checkKeyDDL {
			compDDLS = append(compDDLS, sql)
		}
	}
	if len(d.TableCompatibleDDL) > 0 {
		for _, sql := range d.TableCompatibleDDL {
			compDDLS = append(compDDLS, sql)
		}
	}

	return reverseDDLS, compDDLS
}

func (d *DDL) String() string {
	jsonBytes, _ := json.Marshal(d)
	return string(jsonBytes)
}
