package server

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/ethpandaops/panda/pkg/operations"
	"github.com/ethpandaops/panda/pkg/proxy/handlers"
)

func (s *service) handleClickHouseOperation(operationID string, w http.ResponseWriter, r *http.Request) bool {
	switch operationID {
	case "clickhouse.list_datasources":
		s.handleClickHouseListDatasources(w)
	case "clickhouse.query", "clickhouse.query_raw":
		s.handleClickHouseQuery(w, r)
	default:
		return false
	}

	return true
}

func (s *service) handleClickHouseListDatasources(w http.ResponseWriter) {
	items := make([]listItem, 0)
	for _, info := range s.proxyService.ClickHouseDatasourceInfo() {
		item := listItem{
			Name:        info.Name,
			Description: info.Description,
			URL:         info.Metadata["url"],
			Type:        info.Type,
		}
		if database := info.Metadata["database"]; database != "" {
			item.Extra = map[string]any{"database": database}
		}
		items = append(items, item)
	}

	writeOperationResponse(s.log, w, http.StatusOK, operations.Response{
		Kind: operations.ResultKindObject,
		Data: map[string]any{"datasources": items},
	})
}

func (s *service) handleClickHouseQuery(w http.ResponseWriter, r *http.Request) {
	req, err := decodeOperationRequest(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}

	datasource, err := requiredStringArg(req.Args, "datasource")
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}

	sql, err := requiredStringArg(req.Args, "sql")
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}

	params := url.Values{"default_format": {"TabSeparatedWithNames"}}
	for key, value := range optionalMapArg(req.Args, "parameters") {
		params.Set("param_"+key, formatClickHouseParamValue(value))
	}

	body, status, headers, err := s.proxyDatasourceRequest(
		r.Context(),
		"clickhouse",
		datasource,
		http.MethodPost,
		"/clickhouse/?"+params.Encode(),
		strings.NewReader(sql),
		http.Header{
			handlers.DatasourceHeader: []string{datasource},
			"Content-Type":            []string{"text/plain"},
		},
	)
	if err != nil {
		writeAPIError(w, status, err.Error())
		return
	}

	if status < 200 || status >= 300 {
		writeAPIError(w, status, strings.TrimSpace(string(body)))
		return
	}

	writePassthroughResponse(w, http.StatusOK, headers.Get("Content-Type"), body)
}

func formatClickHouseParamValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case bool:
		if v {
			return "1"
		}
		return "0"
	default:
		return fmt.Sprint(v)
	}
}
