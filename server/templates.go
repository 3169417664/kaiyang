package server

import (
	"net/http"
	"strconv"
)

// planner / worker 提示词模板库 API。与 task_templates（任务描述模板）不同。

func (s *Server) listPromptTemplates(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.m == nil || s.m.pg == nil {
			writeErr(w, 503, "database unavailable")
			return
		}
		tpls, err := s.m.pg.ListPromptTemplates(kind)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"templates": tpls})
	}
}

func (s *Server) createPromptTemplate(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string `json:"name"`
			NKey string `json:"nkey"`
			Body string `json:"body"`
		}
		if !decodeJSON(w, r, &req, maxMediumJSONBodyBytes) {
			return
		}
		tpl, err := s.m.pg.CreatePromptTemplate(kind, req.Name, req.NKey, req.Body)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"template": tpl})
	}
}

func (s *Server) updatePromptTemplate(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		var req struct {
			Name string `json:"name"`
			Body string `json:"body"`
		}
		if !decodeJSON(w, r, &req, maxMediumJSONBodyBytes) {
			return
		}
		if err := s.m.pg.UpdatePromptTemplate(kind, id, req.Name, req.Body); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	}
}

func (s *Server) deletePromptTemplate(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err := s.m.pg.DeletePromptTemplate(kind, id); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	}
}

func (s *Server) setDefaultPromptTemplate(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err := s.m.pg.SetDefaultPromptTemplate(kind, id); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	}
}
