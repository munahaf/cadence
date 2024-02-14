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

package query

import (
	"encoding/json"
	"testing"

	"github.com/olivere/elastic/v7"
	"github.com/stretchr/testify/assert"
)

func TestQueryBuilder(t *testing.T) {
	qb := NewBuilder()
	qb.Query(NewExistsQuery("user"))
	qb.Size(10)
	qb.From(100)
	qb.Sortby(NewFieldSort("StartDate"))
	src, err := qb.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"from":100,"query":{"exists":{"field":"user"}},"size":10,"sort":[{"StartDate":{"order":"asc"}}]}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestBuilderAgainsESv7(t *testing.T) {
	qb := NewBuilder()
	qb.Query(NewExistsQuery("user"))
	qb.Size(10)
	qb.Sortby(NewFieldSort("runid").Desc())
	qb.Query(NewBoolQuery().Must(NewMatchQuery("domainID", "uuid"))).SearchAfter("sortval", "tiebraker")
	qbs, err := qb.Source()
	assert.NoError(t, err)

	searchSource := elastic.NewSearchSource().
		Query(elastic.NewExistsQuery("user")).
		Size(10).
		SortBy(elastic.NewFieldSort("runid").Desc()).
		Query(elastic.NewBoolQuery().Must(elastic.NewMatchQuery("domainID", "uuid"))).SearchAfter("sortval", "tiebraker")

	sss, err := searchSource.Source()
	assert.NoError(t, err)

	assert.Equal(t, sss, qbs, "ESv7 and local QueryBuilder should produce the same query")
}
