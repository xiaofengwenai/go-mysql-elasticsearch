package river

import (
	"fmt"
	"github.com/siddontang/go-mysql-elasticsearch/elastic"
)

const (
	syncInsertDoc = iota
	syncDeleteDoc
	syncUpdateDoc
)

// for insert and delete
func (r *River) makeRequest(rule *Rule, dtype int, rows [][]interface{}) ([]*elastic.BulkRequest, error) {
	reqs := make([]*elastic.BulkRequest, 0, len(rows))

	for _, values := range rows {
		if len(rule.TableInfo.Columns) != len(values) {
			return nil, fmt.Errorf("invalid table format for %s, column number is %d, but real data is %d",
				rule.Table, len(rule.TableInfo.Columns), len(values))
		}

		id, err := r.getDocID(rule, values)
		if err != nil {
			return nil, err
		}

		req := &elastic.BulkRequest{Index: rule.Index, Type: rule.Type, ID: id}

		if dtype == syncDeleteDoc {
			req.Action = elastic.ActionDelete
		} else {
			r.makeReqData(req, rule, values)
		}

		reqs = append(reqs, req)
	}

	return reqs, nil
}

func (r *River) makeInsertRequest(rule *Rule, rows [][]interface{}) ([]*elastic.BulkRequest, error) {
	return r.makeRequest(rule, syncInsertDoc, rows)
}

func (r *River) makeDeleteRequest(rule *Rule, rows [][]interface{}) ([]*elastic.BulkRequest, error) {
	return r.makeRequest(rule, syncDeleteDoc, rows)
}

func (r *River) makeUpdateRequest(rule *Rule, rows [][]interface{}) ([]*elastic.BulkRequest, error) {
	if len(rows)%2 != 0 {
		return nil, fmt.Errorf("invalid update rows event, must have 2x rows, but %d", len(rows))
	}

	reqs := make([]*elastic.BulkRequest, 0, len(rows))

	columnCount := len(rule.TableInfo.Columns)
	for i := 0; i < len(rows); i += 2 {
		if columnCount != len(rows[i]) {
			return nil, fmt.Errorf("invalid table format for %s, column number is %d, but real data is %d",
				rule.Table, len(rule.TableInfo.Columns), len(rows[i]))
		}

		beforeID, err := r.getDocID(rule, rows[i])
		if err != nil {
			return nil, err
		}

		afterID, err := r.getDocID(rule, rows[i+1])
		if err != nil {
			return nil, err
		}

		req := &elastic.BulkRequest{Index: rule.Index, Type: rule.Type, ID: beforeID}

		if beforeID != afterID {
			// PK has been changed in update, delete old id first
			req.Action = elastic.ActionDelete
			reqs = append(reqs, req)

			req = &elastic.BulkRequest{Index: rule.Index, Type: rule.Type, ID: afterID}
		}

		r.makeReqData(req, rule, rows[i+1])

		reqs = append(reqs, req)
	}

	return reqs, nil
}

func (r *River) syncDocument(rule *Rule, dtype int, rows [][]interface{}) error {
	if len(rows) == 0 {
		return nil
	}

	var reqs []*elastic.BulkRequest
	var err error

	switch dtype {
	case syncInsertDoc:
		reqs, err = r.makeInsertRequest(rule, rows)
	case syncDeleteDoc:
		reqs, err = r.makeDeleteRequest(rule, rows)
	case syncUpdateDoc:
		reqs, err = r.makeUpdateRequest(rule, rows)
	}

	// todo, use bulk, now, send one by one
	_, err = r.es.Bulk(reqs)

	return err
}

func (r *River) makeReqData(req *elastic.BulkRequest, rule *Rule, values []interface{}) {
	req.Data = make(map[string]interface{}, len(values))
	req.Action = elastic.ActionIndex

	for i, c := range rule.TableInfo.Columns {
		if values[i] == nil {
			// need to discard nil value ?????
			continue
		}

		if name, ok := rule.FieldMapping[c.Name]; ok {
			// has custom field mapping
			req.Data[name] = values[i]
		} else {
			req.Data[c.Name] = values[i]
		}
	}
}

func (r *River) getDocID(rule *Rule, values []interface{}) (string, error) {
	// now only support one column PK
	id := values[rule.TableInfo.PKColumns[0]]

	if id == nil {
		return "", fmt.Errorf("%s PK is nil for data %v", rule.Table, values)
	}

	return fmt.Sprintf("%v", id), nil
}
