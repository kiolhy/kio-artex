package server

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (s *Server) pushRegister(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	var req struct {
		Token      string `json:"token"`
		Platform   string `json:"platform"`
		DeviceName string `json:"device_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "请求体格式错误")
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" {
		writeErr(w, 400, "token 不能为空")
		return
	}
	if req.Platform == "" {
		req.Platform = "android"
	}
	if err := pg.RegisterPushToken(r.Context(), req.Token, req.Platform, req.DeviceName); err != nil {
		writeErr(w, 500, "保存推送设备失败")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) pushUnregister(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "请求体格式错误")
		return
	}
	if err := pg.UnregisterPushToken(r.Context(), strings.TrimSpace(req.Token)); err != nil {
		writeErr(w, 500, "删除推送设备失败")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) pushDevices(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	tokens, err := pg.ListPushTokens(r.Context())
	if err != nil {
		writeErr(w, 500, "读取推送设备失败")
		return
	}
	writeJSON(w, 200, map[string]any{"devices": tokens})
}
