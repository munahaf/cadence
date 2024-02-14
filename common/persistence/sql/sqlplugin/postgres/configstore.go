// The MIT License (MIT)

// Copyright (c) 2017-2020 Uber Technologies Inc.

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package postgres

import (
	"context"

	"github.com/uber/cadence/common"
	"github.com/uber/cadence/common/persistence"
	"github.com/uber/cadence/common/persistence/sql/sqlplugin"
)

const (
	_selectLatestConfigQuery = "SELECT row_type, version, timestamp, data, data_encoding FROM cluster_config WHERE row_type = $1 ORDER BY version LIMIT 1;"

	_insertConfigQuery = "INSERT INTO cluster_config (row_type, version, timestamp, data, data_encoding) VALUES($1, $2, $3, $4, $5)"
)

func (pdb *db) InsertConfig(ctx context.Context, row *persistence.InternalConfigStoreEntry) error {
	_, err := pdb.driver.ExecContext(ctx, sqlplugin.DbDefaultShard, _insertConfigQuery, row.RowType, -1*row.Version, pdb.converter.ToPostgresDateTime(row.Timestamp), row.Values.Data, row.Values.Encoding)
	return err
}

func (pdb *db) SelectLatestConfig(ctx context.Context, rowType int) (*persistence.InternalConfigStoreEntry, error) {
	var row sqlplugin.ClusterConfigRow
	err := pdb.driver.GetContext(ctx, sqlplugin.DbDefaultShard, &row, _selectLatestConfigQuery, rowType)
	if err != nil {
		return nil, err
	}
	row.Version *= -1
	return &persistence.InternalConfigStoreEntry{
		RowType:   row.RowType,
		Version:   row.Version,
		Timestamp: pdb.converter.FromPostgresDateTime(row.Timestamp),
		Values: &persistence.DataBlob{
			Data:     row.Data,
			Encoding: common.EncodingType(row.DataEncoding),
		},
	}, nil
}
