package server

import (
	"net/http"
	"strconv"
)

// GET /api/credentials/intranet —— 内网凭据（脱敏，不含 secret）。
func (s *Server) getIntranetCredentials(w http.ResponseWriter, r *http.Request) {
	as := s.m.Assets()
	if as == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	q := r.URL.Query()
	companyID, _ := strconv.ParseInt(q.Get("company_id"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	if limit <= 0 {
		limit = 50
	} else if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	credentials, total, err := as.Credentials().QueryIntranet(companyID, limit, offset)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"credentials": credentials, "total": total})
}
